// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"

	"github.com/juju/tc"
	cryptossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/internal/pki/ssh"
)

type FormatSuite struct {
}

var _ = tc.Suite(&FormatSuite{})

func (s *FormatSuite) TestKeyProfilesFormat(c *tc.C) {
	tests := []struct {
		name          string
		profile       ssh.KeyProfile
		publicKeyType string
	}{
		{name: "ecdsa256", profile: ssh.ECDSAP256, publicKeyType: cryptossh.KeyAlgoECDSA256},
		{name: "ecdsa384", profile: ssh.ECDSAP384, publicKeyType: cryptossh.KeyAlgoECDSA384},
		{name: "ecdsa521", profile: ssh.ECDSAP521, publicKeyType: cryptossh.KeyAlgoECDSA521},
		{name: "rsa2048", profile: ssh.RSA2048, publicKeyType: cryptossh.KeyAlgoRSA},
		{name: "rsa3072", profile: ssh.RSA3072, publicKeyType: cryptossh.KeyAlgoRSA},
		{name: "ed25519", profile: ssh.ED25519, publicKeyType: cryptossh.KeyAlgoED25519},
	}
	for _, test := range tests {
		pk, err := test.profile()
		c.Check(err, tc.ErrorIsNil, tc.Commentf("profile %s", test.name))

		private, public, publicKeyType, err := ssh.FormatKey(pk, "test-comment")
		c.Check(err, tc.ErrorIsNil, tc.Commentf("profile %s", test.name))
		c.Check(private, tc.Not(tc.Equals), "")
		c.Check(public, tc.Not(tc.Equals), "")
		c.Check(public, tc.Matches, test.publicKeyType+` .* test-comment\n`)
		c.Check(publicKeyType, tc.Equals, test.publicKeyType)
	}
}

func (s *FormatSuite) TestBadKey(c *tc.C) {
	_, _, _, err := ssh.FormatKey(nil, "nope")
	c.Assert(err, tc.ErrorMatches, `private key not valid`)
	_, _, _, err = ssh.FormatKey(&struct{}{}, "nope")
	c.Assert(err, tc.ErrorMatches, `private key not valid`)
	_, _, _, err = ssh.FormatKey(&ecdsa.PrivateKey{}, "nope")
	c.Assert(err, tc.ErrorMatches, `cannot encode private key: x509: unknown curve while marshaling to PKCS#8`)

	pk, err := ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	c.Assert(err, tc.ErrorIsNil)
	_, _, _, err = ssh.FormatKey(pk, "nope")
	c.Assert(err, tc.ErrorMatches, `cannot encode public key: public key: ssh: only P-256, P-384 and P-521 EC keys are supported`)
}
