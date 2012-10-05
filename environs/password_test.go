package environs_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
)

type PasswordSuite struct{}

var _ = Suite(&PasswordSuite{})

func (PasswordSuite) TestRandomBytes(c *C) {
	b, err := environs.RandomBytes(16)
	c.Assert(err, IsNil)
	c.Assert(b, HasLen, 16)
	for _, x := range b {
		if x != 0 {
			return
		}
	}
	c.Errorf("all zero bytes in result of RandomBytes")
}

func (PasswordSuite) TestPasswordHash(c *C) {
	tests := []struct {
		salt, pass string
	}{{
		"", "",
	}, {
		"xxxxxxxx", "a",
	}, {
		"xxxxxxxy", "a",
	}, {
		"a", "a longer password than i would usually bother with",
	}}

	hs := make(map[string]bool)

	for i, t := range tests {
		c.Logf("test %d", i)
		h := environs.PasswordHash([]byte(t.salt), t.pass)
		c.Logf("hash %q", h)
		c.Assert(hs[h], Equals, false)
		hs[h] = true
		// check it's deterministic
		h1 := environs.PasswordHash([]byte(t.salt), t.pass)
		c.Assert(h1, Equals, h)
	}
}
