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
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

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

// allowlistApp returns a private App with the given allowlist configured.
func allowlistApp(t *testing.T, list string) *App {
	t.Helper()
	cfg := config.New()
	cfg.App.Host = "https://local.example"
	cfg.App.Private = true
	cfg.App.FederationAllowlist = list
	app := &App{cfg: cfg}
	if err := app.initFederationAllowlist(); err != nil {
		t.Fatalf("initFederationAllowlist: %v", err)
	}
	return app
}

func TestFederationAllowedWithNoList(t *testing.T) {
	// No allowlist means the instance behaves as it always has: every host
	// is allowed. This is the compatibility guarantee.
	app := allowlistApp(t, "")
	assert.False(t, app.federationAllowlistActive())
	assert.True(t, app.federationAllowed("anything.example"))
}

func TestFederationAllowedMatchesExactHost(t *testing.T) {
	app := allowlistApp(t, "example.org, mastodon.social")

	assert.True(t, app.federationAllowlistActive())
	assert.True(t, app.federationAllowed("example.org"))
	assert.True(t, app.federationAllowed("EXAMPLE.ORG"))
	assert.True(t, app.federationAllowed("mastodon.social"))
	assert.False(t, app.federationAllowed("evil.example.org"))
	assert.False(t, app.federationAllowed("example.org.evil.com"))
	assert.False(t, app.federationAllowed("other.example"))
	assert.False(t, app.federationAllowed(""))
}

func TestInboxAllowed(t *testing.T) {
	app := allowlistApp(t, "example.org")

	assert.True(t, app.inboxAllowed("https://example.org/inbox"))
	assert.True(t, app.inboxAllowed("https://Example.org:443/users/a/inbox"))
	assert.False(t, app.inboxAllowed("https://evil.example.org/inbox"))
	assert.False(t, app.inboxAllowed("://not a url"))
	assert.False(t, app.inboxAllowed(""))

	open := allowlistApp(t, "")
	assert.True(t, open.inboxAllowed("https://anything.example/inbox"))
}

// testKey generates an RSA key for signing and verifying in tests.
func testKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return k
}

func TestKeyCacheStoresAndExpires(t *testing.T) {
	c := newKeyCache()
	pub := &testKey(t).PublicKey

	_, found := c.get("https://example.org/users/a#main-key")
	assert.False(t, found)

	c.set("https://example.org/users/a#main-key", pub, time.Minute)
	got, found := c.get("https://example.org/users/a#main-key")
	assert.True(t, found)
	assert.Equal(t, pub, got)

	c.set("https://example.org/users/a#main-key", pub, -time.Minute)
	_, found = c.get("https://example.org/users/a#main-key")
	assert.False(t, found, "an expired entry must not be found")
}

func TestKeyCacheRemembersFailure(t *testing.T) {
	// A nil key records a failed fetch, so an unreachable peer does not make
	// every request wait on a network timeout.
	c := newKeyCache()
	c.set("https://example.org/users/a#main-key", nil, time.Minute)

	got, found := c.get("https://example.org/users/a#main-key")
	assert.True(t, found)
	assert.Nil(t, got)
}

func TestAllowlistedKeyReturnsCachedFailureWithoutFetching(t *testing.T) {
	app := allowlistApp(t, "example.org")
	app.fedKeys.set("https://example.org/users/a#main-key", nil, time.Minute)

	_, err := app.allowlistedKey("https://example.org/users/a#main-key")
	assert.Error(t, err)
}

func TestAllowlistedKeyReturnsCachedKey(t *testing.T) {
	app := allowlistApp(t, "example.org")
	pub := &testKey(t).PublicKey
	app.fedKeys.set("https://example.org/users/a#main-key", pub, time.Minute)

	got, err := app.allowlistedKey("https://example.org/users/a#main-key")
	assert.NoError(t, err)
	assert.Equal(t, pub, got)
}
