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
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/writeas/web-core/activitypub"
)

// TestGenerateAPKeysNeedsNoOpenSSLBinary is the regression test for this file's
// reason to exist. web-core's GenerateKeys shells out to the openssl binary, so
// an image that ships the OpenSSL libraries but not the command line tool
// generates no keys at all: every actor is left unable to sign, and the failure
// surfaces far away as an unsigned outbound fetch. Emptying PATH reproduces that
// image exactly, without needing one.
func TestGenerateAPKeysNeedsNoOpenSSLBinary(t *testing.T) {
	t.Setenv("PATH", "")

	pub, priv, err := generateAPKeys()

	assert.NoError(t, err)
	assert.NotEmpty(t, pub)
	assert.NotEmpty(t, priv)
}

// TestGenerateAPKeysUsesDecodablePEMBlockTypes pins the two PEM block types,
// because web-core's decoders reject anything else by name and the public key
// is published in the actor document for remote servers to parse. Neither
// encoding may drift.
func TestGenerateAPKeysUsesDecodablePEMBlockTypes(t *testing.T) {
	pub, priv, err := generateAPKeys()
	assert.NoError(t, err)

	pubBlock, rest := pem.Decode(pub)
	assert.NotNil(t, pubBlock)
	assert.Equal(t, "PUBLIC KEY", pubBlock.Type, "DecodePublicKey accepts this type and no other")
	assert.Empty(t, rest, "a trailing block would be silently ignored by pem.Decode")

	privBlock, rest := pem.Decode(priv)
	assert.NotNil(t, privBlock)
	assert.Equal(t, "RSA PRIVATE KEY", privBlock.Type, "DecodePrivateKey accepts this type and PRIVATE KEY")
	assert.Empty(t, rest)
}

// TestGenerateAPKeysRoundTripsThroughWebCore checks the keys against the
// functions that actually consume them, rather than against our own idea of
// what they should look like.
func TestGenerateAPKeysRoundTripsThroughWebCore(t *testing.T) {
	pubPEM, privPEM, err := generateAPKeys()
	assert.NoError(t, err)

	pub, err := activitypub.DecodePublicKey(pubPEM)
	assert.NoError(t, err)
	priv, err := activitypub.DecodePrivateKey(privPEM)
	assert.NoError(t, err)

	rsaPub, ok := pub.(*rsa.PublicKey)
	assert.True(t, ok, "HTTP signatures are RSA-SHA256, so an RSA key is not optional")
	rsaPriv, ok := priv.(*rsa.PrivateKey)
	assert.True(t, ok)

	assert.Equal(t, keyBitSize, rsaPub.N.BitLen(), "a shorter key would be accepted by us and rejected by peers")
	assert.Equal(t, 0, rsaPriv.PublicKey.N.Cmp(rsaPub.N), "the public key must belong to the private key")
}

// TestGenerateAPKeysReturnsDistinctKeypairs guards against a caching or
// package level variable mistake, which would hand every collection on an
// instance the same identity.
func TestGenerateAPKeysReturnsDistinctKeypairs(t *testing.T) {
	_, first, err := generateAPKeys()
	assert.NoError(t, err)
	_, second, err := generateAPKeys()
	assert.NoError(t, err)

	assert.NotEqual(t, first, second)
}
