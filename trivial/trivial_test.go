package trivial_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/trivial"
	"strings"
	"testing"
	"time"
)

func Test(t *testing.T) {
	TestingT(t)
}

type trivialSuite struct{}

var _ = Suite(trivialSuite{})

func (trivialSuite) TestAttemptTiming(c *C) {
	const delta = 0.01e9
	testAttempt := trivial.AttemptStrategy{
		Total: 0.25e9,
		Delay: 0.1e9,
	}
	want := []time.Duration{0, 0.1e9, 0.2e9, 0.2e9}
	got := make([]time.Duration, 0, len(want)) // avoid allocation when testing timing
	t0 := time.Now()
	for a := testAttempt.Start(); a.Next(); {
		got = append(got, time.Now().Sub(t0))
	}
	got = append(got, time.Now().Sub(t0))
	c.Assert(got, HasLen, len(want))
	for i, got := range want {
		lo := want[i] - delta
		hi := want[i] + delta
		if got < lo || got > hi {
			c.Errorf("attempt %d want %g got %g", i, want[i].Seconds(), got.Seconds())
		}
	}
}

func (trivialSuite) TestRandomBytes(c *C) {
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

func (trivialSuite) TestRandomPassword(c *C) {
	p, err := trivial.RandomPassword()
	c.Assert(err, IsNil)
	if len(p) < 18 {
		c.Errorf("password too short: %q", p)
	}
	// check we're not adding base64 padding.
	c.Assert(p[len(p)-1], Not(Equals), '=')
}

func (trivialSuite) TestPasswordHash(c *C) {
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

func (trivialSuite) TestCompression(c *C) {
	cdata := trivial.Gzip(data)
	c.Assert(len(cdata) < len(data), Equals, true)
	data1, err := trivial.Gunzip(cdata)
	c.Assert(err, IsNil)
	c.Assert(data1, DeepEquals, data)

	data1, err = trivial.Gunzip(compressedData)
	c.Assert(err, IsNil)
	c.Assert(data1, DeepEquals, data)
}

func (trivialSuite) TestUUID(c *C) {
	uuid, err := trivial.NewUUID()
	c.Assert(err, IsNil)
	uuidCopy := uuid.Copy()
	uuidRaw := uuid.Raw()
	uuidStr := uuid.String()
	c.Assert(uuidRaw, HasLen, 16)
	c.Assert(uuidStr, Matches, "[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[8,9,a,b][0-9a-f]{3}-[0-9a-f]{12}")
	uuid[0] = 0x00
	uuidCopy[0] = 0xFF
	c.Assert(uuid, Not(DeepEquals), uuidCopy)
	uuidRaw[0] = 0xFF
	c.Assert(uuid, Not(DeepEquals), uuidRaw)
	nextUUID, err := trivial.NewUUID()
	c.Assert(err, IsNil)
	c.Assert(uuid, Not(DeepEquals), nextUUID)
}
