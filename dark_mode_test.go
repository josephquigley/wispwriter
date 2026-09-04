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
	"regexp"
	"strings"
	"testing"
)

// TestAppSurfacesCarryADarkModeHook covers the body ids dark.less scopes its
// overrides to. The rules deliberately never match a bare `body`, so a
// surface that stops naming itself silently loses dark mode rather than
// breaking visibly.
func TestAppSurfacesCarryADarkModeHook(t *testing.T) {
	app, router := newTemplateTestApp(t, nil)
	u, _, _ := createTemplateTestUser(t, app, "tester")
	cookie := loginCookie(t, app, u)

	cases := []struct {
		name   string
		path   string
		authed bool
		want   string
	}{
		{name: "me: posts list", path: "/me/posts/", authed: true, want: `<body id="me">`},
		{name: "me: settings", path: "/me/settings", authed: true, want: `<body id="me">`},
		{name: "login", path: "/login", want: `id="login"`},
		{name: "reset", path: "/reset", want: `id="login"`},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			var cookies []*http.Cookie
			if c.authed {
				cookies = []*http.Cookie{cookie}
			}
			rec := assertRendersCleanly(t, router, "GET", c.path, cookies, http.StatusOK)
			if !strings.Contains(rec.Body.String(), c.want) {
				t.Errorf("%s does not carry %s, so dark.less cannot reach it", c.path, c.want)
			}
		})
	}
}

// TestDarkModeStaysOffReaderFacingPages is the regression that matters. The
// application shares core.less with the published blog pages, so a dark rule
// that escapes its scope does not merely look wrong: it changes what every
// reader of every blog on the instance sees, and it silently overrides a
// blog's own Custom CSS. Every selector inside the media query must name an
// application body id.
func TestDarkModeStaysOffReaderFacingPages(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("less", "dark.less"))
	if err != nil {
		t.Fatalf("read dark.less: %v", err)
	}
	src := string(b)

	if !strings.Contains(src, "@media (prefers-color-scheme: dark)") {
		t.Fatal("dark.less has no prefers-color-scheme block, so it cannot follow the device")
	}

	// The single top-level selector inside the media query. Anything else at
	// that nesting level is a rule that escapes the application surfaces.
	openers := regexp.MustCompile(`(?m)^\t([^\t\n{}/][^{\n]*)\{`).FindAllStringSubmatch(src, -1)
	if len(openers) != 1 {
		t.Fatalf("expected exactly one selector inside the media query, found %d: %v", len(openers), openers)
	}
	got := strings.TrimSpace(openers[0][1])
	const want = "body#me, body#login"
	if got != want {
		t.Errorf("dark rules are scoped to %q, want %q; a wider selector reaches the Reader (read.tmpl sets id=collection) and published blogs", got, want)
	}
}

// TestDarkLessIsImportedLast pins the ordering the override layer depends on.
// It carries no extra specificity of its own, so it only wins by coming after
// the light rules.
func TestDarkLessIsImportedLast(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("less", "app.less"))
	if err != nil {
		t.Fatalf("read app.less: %v", err)
	}

	imports := regexp.MustCompile(`@import "([^"]+)"`).FindAllStringSubmatch(string(b), -1)
	if len(imports) == 0 {
		t.Fatal("app.less has no imports")
	}
	if last := imports[len(imports)-1][1]; last != "dark" {
		t.Errorf("app.less imports %q last, want \"dark\"; the override layer only wins on order", last)
	}
}
