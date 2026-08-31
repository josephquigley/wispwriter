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
	"strings"
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
