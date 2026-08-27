/*
 * Copyright © 2026 Musing Studio LLC.
 *
 * This file is part of WriteFreely.
 *
 * WriteFreely is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License, included
 * in the LICENSE file in this source code package.
 */

package writefreely

import (
	"errors"
	"io"
	"net/http"
	"regexp"

	"github.com/gorilla/mux"
	"github.com/writeas/impart"
	"github.com/writeas/web-core/log"
)

// orphanImageHours is how long an uploaded image may go unattached to a post
// before the sweep removes it.
const orphanImageHours = 24

// uploadedImage is the JSON representation of a stored image, as returned to
// the editor.
type uploadedImage struct {
	ID   string `json:"id"`
	URL  string `json:"url"`
	Name string `json:"name"`
}

// handleUploadImage stores an image uploaded from the editor and returns the
// URL the instance serves it from.
//
// The order of operations here is deliberate: the body is bounded before it is
// parsed, the type is decided by sniffing the content rather than by trusting
// the submitted filename or Content-Type, the image is re-encoded to discard
// metadata, and the path it is stored at is derived entirely from the owner's
// ID and the hash of the stored bytes.
func handleUploadImage(app *App, u *User, w http.ResponseWriter, r *http.Request) error {
	if !app.cfg.Uploads.Enabled {
		// A disabled feature shouldn't advertise itself.
		return impart.HTTPError{http.StatusNotFound, "Not found."}
	}
	if u.IsSilenced() {
		return ErrUserSilenced
	}

	maxBytes := app.cfg.MaxUploadBytes()

	// Bound the body before parsing it, so an oversized upload is refused
	// without ever being buffered.
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return impart.HTTPError{http.StatusRequestEntityTooLarge, "That image is too large."}
		}
		return impart.HTTPError{http.StatusBadRequest, "Couldn't read the uploaded file."}
	}
	defer r.MultipartForm.RemoveAll()

	file, header, err := r.FormFile("file")
	if err != nil {
		return impart.HTTPError{http.StatusBadRequest, "No file was uploaded."}
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return impart.HTTPError{http.StatusRequestEntityTooLarge, "That image is too large."}
		}
		return impart.HTTPError{http.StatusBadRequest, "Couldn't read the uploaded file."}
	}

	stored, mime, ext, err := decodeAndReencode(data, app.cfg.AllowedUploadTypes())
	if err != nil {
		if err == errUnsupportedImageType {
			return impart.HTTPError{http.StatusUnsupportedMediaType, "That file type isn't supported."}
		}
		log.Error("Failed re-encoding uploaded image: %v", err)
		return impart.HTTPError{http.StatusInternalServerError, "Couldn't process that image."}
	}

	// Identify the image by what we actually store, not by what was sent.
	sum := sha256Hex(stored)

	if existing, err := app.db.GetPostImageBySum(u.ID, sum); err == nil {
		return impart.WriteSuccess(w, uploadedImage{existing.ID, existing.URL(), existing.Filename}, http.StatusOK)
	}

	relPath := imagePath(u.ID, sum, ext)
	if err = app.writeUploadedImage(relPath, stored); err != nil {
		log.Error("Failed writing uploaded image: %v", err)
		return impart.HTTPError{http.StatusInsufficientStorage, "Couldn't store that image."}
	}

	// The client's filename is kept for display only; it never influences
	// where the file lands.
	img, err := app.db.CreatePostImage(u.ID, sum, header.Filename, mime, len(stored))
	if err != nil {
		if rmErr := app.removeUploadedImage(relPath); rmErr != nil {
			log.Error("Failed removing image after a failed insert: %v", rmErr)
		}
		return err
	}

	return impart.WriteSuccess(w, uploadedImage{img.ID, img.URL(), img.Filename}, http.StatusOK)
}

// handleDeleteImage removes one of the current user's images, along with its
// file, as long as no post still references it.
func handleDeleteImage(app *App, u *User, w http.ResponseWriter, r *http.Request) error {
	if !app.cfg.Uploads.Enabled {
		return impart.HTTPError{http.StatusNotFound, "Not found."}
	}

	imgID := mux.Vars(r)["image"]
	img, err := app.db.GetPostImage(imgID)
	if err != nil {
		return err
	}
	if img.OwnerID != u.ID {
		// Don't confirm that someone else's image exists.
		return impart.HTTPError{http.StatusNotFound, "Image doesn't exist."}
	}

	refs, err := app.db.CountPostsReferencingImage(img.URL(), "")
	if err != nil {
		return err
	}
	if refs > 0 {
		return impart.HTTPError{http.StatusConflict, "Another post still uses this image."}
	}

	// Delete the row first: an orphaned file is recoverable, but a row
	// pointing at a missing file is a broken page.
	if err = app.db.DeletePostImage(img.ID); err != nil {
		return err
	}
	if err = app.removeUploadedImage(img.RelPath()); err != nil {
		log.Error("Failed removing image file %s: %v", img.RelPath(), err)
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// uploadHeaders hardens the responses served for uploaded files. This is
// defense in depth behind the type allow-list, not a substitute for it.
func uploadHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Disposition", "inline")
		next.ServeHTTP(w, r)
	})
}

// imageURLPattern matches the URLs of images this instance hosts, so a post's
// body can be scanned for the ones it uses.
var imageURLPattern = regexp.MustCompile(`/` + uploadsDir + `/[0-9]+/[0-9a-f]{2}/([0-9a-f]{64})\.[a-z]+`)

// attachPostImages records which of the user's uploaded images the given post
// body uses, so the images stop being orphans and can be cleaned up with the
// post. Failing to attach must never fail the save, so problems are logged
// rather than returned.
func attachPostImages(app *App, ownerID int64, postID, content string) {
	if !app.cfg.Uploads.Enabled || postID == "" {
		return
	}

	matches := imageURLPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return
	}

	seen := map[string]bool{}
	imageIDs := []string{}
	for _, m := range matches {
		sum := m[1]
		if seen[sum] {
			continue
		}
		seen[sum] = true

		img, err := app.db.GetPostImageBySum(ownerID, sum)
		if err != nil {
			// Someone else's image, or one that's already gone.
			continue
		}
		imageIDs = append(imageIDs, img.ID)
	}

	if err := app.db.AttachImagesToPost(ownerID, postID, imageIDs); err != nil {
		log.Error("Unable to attach images to post %s: %v", postID, err)
	}
}

// cleanUpPostImages removes the images that belonged to a just-deleted post,
// keeping any that another post still references. The post is already gone by
// the time this runs, so failures are logged rather than returned.
func cleanUpPostImages(app *App, postID string) {
	if !app.cfg.Uploads.Enabled {
		return
	}

	imgs, err := app.db.GetImagesForPost(postID)
	if err != nil {
		log.Error("Unable to get images for deleted post %s: %v", postID, err)
		return
	}

	for _, img := range *imgs {
		removeImageIfUnreferenced(app, &img, postID)
	}
}

// removeImageIfUnreferenced deletes an image's row and file if no post other
// than excludingPostID still has its URL in the body.
func removeImageIfUnreferenced(app *App, img *PostImage, excludingPostID string) {
	refs, err := app.db.CountPostsReferencingImage(img.URL(), excludingPostID)
	if err != nil {
		log.Error("Unable to count references to image %s: %v", img.ID, err)
		return
	}
	if refs > 0 {
		return
	}
	if err = app.db.DeletePostImage(img.ID); err != nil {
		log.Error("Unable to delete image %s: %v", img.ID, err)
		return
	}
	if err = app.removeUploadedImage(img.RelPath()); err != nil {
		log.Error("Unable to remove image file %s: %v", img.RelPath(), err)
	}
}

// sweepOrphanedImages removes uploads that were never attached to a post --
// what's left behind when a draft is abandoned after an image is dropped into
// it.
func sweepOrphanedImages(app *App) {
	imgs, err := app.db.GetOrphanedImages(orphanImageHours)
	if err != nil {
		log.Error("[jobs] Unable to get orphaned images: %v", err)
		return
	}
	for _, img := range *imgs {
		removeImageIfUnreferenced(app, &img, "")
	}
}
