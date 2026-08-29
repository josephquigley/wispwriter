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
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/writefreely/writefreely/config"
	"github.com/writefreely/writefreely/key"
)

func TestServesPlaintextHTTP(t *testing.T) {
	cases := map[string]bool{
		"http://localhost:8080": true,
		"HTTP://localhost:8080": true,
		"https://example.com":   false,
		"HTTPS://example.com":   false,
		"":                      false,
	}
	for host, want := range cases {
		cfg := config.New()
		cfg.App.Host = host
		app := &App{cfg: cfg}
		assert.Equal(t, want, app.servesPlaintextHTTP(), "host %q", host)
	}
}

func TestCSRFOptionsRelaxSecureOnlyForPlaintextSites(t *testing.T) {
	// csrf.Path is always present. csrf.Secure(false) is added only for a
	// plain-HTTP site, because a Secure cookie is never returned over http
	// and the token would never reach the server.
	plaintext := config.New()
	plaintext.App.Host = "http://localhost:8080"
	assert.Len(t, csrfOptions(&App{cfg: plaintext}), 2)

	tls := config.New()
	tls.App.Host = "https://example.com"
	assert.Len(t, csrfOptions(&App{cfg: tls}), 1)
}

// TestCSRFProtectHandlerServesThrough builds the handler and serves a
// request through it, so a wrapper that fails to reach the middleware --
// or reaches itself -- is caught here rather than at run time.
func TestCSRFProtectHandlerServesThrough(t *testing.T) {
	for _, host := range []string{"http://localhost:8080", "https://example.com"} {
		cfg := config.New()
		cfg.App.Host = host
		app := &App{cfg: cfg, keys: &key.Keychain{}}
		assert.NoError(t, app.keys.GenerateKeys())

		called := false
		h := csrfProtectHandler(app, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/me/settings", nil))
		assert.True(t, called, "host %q: the wrapped handler must run", host)
		assert.Equal(t, http.StatusOK, rec.Code, "host %q", host)
	}
}

// TestCSRFRejectsForgedPostOverPlaintext confirms relaxing the Referer
// rule for plain HTTP does not disable CSRF protection itself: a POST with
// no token is still refused.
func TestCSRFRejectsForgedPostOverPlaintext(t *testing.T) {
	cfg := config.New()
	cfg.App.Host = "http://localhost:8080"
	app := &App{cfg: cfg, keys: &key.Keychain{}}
	assert.NoError(t, app.keys.GenerateKeys())

	h := csrfProtectHandler(app, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/me/settings", nil))
	assert.Equal(t, http.StatusForbidden, rec.Code, "a POST without a token must still be rejected")
}
