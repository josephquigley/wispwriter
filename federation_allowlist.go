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
	"crypto/rsa"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/writeas/web-core/activitypub"
	"github.com/writeas/web-core/activitystreams"
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

const (
	// allowlistKeyTTL is how long a fetched remote public key is reused.
	allowlistKeyTTL = time.Hour
	// allowlistKeyErrorTTL is how long a failed key fetch is remembered, so
	// an unreachable peer cannot make every request wait on a timeout.
	allowlistKeyErrorTTL = time.Minute
	// allowlistClockSkew is how far a request's Date header may sit from the
	// current time before the request is treated as a replay.
	allowlistClockSkew = 5 * time.Minute
)

// cachedKey is one key cache entry. A nil key records a failed fetch.
type cachedKey struct {
	key     *rsa.PublicKey
	expires time.Time
}

// keyCache holds remote public keys used to verify incoming signatures. It is
// safe for concurrent use.
type keyCache struct {
	mu   sync.RWMutex
	keys map[string]cachedKey
}

// newKeyCache returns an empty key cache.
func newKeyCache() *keyCache {
	return &keyCache{keys: map[string]cachedKey{}}
}

// get returns the cached key for keyID. The second return value reports
// whether a live entry exists: an entry that exists with a nil key records an
// earlier failure and should not be retried yet.
func (c *keyCache) get(keyID string) (*rsa.PublicKey, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.keys[keyID]
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.key, true
}

// set stores a key, or a nil key to record a failure, for the given duration.
func (c *keyCache) set(keyID string, k *rsa.PublicKey, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.keys[keyID] = cachedKey{key: k, expires: time.Now().Add(ttl)}
}

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

// allowlistedKey returns the RSA public key named by keyID, fetching the
// owning actor if the key is not cached.
//
// The caller must already have checked keyID's host against the allowlist:
// this function makes an outbound request.
//
// The actor is fetched directly rather than through getActor, because a
// RemoteUser cached in the database carries no public key and AsPerson would
// return an actor with an empty one.
func (app *App) allowlistedKey(keyID string) (*rsa.PublicKey, error) {
	if k, found := app.fedKeys.get(keyID); found {
		if k == nil {
			return nil, fmt.Errorf("key %s was recently unreachable", keyID)
		}
		return k, nil
	}

	u, err := url.Parse(keyID)
	if err != nil {
		return nil, err
	}
	keyHost := u.Hostname()
	u.Fragment = ""
	actorIRI := u.String()

	resp, err := resolveIRI(app.cfg.App.Host, actorIRI)
	if err != nil {
		app.fedKeys.set(keyID, nil, allowlistKeyErrorTTL)
		return nil, err
	}
	actor := &activitystreams.Person{}
	if err := unmarshalActor(resp, actor); err != nil {
		app.fedKeys.set(keyID, nil, allowlistKeyErrorTTL)
		return nil, err
	}

	// The fetched actor must actually own the key it was asked about, and
	// must still be on the host the allowlist was checked against. Without
	// the host check a redirect could hand back a key from elsewhere.
	if actor.PublicKey.ID != keyID {
		app.fedKeys.set(keyID, nil, allowlistKeyErrorTTL)
		return nil, fmt.Errorf("actor %s does not own key %s", actorIRI, keyID)
	}
	if actor.PublicKey.Owner != actor.ID {
		app.fedKeys.set(keyID, nil, allowlistKeyErrorTTL)
		return nil, fmt.Errorf("key %s names owner %s, but the actor is %s", keyID, actor.PublicKey.Owner, actor.ID)
	}
	actorURL, err := url.Parse(actor.ID)
	if err != nil {
		app.fedKeys.set(keyID, nil, allowlistKeyErrorTTL)
		return nil, err
	}
	if !strings.EqualFold(actorURL.Hostname(), keyHost) {
		app.fedKeys.set(keyID, nil, allowlistKeyErrorTTL)
		return nil, fmt.Errorf("actor %s is not on key host %s", actor.ID, keyHost)
	}

	pub, err := activitypub.DecodePublicKey([]byte(actor.PublicKey.PublicKeyPEM))
	if err != nil {
		app.fedKeys.set(keyID, nil, allowlistKeyErrorTTL)
		return nil, err
	}
	rsaKey, ok := pub.(*rsa.PublicKey)
	if !ok {
		app.fedKeys.set(keyID, nil, allowlistKeyErrorTTL)
		return nil, fmt.Errorf("key %s is not an RSA key", keyID)
	}

	app.fedKeys.set(keyID, rsaKey, allowlistKeyTTL)
	return rsaKey, nil
}
