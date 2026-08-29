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
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"github.com/writefreely/writefreely/config"
)

// On a single-user instance the one collection is served at the site root, so
// every URL that points into it is "/<slug>", not "/<alias>/<slug>". Owners
// are the only visitors who see pin / unpin / delete / subscribe links, which
// is why these paths went unguarded for so long. Each test here renders the
// same page in both modes and asserts the link that mode should produce, so a
// fix for one mode can't silently break the other.

// singleUserModeApp builds a test app in either single-user or multi-user
// mode. It also mirrors Serve()'s isSingleUser assignment, which the rest of
// the package reads to build canonical URLs.
func singleUserModeApp(t *testing.T, singleUser, chorus bool) (*App, *mux.Router) {
	t.Helper()

	app, router := newTemplateTestApp(t, func(cfg *config.Config) {
		cfg.App.SingleUser = singleUser
		cfg.App.Chorus = chorus
	})

	prev := isSingleUser
	isSingleUser = singleUser
	t.Cleanup(func() { isSingleUser = prev })

	return app, router
}

// blogPath returns the path the given collection is served at, i.e. "/" on a
// single-user instance and "/<alias>/" everywhere else.
func blogPath(singleUser bool, alias string) string {
	if singleUser {
		return "/"
	}
	return "/" + alias + "/"
}

func assertContains(t *testing.T, body, want, context string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Errorf("%s: response doesn't contain %q", context, want)
	}
}

func assertNotContains(t *testing.T, body, unwanted, context string) {
	t.Helper()
	if strings.Contains(body, unwanted) {
		t.Errorf("%s: response contains %q, which doesn't resolve in this mode", context, unwanted)
	}
}

// TestEmailSubscriptionRedirectsToPost checks that subscribing through the web
// form sends the blog's owner back to the post they subscribed from, in both
// single-user and multi-user mode.
func TestEmailSubscriptionRedirectsToPost(t *testing.T) {
	for _, tc := range []struct {
		name       string
		singleUser bool
	}{
		{name: "SingleUser", singleUser: true},
		{name: "MultiUser", singleUser: false},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			app, router := singleUserModeApp(t, tc.singleUser, false)
			u, coll, post := createTemplateTestUser(t, app, "tester")
			cookie := loginCookie(t, app, u)

			// Pre-confirm this address against an unrelated collection so
			// the handler doesn't try to send a confirmation email, which
			// would need a mailer we don't have in tests.
			email := "owner@example.com"
			if _, err := app.db.AddEmailSubscription(9999, 0, email, true); err != nil {
				t.Fatalf("seed confirmed subscriber: %v", err)
			}

			form := url.Values{}
			form.Set("email", email)
			form.Set("web", "true")
			form.Set("slug", post.Slug.String)

			req := httptest.NewRequest("POST", "/api/collections/"+coll.Alias+"/email/subscribe", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.AddCookie(cookie)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusFound {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusFound, truncateForLog(rec.Body.String()))
			}
			want := blogPath(tc.singleUser, coll.Alias) + post.Slug.String
			if tc.singleUser {
				want = app.cfg.App.Host + want
			}
			if got := rec.Header().Get("Location"); got != want {
				t.Errorf("owner redirected to %q, want %q", got, want)
			}
		})
	}
}

// TestEmailUnsubscribeRedirectsToPost checks the same thing for the web
// unsubscribe link.
func TestEmailUnsubscribeRedirectsToPost(t *testing.T) {
	for _, tc := range []struct {
		name       string
		singleUser bool
	}{
		{name: "SingleUser", singleUser: true},
		{name: "MultiUser", singleUser: false},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			app, router := singleUserModeApp(t, tc.singleUser, false)
			u, coll, post := createTemplateTestUser(t, app, "tester")
			cookie := loginCookie(t, app, u)

			if _, err := app.db.AddEmailSubscription(coll.ID, u.ID, "", true); err != nil {
				t.Fatalf("seed subscriber: %v", err)
			}

			path := "/api/collections/" + coll.Alias + "/email/unsubscribe?slug=" + post.Slug.String
			rec, _ := renderedRequest(t, router, "GET", path, []*http.Cookie{cookie})

			if rec.Code != http.StatusFound {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusFound, truncateForLog(rec.Body.String()))
			}
			want := blogPath(tc.singleUser, coll.Alias) + post.Slug.String
			if got := rec.Header().Get("Location"); got != want {
				t.Errorf("owner redirected to %q, want %q", got, want)
			}
		})
	}
}

// TestOwnerPostActionLinks checks the pin and delete links the owner sees on
// their blog's index, and the unpin link on a pinned post.
func TestOwnerPostActionLinks(t *testing.T) {
	for _, tc := range []struct {
		name       string
		singleUser bool
		chorus     bool
	}{
		{name: "SingleUser", singleUser: true},
		{name: "SingleUser_ChorusOn", singleUser: true, chorus: true},
		{name: "MultiUser", singleUser: false},
		{name: "MultiUser_ChorusOn", singleUser: false, chorus: true},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			app, router := singleUserModeApp(t, tc.singleUser, tc.chorus)
			u, coll, post := createTemplateTestUser(t, app, "tester")
			cookie := loginCookie(t, app, u)
			cookies := []*http.Cookie{cookie}

			base := blogPath(tc.singleUser, coll.Alias)
			wrong := blogPath(!tc.singleUser, coll.Alias)

			// Blog index: pin and delete links.
			rec := assertRendersCleanly(t, router, "GET", base, cookies, http.StatusOK)
			body := rec.Body.String()
			for _, action := range []string{"pin", "delete"} {
				assertContains(t, body, `href="`+base+post.Slug.String+"/"+action+`"`, "blog index "+action+" link")
				assertNotContains(t, body, `href="`+wrong+post.Slug.String+"/"+action+`"`, "blog index "+action+" link")
			}

			// Post page: unpin link, which only shows on a pinned post.
			if err := app.db.UpdatePostPinState(true, post.ID, coll.ID, u.ID, 1); err != nil {
				t.Fatalf("pin post: %v", err)
			}
			rec = assertRendersCleanly(t, router, "GET", base+post.Slug.String, cookies, http.StatusOK)
			body = rec.Body.String()
			assertContains(t, body, `href="`+base+post.Slug.String+`/unpin"`, "post page unpin link")
			assertNotContains(t, body, `href="`+wrong+post.Slug.String+`/unpin"`, "post page unpin link")
		})
	}
}

// TestUntitledPostViewLink checks the "view" link shown on the blog index for
// an untitled post, which only readers other than the owner ever see.
func TestUntitledPostViewLink(t *testing.T) {
	for _, tc := range []struct {
		name       string
		singleUser bool
	}{
		{name: "SingleUser", singleUser: true},
		{name: "MultiUser", singleUser: false},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			app, router := singleUserModeApp(t, tc.singleUser, false)
			u, coll, _ := createTemplateTestUser(t, app, "tester")
			coll.hostName = app.cfg.App.Host

			title := ""
			content := "An untitled post, so the blog index links to it with a *view* link."
			post, err := app.db.CreatePost(u.ID, coll.ID, &SubmittedPost{Title: &title, Content: &content})
			if err != nil {
				t.Fatalf("create untitled post: %v", err)
			}

			// The link only appears on a blog that doesn't show dates, and
			// only on posts old enough for anonymous readers to see.
			if _, err := app.db.Exec("UPDATE collections SET format = ? WHERE id = ?", "novel", coll.ID); err != nil {
				t.Fatalf("set collection format: %v", err)
			}
			if _, err := app.db.Exec("UPDATE posts SET created = ? WHERE collection_id = ?", "2020-01-01 00:00:00", coll.ID); err != nil {
				t.Fatalf("backdate posts: %v", err)
			}

			rec := assertRendersCleanly(t, router, "GET", blogPath(tc.singleUser, coll.Alias), nil, http.StatusOK)
			assertContains(t, rec.Body.String(), `href="`+coll.CanonicalURL()+post.Slug.String+`">view</a>`, "untitled post view link")
		})
	}
}

// TestOwnerBlogLinks checks the links to the blog itself on the account's blog
// list, the admin's user view, and the password prompt of a protected blog.
func TestOwnerBlogLinks(t *testing.T) {
	for _, tc := range []struct {
		name       string
		singleUser bool
	}{
		{name: "SingleUser", singleUser: true},
		{name: "MultiUser", singleUser: false},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			app, router := singleUserModeApp(t, tc.singleUser, false)
			u, coll, _ := createTemplateTestUser(t, app, "tester")
			cookies := []*http.Cookie{loginCookie(t, app, u)}

			base := blogPath(tc.singleUser, coll.Alias)
			wrong := blogPath(!tc.singleUser, coll.Alias)

			// Account's list of blogs.
			rec := assertRendersCleanly(t, router, "GET", "/me/c/", cookies, http.StatusOK)
			body := rec.Body.String()
			assertContains(t, body, `class="title" href="`+base+`"`, "blogs list link")
			assertNotContains(t, body, `class="title" href="`+wrong+`"`, "blogs list link")

			// Admin's view of the user. The first user is the admin.
			rec = assertRendersCleanly(t, router, "GET", "/admin/user/"+u.Username, cookies, http.StatusOK)
			body = rec.Body.String()
			assertContains(t, body, `<h3><a href="`+base+`">`, "admin user view blog link")
			assertNotContains(t, body, `<h3><a href="`+wrong+`">`, "admin user view blog link")

			// Password prompt shown to unauthorized visitors of a protected blog.
			if _, err := app.db.Exec("UPDATE collections SET privacy = ? WHERE id = ?", CollProtected, coll.ID); err != nil {
				t.Fatalf("protect collection: %v", err)
			}
			if _, err := app.db.Exec("INSERT INTO collectionpasswords (collection_id, password) VALUES (?, ?)", coll.ID, "secret"); err != nil {
				t.Fatalf("set collection password: %v", err)
			}
			rec = assertRendersCleanly(t, router, "GET", base, nil, http.StatusOK)
			body = rec.Body.String()
			assertContains(t, body, `id="blog-title"><a href="`+base+`"`, "password prompt blog title link")
			assertNotContains(t, body, `id="blog-title"><a href="`+wrong+`"`, "password prompt blog title link")
		})
	}
}
