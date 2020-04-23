// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"math/rand"
	"time"
)

//go:generate go run github.com/juju/juju/generate/certgen

// NewCA returns a random one of the pre-generated certs to speed up
// tests. The comment on the certs are not going to match the args.
func NewCA(commonName, UUID string, expiry time.Time) (certPEM, keyPEM string, err error) {
	index := rand.Intn(len(generatedCA))
	cert := generatedCA[index]
	return cert.certPEM, cert.keyPEM, nil
}
