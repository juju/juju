// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"strings"
	"testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/utils"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type utilsSuite struct{}

var _ = gc.Suite(utilsSuite{})

func (utilsSuite) TestRandomBytes(c *gc.C) {
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

func (utilsSuite) TestRandomPassword(c *gc.C) {
	p, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	if len(p) < 18 {
		c.Errorf("password too short: %q", p)
	}
	// check we're not adding base64 padding.
	c.Assert(p[len(p)-1], gc.Not(gc.Equals), '=')
}

var testPasswords = []string{"", "a", "a longer password than i would usually bother with"}

func (utilsSuite) TestPasswordHash(c *gc.C) {
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

func (utilsSuite) TestSlowPasswordHash(c *gc.C) {
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

var (
	data = []byte(strings.Repeat("some data to be compressed\n", 100))
	// compressedData was produced by the gzip command.
	compressedData = []byte{
		0x1f, 0x8b, 0x08, 0x00, 0x33, 0xb5, 0xf6, 0x50,
		0x00, 0x03, 0xed, 0xc9, 0xb1, 0x0d, 0x00, 0x20,
		0x08, 0x45, 0xc1, 0xde, 0x29, 0x58, 0x0d, 0xe5,
		0x97, 0x04, 0x23, 0xee, 0x1f, 0xa7, 0xb0, 0x7b,
		0xd7, 0x5e, 0x57, 0xca, 0xc2, 0xaf, 0xdb, 0x2d,
		0x9b, 0xb2, 0x55, 0xb9, 0x8f, 0xba, 0x15, 0xa3,
		0x29, 0x8a, 0xa2, 0x28, 0x8a, 0xa2, 0x28, 0xea,
		0x67, 0x3d, 0x71, 0x71, 0x6e, 0xbf, 0x8c, 0x0a,
		0x00, 0x00,
	}
)

func (utilsSuite) TestCompression(c *gc.C) {
	cdata := utils.Gzip(data)
	c.Assert(len(cdata) < len(data), gc.Equals, true)
	data1, err := utils.Gunzip(cdata)
	c.Assert(err, gc.IsNil)
	c.Assert(data1, gc.DeepEquals, data)

	data1, err = utils.Gunzip(compressedData)
	c.Assert(err, gc.IsNil)
	c.Assert(data1, gc.DeepEquals, data)
}
