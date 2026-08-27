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

	"github.com/writefreely/writefreely/config"
)

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
