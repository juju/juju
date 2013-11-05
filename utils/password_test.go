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
	// check we're not adding base64 padding.
	c.Assert(p[len(p)-1], gc.Not(gc.Equals), '=')
}

var testPasswords = []string{"", "a", "a longer password than i would usually bother with"}

func (passwordSuite) TestCompatPasswordHash(c *gc.C) {
	seenHashes := make(map[string]bool)
	for i, t := range testPasswords {
		c.Logf("test %d", i)
		hashed := utils.CompatPasswordHash(t)
		c.Logf("hash %q", hashed)
		c.Assert(len(hashed), gc.Equals, 24)
		c.Assert(seenHashes[hashed], gc.Equals, false)
		// check we're not adding base64 padding.
		c.Assert(hashed[len(hashed)-1], gc.Not(gc.Equals), '=')
		seenHashes[hashed] = true
		// check it's deterministic
		h1 := utils.CompatPasswordHash(t)
		c.Assert(h1, gc.Equals, hashed)
	}
}

var testSalts = []string{"abcd", "abcdefgh", "abcdefghijklmnop"}

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
			c.Assert(hashed[len(hashed)-1], gc.Not(gc.Equals), '=')
			seenHashes[hashed] = true
			// check it's deterministic
			altHashed := utils.UserPasswordHash(password, salt)
			c.Assert(altHashed, gc.Equals, hashed)
		}
	}
}

func (passwordSuite) TestAgentPasswordHashRefusesShortPasswords(c *gc.C) {
	// The passwords we have been creating have all been 18 bytes of random
	// data base64 encoded into 24 actual bytes.
	_, err := utils.AgentPasswordHash("not quite 24 chars")
	c.Assert(err, gc.ErrorMatches,
		"password is only 18 bytes long, and is not valid as an Agent password")
}

func (passwordSuite) TestAgentPasswordHash(c *gc.C) {
	seenValues := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		password, err := utils.RandomPassword()
		c.Assert(err, gc.IsNil)
		c.Assert(seenValues[password], jc.IsFalse)
		seenValues[password] = true
		hashed, err := utils.AgentPasswordHash(password)
		c.Assert(err, gc.IsNil)
		c.Assert(hashed, gc.Not(gc.Equals), password)
		c.Assert(seenValues[hashed], jc.IsFalse)
		seenValues[hashed] = true
		c.Assert(len(hashed), gc.Equals, 24)
		// check we're not adding base64 padding.
		c.Assert(hashed[len(hashed)-1], gc.Not(gc.Equals), '=')
	}
}
