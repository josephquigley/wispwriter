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

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"

	"github.com/writefreely/writefreely/config"
)

func TestSubscribeTogglesDefaultOn(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	_, coll, _ := createTemplateTestUser(t, app, "subdefaults")

	loaded, err := app.db.GetCollection(coll.Alias)
	assert.NoError(t, err)
	assert.True(t, loaded.ShowSubscribeIndex, "absent attribute means on")
	assert.True(t, loaded.ShowSubscribePosts, "absent attribute means on")
}

func TestSubscribeTogglesPersist(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	u, coll, _ := createTemplateTestUser(t, app, "subpersist")

	off := false
	on := true
	err := app.db.UpdateCollection(app, &SubmittedCollection{
		OwnerID:            uint64(u.ID),
		Alias:              &coll.Alias,
		ShowSubscribeIndex: &off,
		ShowSubscribePosts: &on,
	}, coll.Alias)
	assert.NoError(t, err)

	loaded, err := app.db.GetCollection(coll.Alias)
	assert.NoError(t, err)
	assert.False(t, loaded.ShowSubscribeIndex)
	assert.True(t, loaded.ShowSubscribePosts)
}

func TestOmittedSubscribeFieldsLeaveValuesAlone(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	u, coll, _ := createTemplateTestUser(t, app, "subomit")

	off := false
	err := app.db.UpdateCollection(app, &SubmittedCollection{
		OwnerID:            uint64(u.ID),
		Alias:              &coll.Alias,
		ShowSubscribeIndex: &off,
	}, coll.Alias)
	assert.NoError(t, err)

	// A later update that mentions neither field must not reset them.
	title := "Renamed"
	err = app.db.UpdateCollection(app, &SubmittedCollection{
		OwnerID: uint64(u.ID),
		Alias:   &coll.Alias,
		Title:   &title,
	}, coll.Alias)
	assert.NoError(t, err)

	loaded, err := app.db.GetCollection(coll.Alias)
	assert.NoError(t, err)
	assert.False(t, loaded.ShowSubscribeIndex, "an unrelated update must not flip the toggle")
	assert.True(t, loaded.ShowSubscribePosts)
}

// enableInstanceEmail configures the test instance so that
// config.EmailCfg.Enabled() reports true, which is what makes the email
// subscription settings available at all.
func enableInstanceEmail(cfg *config.Config) {
	cfg.Email.Domain = "example.com"
	cfg.Email.MailgunPrivate = "key-testing"
}

// newSubscribeTestApp builds a multi-user test instance -- config.New()
// defaults to single-user, where blogs aren't served under /{alias}/ -- with
// instance-wide email delivery configured. extra runs last, so a test can
// vary the configuration further.
func newSubscribeTestApp(t *testing.T, extra func(cfg *config.Config)) (*App, *mux.Router) {
	t.Helper()

	return newTemplateTestApp(t, func(cfg *config.Config) {
		cfg.App.SingleUser = false
		cfg.App.Chorus = false
		enableInstanceEmail(cfg)
		if extra != nil {
			extra(cfg)
		}
	})
}

// enableEmailSubs turns on email subscriptions for the given blog, which is
// what makes the "emailsubscribe" template block render at all. The title is
// resent unchanged only so the update has a column to write; without one,
// UpdateCollection rejects it as having nothing to update.
func enableEmailSubs(t *testing.T, app *App, u *User, coll *Collection) {
	t.Helper()

	title := coll.Title
	err := app.db.UpdateCollection(app, &SubmittedCollection{
		OwnerID:   uint64(u.ID),
		Alias:     &coll.Alias,
		Title:     &title,
		EmailSubs: true,
	}, coll.Alias)
	if err != nil {
		t.Fatalf("enable email subs for %q: %v", coll.Alias, err)
	}
}

// setSubscribeToggles updates the given blog's two subscribe placement
// attributes. It always re-asserts EmailSubs, because UpdateCollection
// deletes the email_subs attribute whenever that field is false.
func setSubscribeToggles(t *testing.T, app *App, u *User, coll *Collection, index, posts bool) {
	t.Helper()

	err := app.db.UpdateCollection(app, &SubmittedCollection{
		OwnerID:            uint64(u.ID),
		Alias:              &coll.Alias,
		EmailSubs:          true,
		ShowSubscribeIndex: &index,
		ShowSubscribePosts: &posts,
	}, coll.Alias)
	if err != nil {
		t.Fatalf("update subscribe toggles for %q: %v", coll.Alias, err)
	}
}

func TestIndexSubscribeButtonRespectsToggle(t *testing.T) {
	app, router := newSubscribeTestApp(t, nil)
	u, coll, _ := createTemplateTestUser(t, app, "indextoggle")

	enableEmailSubs(t, app, u, coll)

	rec, _ := renderedRequest(t, router, "GET", "/"+coll.Alias+"/", nil)
	assert.Contains(t, rec.Body.String(), `id="subscribe-btn"`, "on by default")

	setSubscribeToggles(t, app, u, coll, false, true)

	rec, _ = renderedRequest(t, router, "GET", "/"+coll.Alias+"/", nil)
	assert.NotContains(t, rec.Body.String(), `id="subscribe-btn"`, "hidden when the toggle is off")
}

func TestPostPageSubscribeButtonRespectsToggle(t *testing.T) {
	app, router := newSubscribeTestApp(t, nil)
	u, coll, post := createTemplateTestUser(t, app, "posttoggle")

	enableEmailSubs(t, app, u, coll)
	path := "/" + coll.Alias + "/" + post.Slug.String

	rec, _ := renderedRequest(t, router, "GET", path, nil)
	assert.Contains(t, rec.Body.String(), `id="subscribe-btn"`, "on by default")

	setSubscribeToggles(t, app, u, coll, true, false)

	rec, _ = renderedRequest(t, router, "GET", path, nil)
	assert.NotContains(t, rec.Body.String(), `id="subscribe-btn"`, "hidden when the toggle is off")
}

func TestChorusPostPageSubscribeButtonRespectsToggle(t *testing.T) {
	app, router := newSubscribeTestApp(t, func(cfg *config.Config) {
		cfg.App.Chorus = true
	})
	u, coll, post := createTemplateTestUser(t, app, "chorustoggle")

	enableEmailSubs(t, app, u, coll)
	path := "/" + coll.Alias + "/" + post.Slug.String

	rec, _ := renderedRequest(t, router, "GET", path, nil)
	assert.Contains(t, rec.Body.String(), `id="subscribe-btn"`, "on by default")

	setSubscribeToggles(t, app, u, coll, true, false)

	rec, _ = renderedRequest(t, router, "GET", path, nil)
	assert.NotContains(t, rec.Body.String(), `id="subscribe-btn"`, "hidden when the toggle is off")
}
