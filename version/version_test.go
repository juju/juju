// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version_test

import (
	"strings"
	"testing"

	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/version"
)

type suite struct{}

var _ = Suite(suite{})

func Test(t *testing.T) {
	TestingT(t)
}

// N.B. The FORCE-VERSION logic is tested in the environs package.

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
	{"2.0.0.0", "2.0.0", false, true},
	{"2.0.0.0", "2.0.0.0", false, true},
	{"2.0.0.1", "2.0.0.0", false, false},
	{"2.0.1.10", "2.0.0.0", false, false},
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
	expect version.Number
	dev    bool
}{{
	v: "0.0.0",
}, {
	v:      "0.0.1",
	expect: version.Number{0, 0, 1, 0},
}, {
	v:      "0.0.2",
	expect: version.Number{0, 0, 2, 0},
}, {
	v:      "0.1.0",
	expect: version.Number{0, 1, 0, 0},
	dev:    true,
}, {
	v:      "0.2.3",
	expect: version.Number{0, 2, 3, 0},
}, {
	v:      "1.0.0",
	expect: version.Number{1, 0, 0, 0},
}, {
	v:      "10.234.3456",
	expect: version.Number{10, 234, 3456, 0},
}, {
	v:      "10.234.3456.1",
	expect: version.Number{10, 234, 3456, 1},
	dev:    true,
}, {
	v:      "10.234.3456.64",
	expect: version.Number{10, 234, 3456, 64},
	dev:    true,
}, {
	v:      "10.235.3456",
	expect: version.Number{10, 235, 3456, 0},
	dev:    true,
}, {
	v:   "1234567890.2.1",
	err: "invalid version.*",
}, {
	v:   "0.2..1",
	err: "invalid version.*",
}}

func (suite) TestParse(c *C) {
	for i, test := range parseTests {
		c.Logf("test %d", i)
		got, err := version.Parse(test.v)
		if test.err != "" {
			c.Assert(err, ErrorMatches, test.err)
		} else {
			c.Assert(err, IsNil)
			c.Assert(got, Equals, test.expect)
			c.Check(got.IsDev(), Equals, test.dev)
			c.Check(got.String(), Equals, test.v)
		}
	}
}

func binaryVersion(major, minor, patch, build int, series, arch string) version.Binary {
	return version.Binary{
		Number: version.Number{
			Major: major,
			Minor: minor,
			Patch: patch,
			Build: build,
		},
		Series: series,
		Arch:   arch,
	}
}

var parseBinaryTests = []struct {
	v      string
	err    string
	expect version.Binary
}{{
	v:      "1.2.3-a-b",
	expect: binaryVersion(1, 2, 3, 0, "a", "b"),
}, {
	v:      "1.2.3.4-a-b",
	expect: binaryVersion(1, 2, 3, 4, "a", "b"),
}, {
	v:   "1.2.3--b",
	err: "invalid binary version.*",
}, {
	v:   "1.2.3-a-",
	err: "invalid binary version.*",
}}

func (suite) TestParseBinary(c *C) {
	for i, test := range parseBinaryTests {
		c.Logf("test 1: %d", i)
		got, err := version.ParseBinary(test.v)
		if test.err != "" {
			c.Assert(err, ErrorMatches, test.err)
		} else {
			c.Assert(err, IsNil)
			c.Assert(got, Equals, test.expect)
		}
	}

	for i, test := range parseTests {
		c.Logf("test 2: %d", i)
		v := test.v + "-a-b"
		got, err := version.ParseBinary(v)
		expect := version.Binary{
			Number: test.expect,
			Series: "a",
			Arch:   "b",
		}
		if test.err != "" {
			c.Assert(err, ErrorMatches, strings.Replace(test.err, "version", "binary version", 1))
		} else {
			c.Assert(err, IsNil)
			c.Assert(got, Equals, expect)
			c.Check(got.IsDev(), Equals, test.dev)
		}
	}
}

func (suite) TestBinaryBSON(c *C) {
	type doc struct {
		Version version.Binary
	}
	v := doc{version.MustParseBinary("1.2.3-foo-bar")}
	data, err := bson.Marshal(v)
	c.Assert(err, IsNil)
	var nv doc
	err = bson.Unmarshal(data, &nv)
	c.Assert(err, IsNil)
	c.Assert(v, Equals, nv)
}

func (suite) TestNumberBSON(c *C) {
	type doc struct {
		Version version.Number
	}
	v := doc{version.MustParse("1.2.3")}
	data, err := bson.Marshal(&v)
	c.Assert(err, IsNil)
	var nv doc
	err = bson.Unmarshal(data, &nv)
	c.Assert(err, IsNil)
	c.Assert(v, Equals, nv)
}
