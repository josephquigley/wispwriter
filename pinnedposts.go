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
	"html/template"
	"net/http"

	"github.com/gorilla/csrf"
	"github.com/gorilla/mux"
)

// pinnedPostsPage is the view data for the pinned post management page.
type pinnedPostsPage struct {
	*UserPage
	Collection  *Collection
	Alias       string
	SingleUser  bool
	CSRFField   template.HTML
	PinnedPosts []PublicPost

	// LastIndex is the index of the final pinned post, so the template can
	// tell which row should omit its "move down" control without doing
	// arithmetic.
	LastIndex int
}

// ownedPinnedCollection resolves the {collection} route variable and returns
// it only if the given user owns it. Someone else's blog gets the same "not
// found" error a nonexistent one does, so the response doesn't confirm that
// the blog exists.
func ownedPinnedCollection(app *App, u *User, r *http.Request) (*Collection, error) {
	c, err := app.db.GetCollection(mux.Vars(r)["collection"])
	if err != nil {
		return nil, err
	}
	if c.OwnerID != u.ID {
		return nil, ErrCollectionNotFound
	}
	c.hostName = app.cfg.App.Host
	return c, nil
}

// viewPinnedPosts renders the pinned posts of one of the user's blogs, in
// navigation order, with controls to reorder and unpin them.
func viewPinnedPosts(app *App, u *User, w http.ResponseWriter, r *http.Request) error {
	c, err := ownedPinnedCollection(app, u, r)
	if err != nil {
		return err
	}

	// Repair any drift before reading, so the order shown matches the order
	// the move controls will operate on.
	if err := app.db.NormalizePinnedPositions(c.ID); err != nil {
		return err
	}

	posts, err := app.db.GetPinnedPosts(&CollectionObj{Collection: *c}, true)
	if err != nil {
		return err
	}

	flashes, _ := getSessionFlashes(app, w, r, nil)

	p := pinnedPostsPage{
		UserPage:    NewUserPage(app, r, u, c.DisplayTitle()+" Pinned Posts", flashes),
		Collection:  c,
		Alias:       c.Alias,
		SingleUser:  app.cfg.App.SingleUser,
		CSRFField:   csrf.TemplateField(r),
		PinnedPosts: *posts,
		LastIndex:   len(*posts) - 1,
	}
	showUserPage(w, "collection-pinned", p)
	return nil
}
