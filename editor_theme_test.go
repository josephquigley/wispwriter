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
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/writefreely/writefreely/config"
)

// TestEditorSurfacesFollowSystemTheme covers the three templates that share
// the padTheme preference. All of them must treat an unset preference as
// "follow the OS", because that is what the pad stores: it persists only an
// explicit toggle, so an author who has never picked a theme has no key at
// all. A surface defaulting to light instead leaves that author with a dark
// pad and a light metadata editor in the same sitting.
func TestEditorSurfacesFollowSystemTheme(t *testing.T) {
	cases := []struct {
		name   string
		editor string // [app] editor, empty for the default pad
		path   func(blogPrefix, slug string) string
	}{
		{
			name: "pad",
			path: func(string, string) string { return "/me/new" },
		},
		{
			name:   "classic",
			editor: "classic",
			path:   func(string, string) string { return "/me/new" },
		},
		{
			name: "edit-meta",
			path: func(blogPrefix, slug string) string {
				return blogPrefix + "/" + slug + "/edit/meta"
			},
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			app, router := newTemplateTestApp(t, func(cfg *config.Config) {
				cfg.App.SingleUser = true
				cfg.App.Editor = c.editor
			})
			u, _, post := createTemplateTestUser(t, app, "tester")
			cookie := loginCookie(t, app, u)

			slug := post.Slug.String
			if slug == "" {
				slug = post.ID
			}

			rec := assertRendersCleanly(t, router, "GET", c.path("", slug),
				[]*http.Cookie{cookie}, http.StatusOK)
			body := rec.Body.String()

			if !strings.Contains(body, `src="/js/theme.js"`) {
				t.Errorf("%s does not load /js/theme.js, so it resolves the shared padTheme preference on its own", c.name)
			}
			if strings.Contains(body, "function toggleTheme()") {
				t.Errorf("%s still carries its own copy of the theme code, which is how the three surfaces drifted apart", c.name)
			}
		})
	}
}

// TestSharedThemeScriptFollowsTheSystem pins the behaviour the editor
// surfaces delegate to: an unset preference resolves from the operating
// system, and only an explicit toggle is stored. Defaulting the stored value
// to "light" instead, as edit-meta.tmpl and classic.tmpl did, forces light on
// every author who has never touched the toggle.
func TestSharedThemeScriptFollowsTheSystem(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("static", "js", "theme.js"))
	if err != nil {
		t.Fatalf("read theme.js: %v", err)
	}
	js := string(b)

	if !strings.Contains(js, "prefers-color-scheme") {
		t.Error("theme.js does not consult prefers-color-scheme, so an unset preference cannot follow the OS")
	}
	if !strings.Contains(js, `H.get('padTheme', 'auto')`) {
		t.Error("theme.js does not default the stored preference to 'auto'")
	}
	if strings.Contains(js, `H.get('padTheme', 'light')`) {
		t.Error("theme.js defaults the stored preference to 'light', which forces light on an author who never toggled")
	}
	// Storing on the resolve path is the defect the pad had: it replaces the
	// unset preference on first load, and the OS is never consulted again.
	if strings.Count(js, "H.set('padTheme'") != 1 {
		t.Error("theme.js should store padTheme in exactly one place, the explicit toggle")
	}
}
