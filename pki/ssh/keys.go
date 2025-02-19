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
	"encoding/pem"

	"github.com/juju/errors"
	gossh "golang.org/x/crypto/ssh"
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

// MarshalPrivateKey marshals a private key to a PEM encoded byte slice.
func MarshalPrivateKey(privateKey crypto.PrivateKey) ([]byte, error) {
	pemKey, err := gossh.MarshalPrivateKey(privateKey, "")
	if err != nil {
		return nil, errors.Annotate(err, "failed to marshal private key")
	}
	return pem.EncodeToMemory(pemKey), nil
}

// UnmarshalPrivateKey unmarshals a private key from a PEM encoded byte slice.
func UnmarshalPrivateKey(data []byte) (crypto.PrivateKey, error) {
	privateKey, err := gossh.ParseRawPrivateKey(data)
	if err != nil {
		return nil, errors.Annotate(err, "failed to unmarshal private key")
	}
	return privateKey, nil
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

// NewMarshalledED25519 is a convenience function wrapping a call to
// create a new ED25519 private key and then marhsalling the result.
func NewMarshalledED25519() ([]byte, error) {
	privateKey, err := ED25519()
	if err != nil {
		return nil, err
	}
	return MarshalPrivateKey(privateKey)
}
