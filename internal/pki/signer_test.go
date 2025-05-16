// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pki_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/pki"
)

type SignerSuite struct {
}

func TestSignerSuite(t *stdtesting.T) { tc.Run(t, &SignerSuite{}) }
func (s *SignerSuite) TestKeyProfilesForErrors(c *tc.C) {
	tests := []struct {
		name    string
		profile pki.KeyProfile
	}{
		{name: "ecdsa224", profile: pki.ECDSAP224},
		{name: "ecdsa256", profile: pki.ECDSAP256},
		{name: "ecdsa384", profile: pki.ECDSAP384},
		{name: "rsa3072", profile: pki.RSA3072},
	}

	for _, test := range tests {
		if _, err := test.profile(); err != nil {
			c.Errorf("failed running test for profile %s: %v", test.name, err)
		}
	}
}
