// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package test

import (
	"crypto"
	"crypto/rsa"
	"flag"
	"math/rand"

	"github.com/juju/juju/internal/pki"
)

var insecureRand = rand.New(rand.NewSource(0))

// InsecureKeyProfile for tests. Will panic if used outside tests.
func InsecureKeyProfile() (crypto.Signer, error) {
	if flag.Lookup("test.v") == nil {
		panic("InsecureKeyProfile cannot be used outside tests")
	}
	return rsa.GenerateKey(insecureRand, 512)
}

// OriginalDefaultKeyProfile is the pre-patched pki.DefaultKeyProfile
// value.
var OriginalDefaultKeyProfile = pki.DefaultKeyProfile

func init() {
	pki.DefaultKeyProfile = InsecureKeyProfile
}
