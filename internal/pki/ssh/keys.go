// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"

	"github.com/juju/errors"
)

type KeyProfile func() (crypto.PrivateKey, error)

// ECDSAP256 returns a ECDSA 256 private key
func ECDSAP256() (crypto.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

// ECDSA384 returns a ECDSA 384 private key
func ECDSAP384() (crypto.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
}

// ECDSAP521 returns a ECDSA 521 private key
func ECDSAP521() (crypto.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
}

// RSA2048 returns a RSA 2048 private key
func RSA2048() (crypto.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, 2048)
}

// RSA3072 returns a RSA 3072 private key
func RSA3072() (crypto.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, 3072)
}

// ED25519 returns a ed25519 private key
func ED25519() (crypto.PrivateKey, error) {
	_, pk, err := ed25519.GenerateKey(rand.Reader)
	return pk, err
}

var hostKeyProfiles = []KeyProfile{
	RSA2048,
	ECDSAP256,
	ED25519,
}

// GenerateHostKeys returns newly generated keys of various algorithms/curves/parameters
// to be used as ssh host heys.
func GenerateHostKeys() ([]crypto.PrivateKey, error) {
	var res []crypto.PrivateKey
	for _, f := range hostKeyProfiles {
		k, err := f()
		if err != nil {
			return nil, errors.Trace(err)
		}
		res = append(res, k)
	}
	return res, nil
}
