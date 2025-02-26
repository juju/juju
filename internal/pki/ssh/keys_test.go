// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	"crypto"
	"crypto/ed25519"

	jc "github.com/juju/testing/checkers"
	gossh "golang.org/x/crypto/ssh"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/pki/ssh"
)

type KeySuite struct {
}

var _ = gc.Suite(&KeySuite{})

func (s *KeySuite) TestKeyProfilesForErrors(c *gc.C) {
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
		c.Check(err, jc.ErrorIsNil, gc.Commentf("profile %s", test.name))
	}
}

func (s *KeySuite) TestGenerateHostKeys(c *gc.C) {
	keys, err := ssh.GenerateHostKeys()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(keys, gc.HasLen, 3)
	keys2, err := ssh.GenerateHostKeys()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(keys2, gc.HasLen, 3)
	for i, key := range keys {
		key2 := keys2[i]
		c.Assert(key, gc.FitsTypeOf, key2)
		typedKey, ok := key.(interface {
			Equal(crypto.PrivateKey) bool
		})
		c.Assert(ok, jc.IsTrue, gc.Commentf("cast %v", key))
		c.Assert(typedKey.Equal(key), jc.IsTrue)
		c.Assert(typedKey.Equal(key2), jc.IsFalse)
	}
}

func (s *KeySuite) TestKeyMarshalling(c *gc.C) {
	privateKey, err := ssh.ED25519()
	c.Assert(err, gc.IsNil)

	want, ok := privateKey.(ed25519.PrivateKey)
	c.Assert(ok, gc.Equals, true)

	data, err := ssh.MarshalPrivateKey(privateKey)
	c.Assert(err, gc.IsNil)

	unmarshalledKey, err := ssh.UnmarshalPrivateKey(data)
	c.Assert(err, gc.IsNil)
	got, ok := unmarshalledKey.(*ed25519.PrivateKey)
	c.Assert(ok, gc.Equals, true)

	c.Assert(want, gc.DeepEquals, *got)
}

func (s *KeySuite) TestGenerateMarshalledED25519Key(c *gc.C) {
	keyStr, err := ssh.NewMarshalledED25519()
	c.Assert(err, gc.IsNil)

	signer, err := gossh.ParsePrivateKey(keyStr)
	c.Assert(err, gc.IsNil)

	c.Assert(signer.PublicKey().Type(), gc.Equals, "ssh-ed25519")
}
