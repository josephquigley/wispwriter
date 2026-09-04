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
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

// keyBitSize is the RSA key length for ActivityPub actors. It matches the
// value web-core generated, and 2048 is the floor the fediverse accepts in
// practice: shorter keys are rejected by common server software.
const keyBitSize = 2048

// generateAPKeys creates an RSA keypair for an ActivityPub actor and returns
// the public and private keys PEM-encoded, in that order.
//
// This replaces web-core's activitypub.GenerateKeys, which shells out to the
// openssl command line tool. That is a dependency on a binary rather than on a
// library, so it is satisfied by the build machine and not by the module
// graph, and it goes missing without warning: an image carrying libcrypto but
// not /usr/bin/openssl generates no keys, stores none, and leaves every actor
// unable to sign. The failure then surfaces somewhere else entirely, as an
// unsigned request a remote server refuses. Piping a private key through a
// subprocess's stdin, which is how the public half was derived, is also worse
// than not needing the subprocess at all.
//
// The two encodings are chosen to be byte compatible with what openssl
// produced, so keypairs already in a database stay readable and remote servers
// see no change:
//
//	private  PKCS#1, "RSA PRIVATE KEY", as `openssl genrsa` emitted
//	public   PKIX,   "PUBLIC KEY",      as `openssl rsa -pubout` emitted
//
// Both are named explicitly by web-core's decoders, and the public key is
// published in the actor document, so neither may drift.
func generateAPKeys() (pubPEM []byte, privPEM []byte, err error) {
	key, err := rsa.GenerateKey(rand.Reader, keyBitSize)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to generate RSA key: %v", err)
	}

	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to encode public key: %v", err)
	}

	pubPEM = pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	privPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	return pubPEM, privPEM, nil
}
