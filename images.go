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

	"github.com/gorilla/mux"
	"github.com/writeas/impart"
	"github.com/writeas/web-core/log"
)

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
