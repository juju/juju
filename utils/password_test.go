// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	gc "launchpad.net/gocheck"

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

func (passwordSuite) TestPasswordHash(c *gc.C) {
	hs := make(map[string]bool)
	for i, t := range testPasswords {
		c.Logf("test %d", i)
		h := utils.PasswordHash(t)
		c.Logf("hash %q", h)
		c.Assert(len(h), gc.Equals, 24)
		c.Assert(hs[h], gc.Equals, false)
		// check we're not adding base64 padding.
		c.Assert(h[len(h)-1], gc.Not(gc.Equals), '=')
		hs[h] = true
		// check it's deterministic
		h1 := utils.PasswordHash(t)
		c.Assert(h1, gc.Equals, h)
	}
}

func (passwordSuite) TestSlowPasswordHash(c *gc.C) {
	hs := make(map[string]bool)
	for i, t := range testPasswords {
		c.Logf("test %d", i)
		h := utils.SlowPasswordHash(t)
		c.Logf("hash %q", h)
		c.Assert(len(h), gc.Equals, 24)
		c.Assert(hs[h], gc.Equals, false)
		// check we're not adding base64 padding.
		c.Assert(h[len(h)-1], gc.Not(gc.Equals), '=')
		hs[h] = true
		// check it's deterministic
		h1 := utils.SlowPasswordHash(t)
		c.Assert(h1, gc.Equals, h)
	}
}

