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

func TestParseFederationAllowlist(t *testing.T) {
	cases := map[string]map[string]bool{
		"":                               {},
		"   ":                            {},
		",,":                             {},
		"example.org":                    {"example.org": true},
		" Example.ORG , mastodon.social": {"example.org": true, "mastodon.social": true},
		"example.org,":                   {"example.org": true},
	}
	for in, want := range cases {
		assert.Equal(t, want, parseFederationAllowlist(in), "input %q", in)
	}
}

func TestInitFederationAllowlistRequiresPrivateMode(t *testing.T) {
	cfg := config.New()
	cfg.App.Private = false
	cfg.App.FederationAllowlist = "example.org"
	app := &App{cfg: cfg}

	err := app.initFederationAllowlist()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private")
}

func TestInitFederationAllowlistAcceptsPrivateMode(t *testing.T) {
	cfg := config.New()
	cfg.App.Private = true
	cfg.App.FederationAllowlist = "example.org"
	app := &App{cfg: cfg}

	assert.NoError(t, app.initFederationAllowlist())
	assert.Equal(t, map[string]bool{"example.org": true}, app.fedAllowlist)
	assert.NotNil(t, app.fedKeys)
}

func TestInitFederationAllowlistAllowsPublicInstanceWithNoList(t *testing.T) {
	// A whitespace-only value parses to an empty set, so it must not trip
	// the private-mode requirement.
	cfg := config.New()
	cfg.App.Private = false
	cfg.App.FederationAllowlist = " , "
	app := &App{cfg: cfg}

	assert.NoError(t, app.initFederationAllowlist())
	assert.Empty(t, app.fedAllowlist)
}
