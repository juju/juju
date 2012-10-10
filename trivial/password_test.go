package trivial_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/trivial"
)

type PasswordSuite struct{}

var _ = Suite(&PasswordSuite{})

func (PasswordSuite) TestRandomBytes(c *C) {
	b, err := trivial.RandomBytes(16)
	c.Assert(err, IsNil)
	c.Assert(b, HasLen, 16)
	x0 := b[0]
	for _, x := range b {
		if x != x0 {
			return
		}
	}
	c.Errorf("all same bytes in result of RandomBytes")
}

func (PasswordSuite) TestRandomPassword(c *C) {
	p, err := trivial.RandomPassword()
	c.Assert(err, IsNil)
	if len(p) < 18 {
		c.Errorf("password too short: %q", p)
	}
	// check we're not adding base64 padding.
	c.Assert(p[len(p)-1], Not(Equals), '=')
}

func (PasswordSuite) TestPasswordHash(c *C) {
	tests := []string{"", "a", "a longer password than i would usually bother with"}
	hs := make(map[string]bool)
	for i, t := range tests {
		c.Logf("test %d", i)
		h := trivial.PasswordHash(t)
		c.Logf("hash %q", h)
		c.Assert(len(h), Equals, 24)
		c.Assert(hs[h], Equals, false)
		// check we're not adding base64 padding.
		c.Assert(h[len(h)-1], Not(Equals), '=')
		hs[h] = true
		// check it's deterministic
		h1 := trivial.PasswordHash(t)
		c.Assert(h1, Equals, h)
	}
}
