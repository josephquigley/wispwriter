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
		"":                      false,
	}
	for host, want := range cases {
		cfg := config.New()
		cfg.App.Host = host
		app := &App{cfg: cfg}
		assert.Equal(t, want, app.servesPlaintextHTTP(), "host %q", host)
	}
}

func TestCSRFOptionsRelaxSecureOnlyForPlainHTTPSites(t *testing.T) {
	cases := []struct {
		name        string
		host        string
		wantRelaxed bool
	}{
		{"plain http site", "http://localhost:8080", true},
		{"uppercase scheme", "HTTP://localhost:8080", true},
		{"https site", "https://example.com", false},
		{"https site behind a proxy", "https://blog.example.com", false},
		{"unset host", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := config.New()
			cfg.App.Host = c.host
			app := &App{cfg: cfg}

			opts := csrfOptions(app)
			// csrf.Path is always present; csrf.Secure(false) is the
			// second option and only added for plain-HTTP sites.
			if c.wantRelaxed {
				assert.Len(t, opts, 2, "plain-HTTP sites must relax the secure-cookie/Referer check")
			} else {
				assert.Len(t, opts, 1, "TLS sites must keep the strict check")
			}
		})
	}
}

// TestCSRFProtectHandlerDoesNotRecurse builds the handler and serves a
// request through it. A self-referential wrapper compiles cleanly and only
// fails at run time with a stack overflow, which is how this regressed
// once already.
func TestCSRFProtectHandlerDoesNotRecurse(t *testing.T) {
	cfg := config.New()
	cfg.App.Host = "http://localhost:8080"
	app := &App{cfg: cfg, keys: &key.Keychain{}}
	assert.NoError(t, app.keys.GenerateKeys())

	called := false
	h := csrfProtectHandler(app, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/me/settings", nil))
	assert.True(t, called, "the wrapped handler must actually run")
	assert.Equal(t, http.StatusOK, rec.Code)
}
