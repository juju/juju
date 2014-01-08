// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	gc "launchpad.net/gocheck"

	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/utils"
)

type passwordSuite struct{}

var _ = gc.Suite(passwordSuite{})

// Base64 *can* include a tail of '=' characters, but all the tests here
// explicitly *don't* want those because it is wasteful.
var base64Chars = "^[A-Za-z0-9+/]+$"

func (passwordSuite) TestRandomBytes(c *gc.C) {
	b, err := utils.RandomBytes(16)
	c.Assert(err, gc.IsNil)
	c.Assert(b, gc.HasLen, 16)
	x0 := b[0]
	for _, x := range b {
		if x != x0 {
			return
		}
	}
	c.Errorf("all same bytes in result of RandomBytes")
}

func (passwordSuite) TestRandomPassword(c *gc.C) {
	p, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	if len(p) < 18 {
		c.Errorf("password too short: %q", p)
	}
	c.Assert(p, gc.Matches, base64Chars)
}

func (passwordSuite) TestRandomSalt(c *gc.C) {
	salt, err := utils.RandomSalt()
	c.Assert(err, gc.IsNil)
	if len(salt) < 12 {
		c.Errorf("salt too short: %q", salt)
	}
	// check we're not adding base64 padding.
	c.Assert(salt, gc.Matches, base64Chars)
}

var testPasswords = []string{"", "a", "a longer password than i would usually bother with"}

var testSalts = []string{"abcd", "abcdefgh", "abcdefghijklmnop", utils.CompatSalt}

func (passwordSuite) TestUserPasswordHash(c *gc.C) {
	seenHashes := make(map[string]bool)
	for i, password := range testPasswords {
		for j, salt := range testSalts {
			c.Logf("test %d, %d %s %s", i, j, password, salt)
			hashed := utils.UserPasswordHash(password, salt)
			c.Logf("hash %q", hashed)
			c.Assert(len(hashed), gc.Equals, 24)
			c.Assert(seenHashes[hashed], gc.Equals, false)
			// check we're not adding base64 padding.
			c.Assert(hashed, gc.Matches, base64Chars)
			seenHashes[hashed] = true
			// check it's deterministic
			altHashed := utils.UserPasswordHash(password, salt)
			c.Assert(altHashed, gc.Equals, hashed)
		}
	}
}

func (passwordSuite) TestAgentPasswordHash(c *gc.C) {
	seenValues := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		password, err := utils.RandomPassword()
		c.Assert(err, gc.IsNil)
		c.Assert(seenValues[password], jc.IsFalse)
		seenValues[password] = true
		hashed := utils.AgentPasswordHash(password)
		c.Assert(hashed, gc.Not(gc.Equals), password)
		c.Assert(seenValues[hashed], jc.IsFalse)
		seenValues[hashed] = true
		c.Assert(len(hashed), gc.Equals, 24)
		// check we're not adding base64 padding.
		c.Assert(hashed, gc.Matches, base64Chars)
	}
}
