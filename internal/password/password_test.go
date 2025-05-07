// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package password

import (
	"github.com/juju/tc"
	"github.com/juju/testing"
)

type passwordSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&passwordSuite{})

// Base64 *can* include a tail of '=' characters, but all the tests here
// explicitly *don't* want those because it is wasteful.
var base64Chars = "^[A-Za-z0-9+/]+$"

func (*passwordSuite) TestRandomBytes(c *tc.C) {
	b, err := RandomBytes(16)
	c.Assert(err, tc.IsNil)
	c.Assert(b, tc.HasLen, 16)
	x0 := b[0]
	for _, x := range b {
		if x != x0 {
			return
		}
	}
	c.Errorf("all same bytes in result of RandomBytes")
}

func (*passwordSuite) TestRandomPassword(c *tc.C) {
	p, err := RandomPassword()
	c.Assert(err, tc.IsNil)
	if len(p) < 18 {
		c.Errorf("password too short: %q", p)
	}
	c.Assert(p, tc.Matches, base64Chars)
}

func (*passwordSuite) TestRandomSalt(c *tc.C) {
	salt, err := RandomSalt()
	c.Assert(err, tc.IsNil)
	if len(salt) < 12 {
		c.Errorf("salt too short: %q", salt)
	}
	// check we're not adding base64 padding.
	c.Assert(salt, tc.Matches, base64Chars)
}

var testPasswords = []string{"", "a", "a longer password than i would usually bother with"}

var testSalts = []string{"abcd", "abcdefgh", "abcdefghijklmnop", CompatSalt}

func (*passwordSuite) TestUserPasswordHash(c *tc.C) {
	seenHashes := make(map[string]bool)
	for i, password := range testPasswords {
		for j, salt := range testSalts {
			c.Logf("test %d, %d %s %s", i, j, password, salt)
			hashed := UserPasswordHash(password, salt)
			c.Logf("hash %q", hashed)
			c.Assert(len(hashed), tc.Equals, 24)
			c.Assert(seenHashes[hashed], tc.Equals, false)
			// check we're not adding base64 padding.
			c.Assert(hashed, tc.Matches, base64Chars)
			seenHashes[hashed] = true
			// check it's deterministic
			altHashed := UserPasswordHash(password, salt)
			c.Assert(altHashed, tc.Equals, hashed)
		}
	}
}

func (*passwordSuite) TestAgentPasswordHash(c *tc.C) {
	seenValues := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		password, err := RandomPassword()
		c.Assert(err, tc.IsNil)
		c.Assert(seenValues[password], tc.IsFalse)
		seenValues[password] = true
		hashed := AgentPasswordHash(password)
		c.Assert(hashed, tc.Not(tc.Equals), password)
		c.Assert(seenValues[hashed], tc.IsFalse)
		seenValues[hashed] = true
		c.Assert(len(hashed), tc.Equals, 24)
		// check we're not adding base64 padding.
		c.Assert(hashed, tc.Matches, base64Chars)
	}
}
