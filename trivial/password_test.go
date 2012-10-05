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

func (PasswordSuite) TestPasswordHash(c *C) {
	tests := []string{"", "a", "a longer password than i would usually bother with"}
	hs := make(map[string]bool)
	for i, t := range tests {
		c.Logf("test %d", i)
		h := trivial.PasswordHash(t)
		c.Logf("hash %q", h)
		c.Assert(hs[h], Equals, false)
		hs[h] = true
		// check it's deterministic
		h1 := trivial.PasswordHash(t)
		c.Assert(h1, Equals, h)
	}
}
