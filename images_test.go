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
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/writeas/impart"

	"github.com/writefreely/writefreely/config"
)

func TestUploadConfigDefaults(t *testing.T) {
	cfg := config.New()
	assert.False(t, cfg.Uploads.Enabled, "uploads are opt-in")
	assert.Equal(t, 10, cfg.Uploads.MaxSizeMB)
	assert.Equal(t, []string{"image/png", "image/jpeg", "image/gif"}, cfg.AllowedUploadTypes())
}

func TestPostImagesTableExists(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	_, err := app.db.Exec("SELECT id, owner_id, post_id, sha256, filename, mime, size, created FROM post_images WHERE 1 = 0")
	assert.NoError(t, err, "post_images must exist with the expected columns")
}

// testPNGSum is the SHA-256 of an arbitrary blob used where the bytes
// themselves don't matter, only that the sum is stable and distinct.
func testSum(s string) string {
	return sha256Hex([]byte(s))
}

func TestPostImageCreateAndGet(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	u, _, _ := createTemplateTestUser(t, app, "imgowner")

	sum := testSum("one")
	img, err := app.db.CreatePostImage(u.ID, sum, "holiday.png", "image/png", 1234)
	assert.NoError(t, err)
	assert.NotEmpty(t, img.ID)
	assert.Equal(t, sum, img.Sum)
	assert.Equal(t, "/uploads/"+strconv.FormatInt(u.ID, 10)+"/"+sum[:2]+"/"+sum+".png", img.URL())

	got, err := app.db.GetPostImage(img.ID)
	assert.NoError(t, err)
	assert.Equal(t, img.ID, got.ID)
	assert.Equal(t, u.ID, got.OwnerID)
	assert.Equal(t, "holiday.png", got.Filename)
	assert.Equal(t, "image/png", got.Mime)
	assert.Equal(t, 1234, got.Size)
	assert.False(t, got.PostID.Valid, "a fresh image is not attached to a post")
}

func TestPostImageGetBySum(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	u, _, _ := createTemplateTestUser(t, app, "imgowner")

	sum := testSum("two")
	img, err := app.db.CreatePostImage(u.ID, sum, "a.png", "image/png", 1)
	assert.NoError(t, err)

	got, err := app.db.GetPostImageBySum(u.ID, sum)
	assert.NoError(t, err)
	assert.Equal(t, img.ID, got.ID)

	_, err = app.db.GetPostImageBySum(u.ID+1000, sum)
	assert.Error(t, err, "another owner's sum must not resolve")
}

func TestPostImageDuplicateReturnsExisting(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	u, _, _ := createTemplateTestUser(t, app, "imgowner")

	sum := testSum("three")
	first, err := app.db.CreatePostImage(u.ID, sum, "a.png", "image/png", 1)
	assert.NoError(t, err)
	second, err := app.db.CreatePostImage(u.ID, sum, "b.png", "image/png", 1)
	assert.NoError(t, err, "re-uploading identical bytes is a success, not a conflict")
	assert.Equal(t, first.ID, second.ID)

	var count int
	assert.NoError(t, app.db.QueryRow("SELECT COUNT(*) FROM post_images WHERE owner_id = ?", u.ID).Scan(&count))
	assert.Equal(t, 1, count)
}

func TestPostImageAttachAndList(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	u, _, post := createTemplateTestUser(t, app, "imgowner")

	img, err := app.db.CreatePostImage(u.ID, testSum("four"), "a.png", "image/png", 1)
	assert.NoError(t, err)

	assert.NoError(t, app.db.AttachImagesToPost(u.ID, post.ID, []string{img.ID}))

	imgs, err := app.db.GetImagesForPost(post.ID)
	assert.NoError(t, err)
	assert.Len(t, *imgs, 1)
	assert.Equal(t, img.ID, (*imgs)[0].ID)
}

func TestPostImageDelete(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	u, _, _ := createTemplateTestUser(t, app, "imgowner")

	img, err := app.db.CreatePostImage(u.ID, testSum("five"), "a.png", "image/png", 1)
	assert.NoError(t, err)

	assert.NoError(t, app.db.DeletePostImage(img.ID))
	_, err = app.db.GetPostImage(img.ID)
	assert.Error(t, err)
}

func TestPostImageCountPostsReferencingImage(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	u, coll, _ := createTemplateTestUser(t, app, "imgowner")

	img, err := app.db.CreatePostImage(u.ID, testSum("six"), "a.png", "image/png", 1)
	assert.NoError(t, err)

	n, err := app.db.CountPostsReferencingImage(img.URL(), "")
	assert.NoError(t, err)
	assert.Equal(t, 0, n)

	title := "One"
	body := "Look: ![a](" + img.URL() + ")"
	p1, err := app.db.CreatePost(u.ID, coll.ID, &SubmittedPost{Title: &title, Content: &body})
	assert.NoError(t, err)

	n, err = app.db.CountPostsReferencingImage(img.URL(), "")
	assert.NoError(t, err)
	assert.Equal(t, 1, n)

	n, err = app.db.CountPostsReferencingImage(img.URL(), p1.ID)
	assert.NoError(t, err)
	assert.Equal(t, 0, n, "the excluded post must not count itself")

	title2 := "Two"
	_, err = app.db.CreatePost(u.ID, coll.ID, &SubmittedPost{Title: &title2, Content: &body})
	assert.NoError(t, err)

	n, err = app.db.CountPostsReferencingImage(img.URL(), "")
	assert.NoError(t, err)
	assert.Equal(t, 2, n)

	n, err = app.db.CountPostsReferencingImage(img.URL(), p1.ID)
	assert.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestPostImageGetOrphanedImages(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	u, _, post := createTemplateTestUser(t, app, "imgowner")

	old, err := app.db.CreatePostImage(u.ID, testSum("old"), "old.png", "image/png", 1)
	assert.NoError(t, err)
	recent, err := app.db.CreatePostImage(u.ID, testSum("recent"), "new.png", "image/png", 1)
	assert.NoError(t, err)
	attached, err := app.db.CreatePostImage(u.ID, testSum("attached"), "att.png", "image/png", 1)
	assert.NoError(t, err)
	assert.NoError(t, app.db.AttachImagesToPost(u.ID, post.ID, []string{attached.ID}))

	ageImage(t, app, old.ID, 25)
	ageImage(t, app, recent.ID, 1)
	ageImage(t, app, attached.ID, 25)

	orphans, err := app.db.GetOrphanedImages(24)
	assert.NoError(t, err)

	ids := []string{}
	for _, o := range *orphans {
		ids = append(ids, o.ID)
	}
	assert.Contains(t, ids, old.ID)
	assert.NotContains(t, ids, recent.ID, "a recent draft upload is still in use")
	assert.NotContains(t, ids, attached.ID, "an attached image is never an orphan")
}

// ageImage backdates an image's created time by the given number of hours.
func ageImage(t *testing.T, app *App, id string, hours int) {
	t.Helper()
	_, err := app.db.Exec("UPDATE post_images SET created = "+app.db.dateAdd(-hours, "HOUR")+" WHERE id = ?", id)
	assert.NoError(t, err)
}

// newImageTestApp builds a test app with uploads enabled and its static
// directory in a temp dir, so uploaded files never touch the repository.
func newImageTestApp(t *testing.T) (*App, *mux.Router, *User, []*http.Cookie) {
	t.Helper()
	staticDir := t.TempDir()
	app, router := newTemplateTestApp(t, func(cfg *config.Config) {
		cfg.Server.StaticParentDir = staticDir
		cfg.Uploads.Enabled = true
		cfg.Uploads.MaxSizeMB = 1
	})
	u, _, _ := createTemplateTestUser(t, app, "uploader")
	return app, router, u, []*http.Cookie{loginCookie(t, app, u)}
}

// uploadRequest builds a multipart request carrying the given bytes as the
// "file" field, with a deliberately misleading name and content type.
func uploadRequest(t *testing.T, name, contentType string, body []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="`+name+`"`)
	h.Set("Content-Type", contentType)
	part, err := mw.CreatePart(h)
	if err != nil {
		t.Fatalf("create multipart part: %v", err)
	}
	if _, err = part.Write(body); err != nil {
		t.Fatalf("write multipart part: %v", err)
	}
	if err = mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	r := httptest.NewRequest("POST", "/api/me/images", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return r
}

// doUpload runs the upload handler directly, bypassing CSRF, and returns the
// recorder along with the status the handler resolved to.
func doUpload(t *testing.T, app *App, u *User, r *http.Request) (*httptest.ResponseRecorder, int) {
	t.Helper()
	rec := httptest.NewRecorder()
	err := handleUploadImage(app, u, rec, r)
	if err == nil {
		return rec, http.StatusOK
	}
	if he, ok := err.(impart.HTTPError); ok {
		return rec, he.Status
	}
	t.Fatalf("upload returned a non-HTTP error: %v", err)
	return rec, 0
}

// doDelete runs the delete handler directly for the given image ID.
func doDelete(t *testing.T, app *App, u *User, imgID string) (*httptest.ResponseRecorder, int) {
	t.Helper()
	r := httptest.NewRequest("DELETE", "/api/me/images/"+imgID, nil)
	r = mux.SetURLVars(r, map[string]string{"image": imgID})
	rec := httptest.NewRecorder()
	err := handleDeleteImage(app, u, rec, r)
	if err == nil {
		return rec, rec.Code
	}
	if he, ok := err.(impart.HTTPError); ok {
		return rec, he.Status
	}
	t.Fatalf("delete returned a non-HTTP error: %v", err)
	return rec, 0
}

// countUploadedFiles returns how many regular files live under the uploads
// root.
func countUploadedFiles(t *testing.T, app *App) int {
	t.Helper()
	n := 0
	err := filepath.Walk(app.uploadsRoot(), func(_ string, info os.FileInfo, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			n++
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("walk uploads: %v", err)
	}
	return n
}

// uploadedURL pulls the stored image's URL out of an upload response.
func uploadedURL(t *testing.T, rec *httptest.ResponseRecorder) (id, url string) {
	t.Helper()
	var res struct {
		Data struct {
			ID   string `json:"id"`
			URL  string `json:"url"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode upload response %q: %v", rec.Body.String(), err)
	}
	return res.Data.ID, res.Data.URL
}

func TestImageUploadStoresReencodedFile(t *testing.T) {
	app, _, u, _ := newImageTestApp(t)

	rec, status := doUpload(t, app, u, uploadRequest(t, "../../evil.png", "text/plain", tinyPNG(t)))
	assert.Equal(t, http.StatusOK, status)

	imgID, url := uploadedURL(t, rec)
	assert.NotEmpty(t, imgID)
	assert.True(t, strings.HasPrefix(url, "/uploads/"+strconv.FormatInt(u.ID, 10)+"/"), "path is derived from the owner, not the filename")
	assert.NotContains(t, url, "..", "the client filename never reaches the path")
	assert.NotContains(t, url, "evil")
	assert.Equal(t, 1, countUploadedFiles(t, app))

	img, err := app.db.GetPostImage(imgID)
	assert.NoError(t, err)
	// mime/multipart strips any directory part of the submitted name; what
	// is left is kept for display only.
	assert.Equal(t, "evil.png", img.Filename)
	assert.Equal(t, "image/png", img.Mime)
}

func TestImageUploadRejectsOversized(t *testing.T) {
	app, _, u, _ := newImageTestApp(t)

	big := make([]byte, 2*1024*1024)
	copy(big, tinyPNG(t))
	_, status := doUpload(t, app, u, uploadRequest(t, "big.png", "image/png", big))
	assert.Equal(t, http.StatusRequestEntityTooLarge, status)
	assert.Equal(t, 0, countUploadedFiles(t, app), "an oversized upload must not reach disk")
}

func TestImageUploadRejectsHTMLNamedAsPNG(t *testing.T) {
	app, _, u, _ := newImageTestApp(t)

	_, status := doUpload(t, app, u, uploadRequest(t, "sneaky.png", "image/png", []byte("<html><script>alert(1)</script></html>")))
	assert.Equal(t, http.StatusUnsupportedMediaType, status)
	assert.Equal(t, 0, countUploadedFiles(t, app))
}

func TestImageUploadRejectsSVG(t *testing.T) {
	app, _, u, _ := newImageTestApp(t)

	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	_, status := doUpload(t, app, u, uploadRequest(t, "logo.png", "image/png", svg))
	assert.Equal(t, http.StatusUnsupportedMediaType, status, "SVG must be refused whatever it claims to be")
	assert.Equal(t, 0, countUploadedFiles(t, app))
}

func TestImageUploadDeduplicates(t *testing.T) {
	app, _, u, _ := newImageTestApp(t)
	png := tinyPNG(t)

	rec1, status := doUpload(t, app, u, uploadRequest(t, "a.png", "image/png", png))
	assert.Equal(t, http.StatusOK, status)
	rec2, status := doUpload(t, app, u, uploadRequest(t, "b.png", "image/png", png))
	assert.Equal(t, http.StatusOK, status)

	id1, url1 := uploadedURL(t, rec1)
	id2, url2 := uploadedURL(t, rec2)
	assert.Equal(t, url1, url2)
	assert.Equal(t, id1, id2)
	assert.Equal(t, 1, countUploadedFiles(t, app))
}

func TestImageUploadDisabledReturnsNotFound(t *testing.T) {
	app, _, u, _ := newImageTestApp(t)
	app.cfg.Uploads.Enabled = false

	_, status := doUpload(t, app, u, uploadRequest(t, "a.png", "image/png", tinyPNG(t)))
	assert.Equal(t, http.StatusNotFound, status, "a disabled feature should not advertise itself")
}

func TestImageUploadRequiresSession(t *testing.T) {
	_, router, _, _ := newImageTestApp(t)

	req := uploadRequest(t, "a.png", "image/png", tinyPNG(t))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.True(t, rec.Code >= 400, "an unauthenticated upload must be refused, got %d", rec.Code)
}

func TestImageDeleteRemovesUnreferencedImage(t *testing.T) {
	app, _, u, _ := newImageTestApp(t)

	rec, _ := doUpload(t, app, u, uploadRequest(t, "a.png", "image/png", tinyPNG(t)))
	imgID, _ := uploadedURL(t, rec)

	_, status := doDelete(t, app, u, imgID)
	assert.Equal(t, http.StatusNoContent, status)
	assert.Equal(t, 0, countUploadedFiles(t, app))
	_, err := app.db.GetPostImage(imgID)
	assert.Error(t, err)
}

func TestImageDeleteRefusesReferencedImage(t *testing.T) {
	app, _, u, _ := newImageTestApp(t)
	coll, err := app.db.GetCollection("uploader")
	assert.NoError(t, err)

	rec, _ := doUpload(t, app, u, uploadRequest(t, "a.png", "image/png", tinyPNG(t)))
	imgID, url := uploadedURL(t, rec)

	title := "Uses the image"
	body := "![a](" + url + ")"
	_, err = app.db.CreatePost(u.ID, coll.ID, &SubmittedPost{Title: &title, Content: &body})
	assert.NoError(t, err)

	_, status := doDelete(t, app, u, imgID)
	assert.Equal(t, http.StatusConflict, status)
	assert.Equal(t, 1, countUploadedFiles(t, app), "a referenced file must survive")
	_, err = app.db.GetPostImage(imgID)
	assert.NoError(t, err)
}

func TestImageDeleteRejectsAnotherUsersImage(t *testing.T) {
	app, _, u, _ := newImageTestApp(t)
	other, _, _ := createTemplateTestUser(t, app, "stranger")

	rec, _ := doUpload(t, app, u, uploadRequest(t, "a.png", "image/png", tinyPNG(t)))
	imgID, _ := uploadedURL(t, rec)

	_, status := doDelete(t, app, other, imgID)
	assert.Equal(t, http.StatusNotFound, status)
	assert.Equal(t, 1, countUploadedFiles(t, app))
}

func TestUploadsAreServedWithHardeningHeaders(t *testing.T) {
	app, router, u, _ := newImageTestApp(t)

	rec, _ := doUpload(t, app, u, uploadRequest(t, "a.png", "image/png", tinyPNG(t)))
	_, url := uploadedURL(t, rec)

	req := httptest.NewRequest("GET", url, nil)
	got := httptest.NewRecorder()
	router.ServeHTTP(got, req)

	assert.Equal(t, http.StatusOK, got.Code)
	assert.Equal(t, "nosniff", got.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "inline", got.Header().Get("Content-Disposition"))
}
