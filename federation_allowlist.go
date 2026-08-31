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
	"fmt"
	"net/url"
	"strings"

	"github.com/writeas/web-core/log"
)

// parseFederationAllowlist turns a comma-separated list of hostnames into a
// set. Entries are trimmed and lowercased, and empty entries are dropped, so
// a value of only whitespace or commas yields an empty set.
func parseFederationAllowlist(s string) map[string]bool {
	allowed := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		host := strings.ToLower(strings.TrimSpace(part))
		if host != "" {
			allowed[host] = true
		}
	}
	return allowed
}

// initFederationAllowlist parses the configured allowlist into the App and
// prepares the key cache. It fails when an allowlist is configured on an
// instance that is not private, so an operator never ends up with an
// allowlist they believe is in force over content that is in fact public.
func (app *App) initFederationAllowlist() error {
	app.fedAllowlist = parseFederationAllowlist(app.cfg.App.FederationAllowlist)
	if len(app.fedAllowlist) > 0 && !app.cfg.App.Private {
		return fmt.Errorf("federation_allowlist requires private = true: refusing to run an allowlist on a public instance")
	}
	if app.fedKeys == nil {
		app.fedKeys = newKeyCache()
	}
	return nil
}

// keyCache caches remote public keys used for signature verification.
type keyCache struct{}

// newKeyCache returns an empty key cache.
func newKeyCache() *keyCache { return &keyCache{} }

// federationAllowlistActive reports whether a federation allowlist is
// configured.
func (app *App) federationAllowlistActive() bool {
	return len(app.fedAllowlist) > 0
}

// federationAllowed reports whether the given hostname may federate with this
// instance. With no allowlist configured every host is allowed, which
// preserves the behaviour of an instance that has not opted in.
//
// Matching is exact and case-insensitive. An entry never admits its
// subdomains: example.org does not allow evil.example.org.
func (app *App) federationAllowed(host string) bool {
	if !app.federationAllowlistActive() {
		return true
	}
	return app.fedAllowlist[strings.ToLower(host)]
}

// inboxAllowed reports whether an activity may be delivered to the given
// inbox URL. A URL that cannot be parsed is never allowed.
func (app *App) inboxAllowed(inboxURL string) bool {
	if !app.federationAllowlistActive() {
		return true
	}
	u, err := url.Parse(inboxURL)
	if err != nil {
		log.Error("Federation allowlist: can't parse inbox %q: %v", inboxURL, err)
		return false
	}
	return app.federationAllowed(u.Hostname())
}
