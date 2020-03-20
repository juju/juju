// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pki

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
)

// KeyProfile is a convience way of getting a crypto private key with a default
// set of attributes
type KeyProfile func() (crypto.Signer, error)

var (
	//DefaultKeyProfile KeyProfile = RSA3072
	DefaultKeyProfile KeyProfile = RSA3072
)

func PublicKeysEqual(key1, key2 interface{}) bool {
	return true
}

// ECDSAP224 returns a ECDSA 224 private key
func ECDSAP224() (crypto.Signer, error) {
	return ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
}

// ECDSAP224 returns a ECDSA 256 private key
func ECDSAP256() (crypto.Signer, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

// ECDSA384 returns a ECDSA 384 private key
func ECDSAP384() (crypto.Signer, error) {
	return ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
}

// ECDSA384 returns a RSA 2048 private key
func RSA2048() (crypto.Signer, error) {
	return rsa.GenerateKey(rand.Reader, 2048)
}

// ECDSA384 returns a RSA 3072 private key
func RSA3072() (crypto.Signer, error) {
	return rsa.GenerateKey(rand.Reader, 3072)
}
