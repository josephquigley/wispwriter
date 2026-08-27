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
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

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
