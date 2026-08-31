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
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/writeas/httpsig"

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

// signedRequest builds a request signed the way WriteFreely itself signs
// outbound requests, so the gate is exercised against a real signature.
func signedRequest(t *testing.T, k *rsa.PrivateKey, keyID, method, target string, body []byte) *http.Request {
	t.Helper()
	return signedRequestWithHeaders(t, k, keyID, method, target, body, []string{"(request-target)", "date", "host", "digest"})
}

// signedRequestWithHeaders is signedRequest, but with the caller's own
// choice of signed header set. It exists to build signatures that omit
// headers the gate is expected to require.
func signedRequestWithHeaders(t *testing.T, k *rsa.PrivateKey, keyID, method, target string, body []byte, headers []string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(method, target, bytes.NewReader(body))
	r.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	sum := sha256.Sum256(body)
	r.Header.Set("Digest", "SHA-256="+base64.StdEncoding.EncodeToString(sum[:]))

	signer := httpsig.NewSigner(keyID, k, httpsig.RSASHA256, headers)
	if err := signer.SignSigHeader(r); err != nil {
		t.Fatalf("sign: %v", err)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return r
}

func TestVerifyRejectsWhenNoAllowlistConfigured(t *testing.T) {
	app := allowlistApp(t, "")
	k := testKey(t)
	r := signedRequest(t, k, "https://example.org/users/a#main-key", "GET", "https://local.example/api/collections/x/outbox", nil)

	assert.Equal(t, ErrFederationNotAllowed, app.verifyAllowlistedSignature(r))
}

func TestVerifyRejectsUnsignedRequest(t *testing.T) {
	app := allowlistApp(t, "example.org")
	r := httptest.NewRequest("GET", "https://local.example/api/collections/x/outbox", nil)

	assert.Equal(t, ErrFederationNotAllowed, app.verifyAllowlistedSignature(r))
}

func TestVerifyRejectsUnlistedHostWithoutTouchingTheKeyCache(t *testing.T) {
	// This pins the ordering that matters: a request from a host that is not
	// on the allowlist must be rejected before anything is fetched, so it can
	// never make this server dial a URL of the caller's choosing.
	app := allowlistApp(t, "example.org")
	k := testKey(t)
	r := signedRequest(t, k, "https://evil.example/users/a#main-key", "GET", "https://local.example/api/collections/x/outbox", nil)

	assert.Equal(t, ErrFederationNotAllowed, app.verifyAllowlistedSignature(r))
	assert.Empty(t, app.fedKeys.keys, "no key fetch may be attempted for an unlisted host")
}

func TestVerifyAcceptsAllowlistedSignature(t *testing.T) {
	app := allowlistApp(t, "example.org")
	k := testKey(t)
	keyID := "https://example.org/users/a#main-key"
	app.fedKeys.set(keyID, &k.PublicKey, time.Minute)
	r := signedRequest(t, k, keyID, "GET", "https://local.example/api/collections/x/outbox", nil)

	assert.NoError(t, app.verifyAllowlistedSignature(r))
}

func TestVerifyRejectsTamperedRequest(t *testing.T) {
	app := allowlistApp(t, "example.org")
	k := testKey(t)
	keyID := "https://example.org/users/a#main-key"
	app.fedKeys.set(keyID, &k.PublicKey, time.Minute)
	r := signedRequest(t, k, keyID, "GET", "https://local.example/api/collections/x/outbox", nil)
	r.URL.Path = "/api/collections/other/outbox"

	assert.Equal(t, ErrFederationNotAllowed, app.verifyAllowlistedSignature(r))
}

func TestVerifyRejectsWrongKey(t *testing.T) {
	app := allowlistApp(t, "example.org")
	signing := testKey(t)
	other := testKey(t)
	keyID := "https://example.org/users/a#main-key"
	app.fedKeys.set(keyID, &other.PublicKey, time.Minute)
	r := signedRequest(t, signing, keyID, "GET", "https://local.example/api/collections/x/outbox", nil)

	assert.Equal(t, ErrFederationNotAllowed, app.verifyAllowlistedSignature(r))
}

func TestVerifyRejectsStaleDate(t *testing.T) {
	app := allowlistApp(t, "example.org")
	k := testKey(t)
	keyID := "https://example.org/users/a#main-key"
	app.fedKeys.set(keyID, &k.PublicKey, time.Minute)

	r := httptest.NewRequest("GET", "https://local.example/api/collections/x/outbox", nil)
	r.Header.Set("Date", time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat))
	sum := sha256.Sum256(nil)
	r.Header.Set("Digest", "SHA-256="+base64.StdEncoding.EncodeToString(sum[:]))
	signer := httpsig.NewSigner(keyID, k, httpsig.RSASHA256, []string{"(request-target)", "date", "host", "digest"})
	if err := signer.SignSigHeader(r); err != nil {
		t.Fatalf("sign: %v", err)
	}

	assert.Equal(t, ErrFederationNotAllowed, app.verifyAllowlistedSignature(r))
}

func TestCheckRequestDate(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)

	fresh := httptest.NewRequest("GET", "https://local.example/", nil)
	fresh.Header.Set("Date", now.Format(http.TimeFormat))
	assert.NoError(t, checkRequestDate(fresh, now))

	old := httptest.NewRequest("GET", "https://local.example/", nil)
	old.Header.Set("Date", now.Add(-10*time.Minute).Format(http.TimeFormat))
	assert.Error(t, checkRequestDate(old, now))

	future := httptest.NewRequest("GET", "https://local.example/", nil)
	future.Header.Set("Date", now.Add(10*time.Minute).Format(http.TimeFormat))
	assert.Error(t, checkRequestDate(future, now))

	missing := httptest.NewRequest("GET", "https://local.example/", nil)
	assert.Error(t, checkRequestDate(missing, now))

	junk := httptest.NewRequest("GET", "https://local.example/", nil)
	junk.Header.Set("Date", "not a date")
	assert.Error(t, checkRequestDate(junk, now))
}

func TestVerifyDigest(t *testing.T) {
	body := []byte(`{"type":"Follow"}`)
	sum := sha256.Sum256(body)

	ok := httptest.NewRequest("POST", "https://local.example/inbox", bytes.NewReader(body))
	ok.Header.Set("Digest", "SHA-256="+base64.StdEncoding.EncodeToString(sum[:]))
	assert.NoError(t, verifyDigest(ok))

	// The body must still be readable by the next handler.
	rest, err := io.ReadAll(ok.Body)
	assert.NoError(t, err)
	assert.Equal(t, body, rest)

	bad := httptest.NewRequest("POST", "https://local.example/inbox", bytes.NewReader([]byte(`{"type":"Undo"}`)))
	bad.Header.Set("Digest", "SHA-256="+base64.StdEncoding.EncodeToString(sum[:]))
	assert.Error(t, verifyDigest(bad))

	none := httptest.NewRequest("POST", "https://local.example/inbox", bytes.NewReader(body))
	assert.Error(t, verifyDigest(none))

	unsupported := httptest.NewRequest("POST", "https://local.example/inbox", bytes.NewReader(body))
	unsupported.Header.Set("Digest", "SHA-512=abc")
	assert.Error(t, verifyDigest(unsupported))
}

func TestVerifyRejectsMismatchedDigestOnPost(t *testing.T) {
	app := allowlistApp(t, "example.org")
	k := testKey(t)
	keyID := "https://example.org/users/a#main-key"
	app.fedKeys.set(keyID, &k.PublicKey, time.Minute)

	body := []byte(`{"type":"Follow"}`)
	r := signedRequest(t, k, keyID, "POST", "https://local.example/inbox", body)
	// Swap the body without touching the signed Digest header.
	r.Body = io.NopCloser(bytes.NewReader([]byte(`{"type":"Undo"}`)))

	assert.Equal(t, ErrFederationNotAllowed, app.verifyAllowlistedSignature(r))
}

func TestVerifyRejectsSignatureOmittingDigestOnRequestWithBody(t *testing.T) {
	// A signature that does not cover Digest leaves the body wholly unbound
	// to what was signed: the gate must not accept it just because a
	// separately correct Digest header happens to be present.
	app := allowlistApp(t, "example.org")
	k := testKey(t)
	keyID := "https://example.org/users/a#main-key"
	app.fedKeys.set(keyID, &k.PublicKey, time.Minute)

	body := []byte(`{"type":"Follow"}`)
	r := signedRequestWithHeaders(t, k, keyID, "POST", "https://local.example/inbox", body,
		[]string{"(request-target)", "date", "host"})

	assert.Equal(t, ErrFederationNotAllowed, app.verifyAllowlistedSignature(r))
}

func TestVerifyRejectsSignatureOmittingHost(t *testing.T) {
	// A signature that does not cover Host could be replayed against a
	// different destination server entirely.
	app := allowlistApp(t, "example.org")
	k := testKey(t)
	keyID := "https://example.org/users/a#main-key"
	app.fedKeys.set(keyID, &k.PublicKey, time.Minute)

	r := signedRequestWithHeaders(t, k, keyID, "GET", "https://local.example/api/collections/x/outbox", nil,
		[]string{"(request-target)", "date", "digest"})

	assert.Equal(t, ErrFederationNotAllowed, app.verifyAllowlistedSignature(r))
}

func TestVerifyRejectsSignatureOmittingRequestTarget(t *testing.T) {
	// A signature that does not cover (request-target) could be replayed
	// against a different method or path.
	app := allowlistApp(t, "example.org")
	k := testKey(t)
	keyID := "https://example.org/users/a#main-key"
	app.fedKeys.set(keyID, &k.PublicKey, time.Minute)

	r := signedRequestWithHeaders(t, k, keyID, "GET", "https://local.example/api/collections/x/outbox", nil,
		[]string{"date", "host", "digest"})

	assert.Equal(t, ErrFederationNotAllowed, app.verifyAllowlistedSignature(r))
}

func TestVerifyRejectsSignatureWithNoHeadersParameter(t *testing.T) {
	// With no headers parameter at all, go-fed defaults the covered set to
	// ["date"]. That defaulting must not be treated as an acceptable
	// signed set: it is exactly the case this gate exists to close.
	app := allowlistApp(t, "example.org")
	k := testKey(t)
	keyID := "https://example.org/users/a#main-key"
	app.fedKeys.set(keyID, &k.PublicKey, time.Minute)

	r := signedRequestWithHeaders(t, k, keyID, "GET", "https://local.example/api/collections/x/outbox", nil,
		[]string{"date"})
	assert.NotContains(t, r.Header.Get("Signature"), "headers=", "test setup: this signature must omit the headers parameter")

	assert.Equal(t, ErrFederationNotAllowed, app.verifyAllowlistedSignature(r))
}

func TestVerifyRejectsChunkedRequestSignatureOmittingDigest(t *testing.T) {
	// A chunked request has no Content-Length: net/http reports -1 for
	// unknown length. requestHasBody must treat that as "there is a body"
	// rather than "there is no body", or an attacker could dodge the
	// digest requirement entirely by choosing a transfer coding.
	app := allowlistApp(t, "example.org")
	k := testKey(t)
	keyID := "https://example.org/users/a#main-key"
	app.fedKeys.set(keyID, &k.PublicKey, time.Minute)

	body := []byte(`{"type":"Follow"}`)
	r := httptest.NewRequest("POST", "https://local.example/inbox", bytes.NewReader(body))
	r.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	sum := sha256.Sum256(body)
	r.Header.Set("Digest", "SHA-256="+base64.StdEncoding.EncodeToString(sum[:]))
	// Make this look like a chunked request to the gate: unknown length,
	// no Content-Length header.
	r.ContentLength = -1
	r.TransferEncoding = []string{"chunked"}

	// Sign without covering Digest, exactly the attack this test exists to
	// close off.
	signer := httpsig.NewSigner(keyID, k, httpsig.RSASHA256, []string{"(request-target)", "date", "host"})
	if err := signer.SignSigHeader(r); err != nil {
		t.Fatalf("sign: %v", err)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	assert.Equal(t, ErrFederationNotAllowed, app.verifyAllowlistedSignature(r))
}

func TestVerifyRejectsSignatureWithDuplicatedHeadersParameterFakeFirst(t *testing.T) {
	// This is the parser differential: signedHeaderSet (before the fix) uses
	// strings.Index, which finds the FIRST occurrence of headers=. go-fed's
	// own parser assigns on every match it sees, so it ends up holding the
	// LAST one. An attacker who inserts a fabricated, broad headers= ahead
	// of the real, narrow one can satisfy our check while go-fed verifies
	// against the real, narrower set the signature was actually computed
	// over, leaving the body unbound to what was verified.
	app := allowlistApp(t, "example.org")
	k := testKey(t)
	keyID := "https://example.org/users/a#main-key"
	app.fedKeys.set(keyID, &k.PublicKey, time.Minute)

	body := []byte(`{"type":"Follow"}`)
	// Genuinely signed over a narrow set: no digest.
	r := signedRequestWithHeaders(t, k, keyID, "POST", "https://local.example/inbox", body,
		[]string{"(request-target)", "date", "host"})

	sig := r.Header.Get("Signature")
	idx := strings.Index(sig, `headers="`)
	if idx == -1 {
		t.Fatalf("test setup: signature has no headers parameter to duplicate")
	}
	// Insert a fabricated, broader headers= parameter before the real one.
	fake := `headers="(request-target) date host digest",`
	r.Header.Set("Signature", sig[:idx]+fake+sig[idx:])

	assert.Equal(t, ErrFederationNotAllowed, app.verifyAllowlistedSignature(r))
}

func TestVerifyRejectsSignatureWithDuplicatedHeadersParameterFakeSecond(t *testing.T) {
	// The mirrored ordering: a fabricated, narrow headers= is inserted
	// before the real, broad one. Duplicates are refused outright
	// regardless of which occurrence would otherwise "win", so this must be
	// rejected the same as the other ordering.
	app := allowlistApp(t, "example.org")
	k := testKey(t)
	keyID := "https://example.org/users/a#main-key"
	app.fedKeys.set(keyID, &k.PublicKey, time.Minute)

	body := []byte(`{"type":"Follow"}`)
	// Genuinely signed over the full required set.
	r := signedRequest(t, k, keyID, "POST", "https://local.example/inbox", body)

	sig := r.Header.Get("Signature")
	idx := strings.Index(sig, `headers="`)
	if idx == -1 {
		t.Fatalf("test setup: signature has no headers parameter to duplicate")
	}
	fake := `headers="(request-target) date host",`
	r.Header.Set("Signature", sig[:idx]+fake+sig[idx:])

	assert.Equal(t, ErrFederationNotAllowed, app.verifyAllowlistedSignature(r))
}

func TestVerifyRejectsSignatureWithDuplicatedKeyIdParameter(t *testing.T) {
	app := allowlistApp(t, "example.org")
	k := testKey(t)
	keyID := "https://example.org/users/a#main-key"
	app.fedKeys.set(keyID, &k.PublicKey, time.Minute)

	r := signedRequest(t, k, keyID, "GET", "https://local.example/api/collections/x/outbox", nil)

	sig := r.Header.Get("Signature")
	idx := strings.Index(sig, `keyId="`)
	if idx == -1 {
		t.Fatalf("test setup: signature has no keyId parameter to duplicate")
	}
	fake := `keyId="https://example.org/users/a#main-key",`
	r.Header.Set("Signature", sig[:idx]+fake+sig[idx:])

	assert.Equal(t, ErrFederationNotAllowed, app.verifyAllowlistedSignature(r))
}

func TestVerifySignatureNotConfusedByHeadersSubstringInsideAnotherParameter(t *testing.T) {
	// The token "headers=" can legitimately occur inside another
	// parameter's value, such as a keyId URL's query string. A correct
	// comma-and-first-equals parser is not confused by it.
	app := allowlistApp(t, "example.org")
	k := testKey(t)
	keyID := "https://example.org/users/a?query=headers=foo#main-key"
	app.fedKeys.set(keyID, &k.PublicKey, time.Minute)
	r := signedRequest(t, k, keyID, "GET", "https://local.example/api/collections/x/outbox", nil)

	assert.NoError(t, app.verifyAllowlistedSignature(r))
}

func TestVerifyRejectsSignatureWithCapitalizedHeadersParameterAndForgedBody(t *testing.T) {
	// go-fed matches parameter keys with an exact-case switch against the
	// literal strings "keyId", "algorithm", "headers", "signature". A
	// differently-cased key is silently ignored, and when no correctly
	// cased headers key is present at all, go-fed defaults the covered set
	// to ["date"]. If this code lowercased parameter keys before comparing
	// them, a fabricated "Headers=" would be read as authoritative here
	// while go-fed ignores it and falls back to the real, narrower set the
	// signature actually covers. Because the genuine signature carries no
	// lowercase "headers" key at all, round 3's duplicate detection never
	// triggers: there is only one key spelled "headers" (there are zero).
	//
	// This is full forgery: sign over ["date"] only, then splice in a
	// capitalized Headers= parameter and rewrite both the body and the
	// Digest header to arbitrary content.
	app := allowlistApp(t, "example.org")
	k := testKey(t)
	keyID := "https://example.org/users/a#main-key"
	app.fedKeys.set(keyID, &k.PublicKey, time.Minute)

	body := []byte(`{"type":"Follow"}`)
	r := signedRequestWithHeaders(t, k, keyID, "POST", "https://local.example/inbox", body, []string{"date"})

	sig := r.Header.Get("Signature")
	if strings.Contains(sig, "headers=") {
		t.Fatalf("test setup: signature must genuinely have no headers parameter")
	}
	fake := `Headers="(request-target) host date digest",`
	r.Header.Set("Signature", fake+sig)

	forged := []byte(`{"type":"Delete","object":"everything"}`)
	forgedSum := sha256.Sum256(forged)
	r.Header.Set("Digest", "SHA-256="+base64.StdEncoding.EncodeToString(forgedSum[:]))
	r.Body = io.NopCloser(bytes.NewReader(forged))
	r.ContentLength = int64(len(forged))

	assert.Equal(t, ErrFederationNotAllowed, app.verifyAllowlistedSignature(r))
}

func TestVerifyRejectsSignatureWhoseOnlyHeadersParameterIsWronglyCased(t *testing.T) {
	// After keys are compared case-sensitively, a signature whose only
	// header-set parameter is spelled HEADERS= or Headers= presents no
	// "headers" key at all, which the round 1 rule already rejects.
	app := allowlistApp(t, "example.org")
	k := testKey(t)
	keyID := "https://example.org/users/a#main-key"
	app.fedKeys.set(keyID, &k.PublicKey, time.Minute)

	r := signedRequestWithHeaders(t, k, keyID, "GET", "https://local.example/api/collections/x/outbox", nil,
		[]string{"(request-target)", "date", "host"})

	sig := r.Header.Get("Signature")
	idx := strings.Index(sig, `headers="`)
	if idx == -1 {
		t.Fatalf("test setup: signature has no headers parameter to recase")
	}
	recased := sig[:idx] + "HEADERS=" + sig[idx+len("headers="):]
	r.Header.Set("Signature", recased)

	assert.Equal(t, ErrFederationNotAllowed, app.verifyAllowlistedSignature(r))
}
