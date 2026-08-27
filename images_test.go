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
	"regexp"
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

func TestImageUploadAcceptsSVGAndServesItSafely(t *testing.T) {
	_, router, _, cookies := newImageTestApp(t)

	padReq := httptest.NewRequest("GET", "/me/new", nil)
	for _, c := range cookies {
		padReq.AddCookie(c)
	}
	padRec := httptest.NewRecorder()
	router.ServeHTTP(padRec, padReq)
	m := regexp.MustCompile(`csrfToken: "([^"]*)"`).FindStringSubmatch(padRec.Body.String())
	assert.NotNil(t, m)
	if m == nil {
		return
	}

	svg := []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><rect width="1" height="1"/></svg>`)
	req := uploadRequest(t, "diagram.svg", "image/svg+xml", svg)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	for _, c := range padRec.Result().Cookies() {
		req.AddCookie(c)
	}
	req.Header.Set("X-CSRF-Token", m[1])
	// gorilla/csrf assumes TLS on this branch, so this is the Origin a
	// browser would send over HTTPS.
	req.Header.Set("Origin", "https://example.com")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var res struct {
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	assert.True(t, strings.HasSuffix(res.Data.URL, ".svg"), "got %q", res.Data.URL)

	// Serving it must sandbox the response and force a download on
	// navigation, so script inside it can never run in this origin.
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, httptest.NewRequest("GET", res.Data.URL, nil))
	assert.Equal(t, http.StatusOK, getRec.Code)
	assert.Contains(t, getRec.Header().Get("Content-Security-Policy"), "sandbox")
	assert.Equal(t, "attachment", getRec.Header().Get("Content-Disposition"))
	assert.Equal(t, "image/svg+xml", getRec.Header().Get("Content-Type"))
	assert.Equal(t, "nosniff", getRec.Header().Get("X-Content-Type-Options"))
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

func TestImageUploadRejectsUnauthenticatedRequest(t *testing.T) {
	app, router, _, cookies := newImageTestApp(t)

	// No session and no CSRF token.
	req := uploadRequest(t, "a.png", "image/png", tinyPNG(t))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.True(t, rec.Code >= 400, "an unauthenticated upload must be refused, got %d", rec.Code)

	// A session but no CSRF token is refused too.
	req = uploadRequest(t, "a.png", "image/png", tinyPNG(t))
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	assert.Equal(t, 0, countUploadedFiles(t, app))
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

func TestPadIncludesUploaderOnlyWhenEnabled(t *testing.T) {
	_, router, _, cookies := newImageTestApp(t)
	rec := assertRendersCleanly(t, router, "GET", "/me/new", cookies, http.StatusOK)
	assert.Contains(t, rec.Body.String(), "/js/image-upload.js")
	assert.Contains(t, rec.Body.String(), `id="image-strip"`)

	staticDir := t.TempDir()
	app2, router2 := newTemplateTestApp(t, func(cfg *config.Config) {
		cfg.Server.StaticParentDir = staticDir
	})
	u2, _, _ := createTemplateTestUser(t, app2, "nouploads")
	rec2 := assertRendersCleanly(t, router2, "GET", "/me/new", []*http.Cookie{loginCookie(t, app2, u2)}, http.StatusOK)
	assert.NotContains(t, rec2.Body.String(), "/js/image-upload.js",
		"a disabled feature must not appear in the editor")
}

func TestImageUploadThroughRouterWithCSRF(t *testing.T) {
	app, router, u, cookies := newImageTestApp(t)

	// Load the editor to pick up a CSRF token and its cookie.
	padReq := httptest.NewRequest("GET", "/me/new", nil)
	for _, c := range cookies {
		padReq.AddCookie(c)
	}
	padRec := httptest.NewRecorder()
	router.ServeHTTP(padRec, padReq)
	assert.Equal(t, http.StatusOK, padRec.Code)

	m := regexp.MustCompile(`csrfToken: "([^"]*)"`).FindStringSubmatch(padRec.Body.String())
	assert.NotNil(t, m, "the editor must carry a CSRF token")
	if m == nil {
		return
	}
	assert.NotEmpty(t, m[1])

	req := uploadRequest(t, "a.png", "image/png", tinyPNG(t))
	for _, c := range cookies {
		req.AddCookie(c)
	}
	for _, c := range padRec.Result().Cookies() {
		req.AddCookie(c)
	}
	req.Header.Set("X-CSRF-Token", m[1])
	// The test instance's configured host is http://, so the routes mark
	// requests as plaintext and gorilla/csrf compares the Origin against an
	// http scheme -- exactly what a browser on such an instance sends.
	req.Header.Set("Origin", "http://example.com")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Equal(t, 1, countUploadedFiles(t, app))
	_ = u
}

func TestProseEditorIncludesUploader(t *testing.T) {
	staticDir := t.TempDir()
	app, router := newTemplateTestApp(t, func(cfg *config.Config) {
		cfg.Server.StaticParentDir = staticDir
		cfg.App.Editor = "classic"
		cfg.Uploads.Enabled = true
	})
	u, _, _ := createTemplateTestUser(t, app, "proser")
	rec := assertRendersCleanly(t, router, "GET", "/me/new", []*http.Cookie{loginCookie(t, app, u)}, http.StatusOK)
	assert.Contains(t, rec.Body.String(), "/js/image-upload.js")
	assert.Contains(t, rec.Body.String(), "window.wfImageUploads")
	assert.Contains(t, rec.Body.String(), `id="image-strip"`)
}

func TestSavingPostAttachesItsImages(t *testing.T) {
	app, _, u, _ := newImageTestApp(t)
	coll, err := app.db.GetCollection("uploader")
	assert.NoError(t, err)

	rec, _ := doUpload(t, app, u, uploadRequest(t, "a.png", "image/png", tinyPNG(t)))
	imgID, url := uploadedURL(t, rec)

	title := "With an image"
	body := "Look at this ![a](" + url + ")"
	post, err := app.db.CreatePost(u.ID, coll.ID, &SubmittedPost{Title: &title, Content: &body})
	assert.NoError(t, err)

	attachPostImages(app, u.ID, post.ID, body)

	imgs, err := app.db.GetImagesForPost(post.ID)
	assert.NoError(t, err)
	assert.Len(t, *imgs, 1)
	assert.Equal(t, imgID, (*imgs)[0].ID)
}

func TestDeletingPostRemovesUnsharedImages(t *testing.T) {
	app, _, u, _ := newImageTestApp(t)
	coll, err := app.db.GetCollection("uploader")
	assert.NoError(t, err)

	rec, _ := doUpload(t, app, u, uploadRequest(t, "a.png", "image/png", tinyPNG(t)))
	imgID, url := uploadedURL(t, rec)

	title := "Only user"
	body := "![a](" + url + ")"
	post, err := app.db.CreatePost(u.ID, coll.ID, &SubmittedPost{Title: &title, Content: &body})
	assert.NoError(t, err)
	attachPostImages(app, u.ID, post.ID, body)

	// The post row goes first, exactly as the delete handler does it.
	_, err = app.db.Exec("DELETE FROM posts WHERE id = ?", post.ID)
	assert.NoError(t, err)
	cleanUpPostImages(app, post.ID)

	_, err = app.db.GetPostImage(imgID)
	assert.Error(t, err, "the image row must be gone")
	assert.Equal(t, 0, countUploadedFiles(t, app))
}

func TestDeletingPostKeepsSharedImages(t *testing.T) {
	app, _, u, _ := newImageTestApp(t)
	coll, err := app.db.GetCollection("uploader")
	assert.NoError(t, err)

	rec, _ := doUpload(t, app, u, uploadRequest(t, "a.png", "image/png", tinyPNG(t)))
	imgID, url := uploadedURL(t, rec)

	body := "![a](" + url + ")"
	t1 := "First"
	p1, err := app.db.CreatePost(u.ID, coll.ID, &SubmittedPost{Title: &t1, Content: &body})
	assert.NoError(t, err)
	t2 := "Second"
	_, err = app.db.CreatePost(u.ID, coll.ID, &SubmittedPost{Title: &t2, Content: &body})
	assert.NoError(t, err)
	attachPostImages(app, u.ID, p1.ID, body)

	_, err = app.db.Exec("DELETE FROM posts WHERE id = ?", p1.ID)
	assert.NoError(t, err)
	cleanUpPostImages(app, p1.ID)

	_, err = app.db.GetPostImage(imgID)
	assert.NoError(t, err, "an image another post still uses must survive")
	assert.Equal(t, 1, countUploadedFiles(t, app))
}

func TestOrphanSweepRemovesOldUnattachedImages(t *testing.T) {
	app, _, u, _ := newImageTestApp(t)
	_, _, post := createTemplateTestUser(t, app, "sweeper")

	rec, _ := doUpload(t, app, u, uploadRequest(t, "old.png", "image/png", tinyPNG(t)))
	oldID, _ := uploadedURL(t, rec)
	rec, _ = doUpload(t, app, u, uploadRequest(t, "new.jpg", "image/jpeg", tinyJPEG(t)))
	recentID, _ := uploadedURL(t, rec)
	rec, _ = doUpload(t, app, u, uploadRequest(t, "att.gif", "image/gif", animatedGIF(t)))
	attachedID, _ := uploadedURL(t, rec)
	assert.NoError(t, app.db.AttachImagesToPost(u.ID, post.ID, []string{attachedID}))

	assert.Equal(t, 3, countUploadedFiles(t, app))

	ageImage(t, app, oldID, 25)
	ageImage(t, app, recentID, 1)
	ageImage(t, app, attachedID, 25)

	sweepOrphanedImages(app)

	_, err := app.db.GetPostImage(oldID)
	assert.Error(t, err, "an abandoned draft upload must be swept")
	_, err = app.db.GetPostImage(recentID)
	assert.NoError(t, err, "a recent upload must be kept")
	_, err = app.db.GetPostImage(attachedID)
	assert.NoError(t, err, "an attached image must be kept")
	assert.Equal(t, 2, countUploadedFiles(t, app))
}

func TestDeletePostEndpointCleansUpImages(t *testing.T) {
	app, router, u, cookies := newImageTestApp(t)

	rec, _ := doUpload(t, app, u, uploadRequest(t, "a.png", "image/png", tinyPNG(t)))
	imgID, url := uploadedURL(t, rec)

	title := "Draft with an image"
	body := "![a](" + url + ")"
	post, err := app.db.CreatePost(u.ID, 0, &SubmittedPost{Title: &title, Content: &body})
	assert.NoError(t, err)
	attachPostImages(app, u.ID, post.ID, body)

	req := httptest.NewRequest("DELETE", "/api/posts/"+post.ID, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	del := httptest.NewRecorder()
	router.ServeHTTP(del, req)
	assert.Equal(t, http.StatusNoContent, del.Code, "body: %s", del.Body.String())

	_, err = app.db.GetPostImage(imgID)
	assert.Error(t, err, "deleting a post must clean up the images only it used")
	assert.Equal(t, 0, countUploadedFiles(t, app))
}

// TestImageUploadErrorsAreJSON asserts the upload endpoint answers a
// rejected upload with a JSON error and the real status code. Routed as a
// web handler it rendered the HTML error page with a 200, so the uploader
// JS could not tell success from failure.
func TestImageUploadErrorsAreJSON(t *testing.T) {
	_, router, _, cookies := newImageTestApp(t)

	// Load the editor to pick up a CSRF token and its cookie.
	padReq := httptest.NewRequest("GET", "/me/new", nil)
	for _, c := range cookies {
		padReq.AddCookie(c)
	}
	padRec := httptest.NewRecorder()
	router.ServeHTTP(padRec, padReq)
	m := regexp.MustCompile(`csrfToken: "([^"]*)"`).FindStringSubmatch(padRec.Body.String())
	assert.NotNil(t, m, "the editor must carry a CSRF token")
	if m == nil {
		return
	}

	req := uploadRequest(t, "evil.png", "image/png", []byte("<html><script>alert(1)</script></html>"))
	for _, c := range cookies {
		req.AddCookie(c)
	}
	for _, c := range padRec.Result().Cookies() {
		req.AddCookie(c)
	}
	req.Header.Set("X-CSRF-Token", m[1])
	req.Header.Set("Origin", "http://example.com")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code,
		"a rejected upload must carry its real status, not 200")
	assert.NotContains(t, rec.Body.String(), "<!DOCTYPE HTML",
		"the error must not be an HTML page")

	var body map[string]interface{}
	assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body),
		"the error body must be JSON: %s", rec.Body.String())
}
