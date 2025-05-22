// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	"crypto"
	"crypto/ed25519"
	"testing"

	"github.com/juju/tc"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/internal/pki/ssh"
)

type KeySuite struct {
}

func TestKeySuite(t *testing.T) {
	tc.Run(t, &KeySuite{})
}

func (s *KeySuite) TestKeyProfilesForErrors(c *tc.C) {
	tests := []struct {
		name    string
		profile ssh.KeyProfile
	}{
		{name: "ecdsa256", profile: ssh.ECDSAP256},
		{name: "ecdsa384", profile: ssh.ECDSAP384},
		{name: "ecdsa521", profile: ssh.ECDSAP521},
		{name: "rsa2048", profile: ssh.RSA2048},
		{name: "rsa3072", profile: ssh.RSA3072},
		{name: "ed25519", profile: ssh.ED25519},
	}
	for _, test := range tests {
		_, err := test.profile()
		c.Check(err, tc.ErrorIsNil, tc.Commentf("profile %s", test.name))
	}
}

func (s *KeySuite) TestGenerateHostKeys(c *tc.C) {
	keys, err := ssh.GenerateHostKeys()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(keys, tc.HasLen, 3)
	keys2, err := ssh.GenerateHostKeys()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(keys2, tc.HasLen, 3)
	for i, key := range keys {
		key2 := keys2[i]
		c.Assert(key, tc.FitsTypeOf, key2)
		typedKey, ok := key.(interface {
			Equal(crypto.PrivateKey) bool
		})
		c.Assert(ok, tc.IsTrue, tc.Commentf("cast %v", key))
		c.Assert(typedKey.Equal(key), tc.IsTrue)
		c.Assert(typedKey.Equal(key2), tc.IsFalse)
	}
}

func (s *KeySuite) TestKeyMarshalling(c *tc.C) {
	privateKey, err := ssh.ED25519()
	c.Assert(err, tc.IsNil)

	want, ok := privateKey.(ed25519.PrivateKey)
	c.Assert(ok, tc.Equals, true)

	data, err := ssh.MarshalPrivateKey(privateKey)
	c.Assert(err, tc.IsNil)

	unmarshalledKey, err := ssh.UnmarshalPrivateKey(data)
	c.Assert(err, tc.IsNil)
	got, ok := unmarshalledKey.(*ed25519.PrivateKey)
	c.Assert(ok, tc.Equals, true)

	c.Assert(want, tc.DeepEquals, *got)
}

func (s *KeySuite) TestGenerateMarshalledED25519Key(c *tc.C) {
	keyStr, err := ssh.NewMarshalledED25519()
	c.Assert(err, tc.IsNil)

	signer, err := gossh.ParsePrivateKey(keyStr)
	c.Assert(err, tc.IsNil)

	c.Assert(signer.PublicKey().Type(), tc.Equals, "ssh-ed25519")
}
