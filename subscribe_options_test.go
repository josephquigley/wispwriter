/*
 * Copyright © 2026 Joseph Quigley.
 *
 * This file is part of WriteFreely.
 *
 * WriteFreely is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License, included
 * in the LICENSE file in this source code package.
 */

package writefreely

import (
	"net/http"
	"net/url"
	"strings"
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

func TestSubscribeTogglesAreIndependent(t *testing.T) {
	cases := []struct {
		name         string
		index, posts bool
	}{
		{"both on", true, true},
		{"index only", true, false},
		{"posts only", false, true},
		{"both off", false, false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			app, router := newSubscribeTestApp(t, nil)
			u, coll, post := createTemplateTestUser(t, app, "indep"+strings.Replace(c.name, " ", "", -1))

			enableEmailSubs(t, app, u, coll)
			setSubscribeToggles(t, app, u, coll, c.index, c.posts)

			rec, _ := renderedRequest(t, router, "GET", "/"+coll.Alias+"/", nil)
			assert.Equal(t, c.index, strings.Contains(rec.Body.String(), `id="subscribe-btn"`),
				"index page follows show_subscribe_index alone")

			rec, _ = renderedRequest(t, router, "GET", "/"+coll.Alias+"/"+post.Slug.String, nil)
			assert.Equal(t, c.posts, strings.Contains(rec.Body.String(), `id="subscribe-btn"`),
				"post page follows show_subscribe_posts alone")
		})
	}
}

func TestNoSubscribeButtonWhenEmailSubscriptionsDisabled(t *testing.T) {
	app, router := newSubscribeTestApp(t, func(cfg *config.Config) {
		cfg.Email.Domain = ""
		cfg.Email.MailgunPrivate = ""
	})
	assert.False(t, app.cfg.Email.Enabled(), "instance email delivery is off for this test")

	u, coll, post := createTemplateTestUser(t, app, "nosubs")

	on := true
	err := app.db.UpdateCollection(app, &SubmittedCollection{
		OwnerID:            uint64(u.ID),
		Alias:              &coll.Alias,
		ShowSubscribeIndex: &on,
		ShowSubscribePosts: &on,
	}, coll.Alias)
	assert.NoError(t, err)

	rec, _ := renderedRequest(t, router, "GET", "/"+coll.Alias+"/", nil)
	assert.NotContains(t, rec.Body.String(), `id="subscribe-btn"`, "no index form")

	rec, _ = renderedRequest(t, router, "GET", "/"+coll.Alias+"/"+post.Slug.String, nil)
	assert.NotContains(t, rec.Body.String(), `id="subscribe-btn"`, "no post-page form")

	cookies := []*http.Cookie{loginCookie(t, app, u)}
	rec, _ = renderedRequest(t, router, "GET", "/me/c/"+coll.Alias, cookies)
	assert.NotContains(t, rec.Body.String(), `name="show_subscribe_index"`, "no settings block")
	assert.NotContains(t, rec.Body.String(), `name="show_subscribe_posts"`, "no settings block")
}

func TestTogglingDoesNotAffectSubscribers(t *testing.T) {
	app, _ := newSubscribeTestApp(t, nil)
	u, coll, _ := createTemplateTestUser(t, app, "keepsubs")

	enableEmailSubs(t, app, u, coll)
	_, err := app.db.AddEmailSubscription(coll.ID, 0, "reader@example.com", true)
	assert.NoError(t, err)

	before, err := app.db.GetEmailSubscribers(coll.ID, true)
	assert.NoError(t, err)
	assert.Len(t, before, 1)

	setSubscribeToggles(t, app, u, coll, false, false)
	setSubscribeToggles(t, app, u, coll, true, true)

	after, err := app.db.GetEmailSubscribers(coll.ID, true)
	assert.NoError(t, err)
	assert.Len(t, after, 1, "toggling placement must not touch the subscriber list")
	assert.Equal(t, before[0].Email, after[0].Email)
}

func TestSettingsShowsSubscribeToggles(t *testing.T) {
	app, router := newSubscribeTestApp(t, nil)
	u, coll, _ := createTemplateTestUser(t, app, "subsettings")

	enableEmailSubs(t, app, u, coll)

	cookies := []*http.Cookie{loginCookie(t, app, u)}
	rec, _ := renderedRequest(t, router, "GET", "/me/c/"+coll.Alias, cookies)
	assert.Contains(t, rec.Body.String(), `name="show_subscribe_index"`)
	assert.Contains(t, rec.Body.String(), `name="show_subscribe_posts"`)
}

func TestUncheckedToggleStoresFalse(t *testing.T) {
	app, router := newSubscribeTestApp(t, nil)
	u, coll, _ := createTemplateTestUser(t, app, "uncheckedtoggle")

	enableEmailSubs(t, app, u, coll)
	setSubscribeToggles(t, app, u, coll, true, true)

	cookies := []*http.Cookie{loginCookie(t, app, u)}
	// This is exactly what the customization form submits: the hidden input
	// always, and the checkbox's own value only when it's checked. The index
	// box is unchecked here, the posts box is checked.
	rec := postForm(t, router, "/api/collections/"+coll.Alias, cookies, url.Values{
		"web":                  {"1"},
		"title":                {coll.Title},
		"email_subs":           {"on"},
		"show_subscribe_index": {"false"},
		"show_subscribe_posts": {"false", "true"},
	})
	assert.Less(t, rec.Code, 400, "form submission succeeded")

	loaded, err := app.db.GetCollection(coll.Alias)
	assert.NoError(t, err)
	assert.False(t, loaded.ShowSubscribeIndex, "an unchecked box must store false, not be ignored")
	assert.True(t, loaded.ShowSubscribePosts, "a checked box must win over its hidden default")
}

// TestSettingsHidesSubscribeTogglesWhenBlogHasSubsOff covers the gate on
// the settings block. With email subscriptions off for this blog neither
// placement renders whatever the toggles say, so offering them is
// offering a setting that does nothing.
func TestSettingsHidesSubscribeTogglesWhenBlogHasSubsOff(t *testing.T) {
	app, router := newSubscribeTestApp(t, nil)
	u, coll, _ := createTemplateTestUser(t, app, "subsoffsettings")
	cookies := []*http.Cookie{loginCookie(t, app, u)}

	// email_subs is off by default for a new blog.
	rec, _ := renderedRequest(t, router, "GET", "/me/c/"+coll.Alias, cookies)
	assert.NotContains(t, rec.Body.String(), `name="show_subscribe_index"`,
		"the placement toggles do nothing while the blog has subscriptions off")

	enableEmailSubs(t, app, u, coll)

	rec, _ = renderedRequest(t, router, "GET", "/me/c/"+coll.Alias, cookies)
	assert.Contains(t, rec.Body.String(), `name="show_subscribe_index"`)
	assert.Contains(t, rec.Body.String(), `name="show_subscribe_posts"`)
}
