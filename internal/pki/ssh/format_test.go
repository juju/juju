// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"

	jc "github.com/juju/testing/checkers"
	cryptossh "golang.org/x/crypto/ssh"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/pki/ssh"
)

type FormatSuite struct {
}

var _ = gc.Suite(&FormatSuite{})

func (s *FormatSuite) TestKeyProfilesFormat(c *gc.C) {
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
		c.Check(err, jc.ErrorIsNil, gc.Commentf("profile %s", test.name))

		private, public, publicKeyType, err := ssh.FormatKey(pk, "test-comment")
		c.Check(err, jc.ErrorIsNil, gc.Commentf("profile %s", test.name))
		c.Check(private, gc.Not(gc.Equals), "")
		c.Check(public, gc.Not(gc.Equals), "")
		c.Check(public, gc.Matches, test.publicKeyType+` .* test-comment\n`)
		c.Check(publicKeyType, gc.Equals, test.publicKeyType)
	}
}

func (s *FormatSuite) TestBadKey(c *gc.C) {
	_, _, _, err := ssh.FormatKey(nil, "nope")
	c.Assert(err, gc.ErrorMatches, `private key not valid`)
	_, _, _, err = ssh.FormatKey(&struct{}{}, "nope")
	c.Assert(err, gc.ErrorMatches, `private key not valid`)
	_, _, _, err = ssh.FormatKey(&ecdsa.PrivateKey{}, "nope")
	c.Assert(err, gc.ErrorMatches, `cannot encode private key: x509: unknown curve while marshaling to PKCS#8`)

	pk, err := ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	c.Assert(err, jc.ErrorIsNil)
	_, _, _, err = ssh.FormatKey(pk, "nope")
	c.Assert(err, gc.ErrorMatches, `cannot encode public key: public key: ssh: only P-256, P-384 and P-521 EC keys are supported`)
}
