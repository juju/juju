package version_test

import (
	"."
	. "launchpad.net/gocheck"
	"testing"
)

type suite struct{}

var _ = Suite(suite{})

func Test(t *testing.T) {
	TestingT(t)
}

var cmpTests = []struct {
	v1, v2 string
	less   bool
	eq     bool
}{
	{"1.0.0", "1.0.0", false, true},
	{"01.0.0", "1.0.0", false, true},
	{"10.0.0", "9.0.0", false, false},
	{"1.0.0", "1.0.1", true, false},
	{"1.0.1", "1.0.0", false, false},
	{"1.0.0", "1.1.0", true, false},
	{"1.1.0", "1.0.0", false, false},
	{"1.0.0", "2.0.0", true, false},
	{"2.0.0", "1.0.0", false, false},
}

func (suite) TestComparison(c *C) {
	for i, test := range cmpTests {
		c.Logf("test %d", i)
		v1, err := version.Parse(test.v1)
		c.Assert(err, IsNil)
		v2, err := version.Parse(test.v2)
		c.Assert(err, IsNil)
		less := v1.Less(v2)
		gt := v2.Less(v1)
		c.Check(less, Equals, test.less)
		if test.eq {
			c.Check(gt, Equals, false)
		} else {
			c.Check(gt, Equals, !test.less)
		}
	}
}

var parseTests = []struct {
	v      string
	err    string
	expect version.Version
	dev    bool
}{
	{
		v:   "0.0.0",
		dev: false,
	},
	{
		v:      "1.0.0",
		expect: version.Version{Major: 1},
		dev:    true,
	},
	{
		v:      "0.1.0",
		expect: version.Version{Minor: 1},
		dev:    true,
	},
	{
		v:      "0.0.1",
		expect: version.Version{Patch: 1},
		dev:    true,
	},
	{
		v:      "10.234.3456",
		expect: version.Version{Major: 10, Minor: 234, Patch: 3456},
		dev:    false,
	},
	{
		v:   "1234567890.2.1",
		err: "invalid version.*",
	},
	{
		v:   "0.2..1",
		err: "invalid version.*",
	},
}

func (suite) TestParse(c *C) {
	for i, test := range parseTests {
		c.Logf("test %d", i)
		v, err := version.Parse(test.v)
		if test.err != "" {
			c.Assert(err, ErrorMatches, test.err)
		} else {
			c.Assert(err, IsNil)
			c.Assert(v, Equals, test.expect)
			c.Check(v.IsDev(), Equals, test.dev)
		}
	}
}
