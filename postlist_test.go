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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCollectionPostsForOwner(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	u, coll, _ := createTemplateTestUser(t, app, "listowner")

	// createTemplateTestUser leaves one post on the collection.
	posts, total, err := app.db.GetCollectionPostsForOwner(coll.ID, 1)
	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, *posts, 1)
	assert.NotZero(t, u.ID)

	// Bodies must not be loaded -- this view never renders content.
	assert.Empty(t, (*posts)[0].Content)
}

func TestGetCollectionPostsForOwnerPaging(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	u, coll, _ := createTemplateTestUser(t, app, "pager")

	title := "post"
	content := "body"
	for i := 0; i < postListPageSize+5; i++ {
		_, err := app.db.CreatePost(u.ID, coll.ID, &SubmittedPost{Title: &title, Content: &content})
		assert.NoError(t, err)
	}

	first, total, err := app.db.GetCollectionPostsForOwner(coll.ID, 1)
	assert.NoError(t, err)
	assert.Equal(t, postListPageSize+6, total) // +1 for the user's initial post
	assert.Len(t, *first, postListPageSize)

	second, _, err := app.db.GetCollectionPostsForOwner(coll.ID, 2)
	assert.NoError(t, err)
	assert.Len(t, *second, 6)
}
