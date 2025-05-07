// Copyright 2025 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package semversion_test

import (
	"encoding/json"
	"strings"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	goyaml "gopkg.in/yaml.v3"

	"github.com/juju/juju/core/semversion"
)

type suite struct{}

var _ = tc.Suite(&suite{})

func (*suite) TestCompare(c *tc.C) {
	cmpTests := []struct {
		v1, v2  string
		compare int
	}{
		{"1.0.0", "1.0.0", 0},
		{"01.0.0", "1.0.0", 0},
		{"10.0.0", "9.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.1.0", -1},
		{"1.1.0", "1.0.0", 1},
		{"1.0.0", "2.0.0", -1},
		{"1.2-alpha1", "1.2.0", -1},
		{"1.2-alpha2", "1.2-alpha1", 1},
		{"1.2-alpha2.1", "1.2-alpha2", 1},
		{"1.2-alpha2.2", "1.2-alpha2.1", 1},
		{"1.2-beta1", "1.2-alpha1", 1},
		{"1.2-beta1", "1.2-alpha2.1", 1},
		{"1.2-beta1", "1.2.0", -1},
		{"1.2.1", "1.2.0", 1},
		{"2.0.0", "1.0.0", 1},
		{"2.0.0.0", "2.0.0", 0},
		{"2.0.0.0", "2.0.0.0", 0},
		{"2.0.0.1", "2.0.0.0", 1},
		{"2.0.1.10", "2.0.0.0", 1},
	}

	for i, test := range cmpTests {
		c.Logf("test %d: %q == %q", i, test.v1, test.v2)
		v1, err := semversion.Parse(test.v1)
		c.Assert(err, jc.ErrorIsNil)
		v2, err := semversion.Parse(test.v2)
		c.Assert(err, jc.ErrorIsNil)
		compare := v1.Compare(v2)
		c.Check(compare, tc.Equals, test.compare)
		// Check that reversing the operands has
		// the expected result.
		compare = v2.Compare(v1)
		c.Check(compare, tc.Equals, -test.compare)
	}
}

func (*suite) TestCompareAfterPatched(c *tc.C) {
	cmpTests := []struct {
		v1, v2  string
		compare int
	}{
		{"1.0.0", "1.0.0", 0},
		{"01.0.0", "1.0.0", 0},
		{"10.0.0", "9.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.1.0", -1},
		{"1.1.0", "1.0.0", 1},
		{"1.0.0", "2.0.0", -1},
		{"1.2-alpha1", "1.2.0", -1},
		{"1.2-alpha2", "1.2-alpha1", 1},
		{"1.2-alpha2.1", "1.2-alpha2", 0},
		{"1.2-alpha2.2", "1.2-alpha2.1", -1},
		{"1.2-beta1", "1.2-alpha1", 1},
		{"1.2-beta1", "1.2-alpha2.1", 1},
		{"1.2-beta1", "1.2.0", -1},
		{"1.2.1", "1.2.0", 1},
		{"2.0.0", "1.0.0", 1},
		{"2.0.0.0", "2.0.0", 0},
		{"2.0.0.0", "2.0.0.0", 0},
		{"2.0.0.1", "2.0.0.0", 0},
		{"2.0.1.10", "2.0.0.0", 1},
	}

	for i, test := range cmpTests {
		c.Logf("test %d: %q == %q", i, test.v1, test.v2)
		v1, err := semversion.Parse(test.v1)
		c.Assert(err, jc.ErrorIsNil)
		v2, err := semversion.Parse(test.v2)
		c.Assert(err, jc.ErrorIsNil)
		compare := v1.ToPatch().Compare(v2)
		c.Check(compare, tc.Equals, test.compare)
		// Check that reversing the operands has
		// the expected result.
		compare = v2.Compare(v1.ToPatch())
		c.Check(compare, tc.Equals, -test.compare)
	}
}

var parseTests = []struct {
	v      string
	err    string
	expect semversion.Number
}{{
	v: "0.0.0",
}, {
	v:      "0.0.1",
	expect: semversion.Number{Major: 0, Minor: 0, Patch: 1},
}, {
	v:      "0.0.2",
	expect: semversion.Number{Major: 0, Minor: 0, Patch: 2},
}, {
	v:      "0.1.0",
	expect: semversion.Number{Major: 0, Minor: 1, Patch: 0},
}, {
	v:      "0.2.3",
	expect: semversion.Number{Major: 0, Minor: 2, Patch: 3},
}, {
	v:      "1.0.0",
	expect: semversion.Number{Major: 1, Minor: 0, Patch: 0},
}, {
	v:      "10.234.3456",
	expect: semversion.Number{Major: 10, Minor: 234, Patch: 3456},
}, {
	v:      "10.234.3456.1",
	expect: semversion.Number{Major: 10, Minor: 234, Patch: 3456, Build: 1},
}, {
	v:      "10.234.3456.64",
	expect: semversion.Number{Major: 10, Minor: 234, Patch: 3456, Build: 64},
}, {
	v:      "10.235.3456",
	expect: semversion.Number{Major: 10, Minor: 235, Patch: 3456},
}, {
	v:      "1.21-alpha1",
	expect: semversion.Number{Major: 1, Minor: 21, Patch: 1, Tag: "alpha"},
}, {
	v:      "1.21-alpha1.1",
	expect: semversion.Number{Major: 1, Minor: 21, Patch: 1, Tag: "alpha", Build: 1},
}, {
	v:      "1.21-alpha1",
	expect: semversion.Number{Major: 1, Minor: 21, Patch: 1, Tag: "alpha", Build: 1}.ToPatch(),
}, {
	v:      "1.21-alpha10",
	expect: semversion.Number{Major: 1, Minor: 21, Patch: 10, Tag: "alpha"},
}, {
	v:      "1.21.0",
	expect: semversion.Number{Major: 1, Minor: 21},
}, {
	v:   "1234567890.2.1",
	err: "invalid version.*",
}, {
	v:   "0.2..1",
	err: "invalid version.*",
}, {
	v:   "1.21.alpha1",
	err: "invalid version.*",
}, {
	v:   "1.21-alpha",
	err: "invalid version.*",
}, {
	v:   "1.21-alpha1beta",
	err: "invalid version.*",
}, {
	v:   "1.21-alpha-dev",
	err: "invalid version.*",
}, {
	v:   "1.21-alpha_dev3",
	err: "invalid version.*",
}, {
	v:   "1.21-alpha123dev3",
	err: "invalid version.*",
}}

func (*suite) TestParse(c *tc.C) {
	for i, test := range parseTests {
		c.Logf("test %d: %q", i, test.v)
		got, err := semversion.Parse(test.v)
		if test.err != "" {
			c.Assert(err, tc.ErrorMatches, test.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(got, tc.Equals, test.expect)
			c.Check(got.String(), tc.Equals, test.v)
		}
	}
}

var parseNonStrictTests = []struct {
	v      string
	err    string
	expect string
}{{
	v:      "1",
	expect: "1.0.0",
}, {
	v:      "1.2",
	expect: "1.2.0",
}, {
	v:      "1.2-alpha",
	expect: "1.2-alpha0",
}, {
	v:   "1.",
	err: "invalid version.*",
}, {
	v:   "1.2-",
	err: "invalid version.*",
}}

func (*suite) TestParseNonStrict(c *tc.C) {
	for i, test := range parseNonStrictTests {
		c.Logf("test %d: %q", i, test.v)
		got, err := semversion.ParseNonStrict(test.v)
		if test.err != "" {
			c.Assert(err, tc.ErrorMatches, test.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Check(got.String(), tc.Equals, test.expect)
		}
	}
}

func binaryVersion(major, minor, patch, build int, tag, series, arch string) semversion.Binary {
	return semversion.Binary{
		Number: semversion.Number{
			Major: major,
			Minor: minor,
			Patch: patch,
			Build: build,
			Tag:   tag,
		},
		Release: series,
		Arch:    arch,
	}
}

func (*suite) TestParseBinary(c *tc.C) {
	parseBinaryTests := []struct {
		v      string
		err    string
		expect semversion.Binary
	}{{
		v:      "1.2.3-ubuntu-amd64",
		expect: binaryVersion(1, 2, 3, 0, "", "ubuntu", "amd64"),
	}, {
		v: "1.2-tag3-windows-amd64",
		expect: semversion.Binary{
			Number: semversion.Number{
				Major: 1,
				Minor: 2,
				Patch: 3,
				Build: 4,
				Tag:   "tag",
			}.ToPatch(),
			Release: "windows",
			Arch:    "amd64",
		},
	}, {
		v:      "1.2.3.4-ubuntu-amd64",
		expect: binaryVersion(1, 2, 3, 4, "", "ubuntu", "amd64"),
	}, {
		v:      "1.2-alpha3-ubuntu-amd64",
		expect: binaryVersion(1, 2, 3, 0, "alpha", "ubuntu", "amd64"),
	}, {
		v:      "1.2-alpha3.4-ubuntu-amd64",
		expect: binaryVersion(1, 2, 3, 4, "alpha", "ubuntu", "amd64"),
	}, {
		v:   "1.2.3",
		err: "invalid binary version.*",
	}, {
		v:   "1.2-beta1",
		err: "invalid binary version.*",
	}, {
		v:   "1.2.3--amd64",
		err: "invalid binary version.*",
	}, {
		v:   "1.2.3-ubuntu-",
		err: "invalid binary version.*",
	}}

	for i, test := range parseBinaryTests {
		c.Logf("first test, %d: %q", i, test.v)
		got, err := semversion.ParseBinary(test.v)
		if test.err != "" {
			c.Assert(err, tc.ErrorMatches, test.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(got, tc.Equals, test.expect)
		}
	}

	for i, test := range parseTests {
		c.Logf("second test, %d: %q", i, test.v)
		v := test.v + "-ubuntu-amd64"
		got, err := semversion.ParseBinary(v)
		expect := semversion.Binary{
			Number:  test.expect,
			Release: "ubuntu",
			Arch:    "amd64",
		}
		if test.err != "" {
			c.Assert(err, tc.ErrorMatches, strings.Replace(test.err, "version", "binary version", 1))
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(got, tc.Equals, expect)
		}
	}
}

var marshallers = []struct {
	name      string
	marshal   func(interface{}) ([]byte, error)
	unmarshal func([]byte, interface{}) error
}{{
	"json",
	json.Marshal,
	json.Unmarshal,
}, {
	"yaml",
	goyaml.Marshal,
	goyaml.Unmarshal,
}}

func (*suite) TestBinaryMarshalUnmarshal(c *tc.C) {
	for _, m := range marshallers {
		c.Logf("encoding %v", m.name)
		type doc struct {
			Version *semversion.Binary
		}
		// Work around goyaml bug #1096149
		// SetYAML is not called for non-pointer fields.
		bp := semversion.MustParseBinary("1.2.3-ubuntu-amd64")
		v := doc{&bp}
		data, err := m.marshal(&v)
		c.Assert(err, jc.ErrorIsNil)
		var bv doc
		err = m.unmarshal(data, &bv)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(bv, tc.DeepEquals, v)
	}
}

func (*suite) TestNumberMarshalUnmarshal(c *tc.C) {
	for _, m := range marshallers {
		c.Logf("encoding %v", m.name)
		type doc struct {
			Version *semversion.Number
		}
		// Work around goyaml bug #1096149
		// SetYAML is not called for non-pointer fields.
		np := semversion.MustParse("1.2.3")
		v := doc{&np}
		data, err := m.marshal(&v)
		c.Assert(err, jc.ErrorIsNil)
		var nv doc
		err = m.unmarshal(data, &nv)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(nv, tc.DeepEquals, v)
	}
}

func (*suite) TestParseMajorMinor(c *tc.C) {
	parseMajorMinorTests := []struct {
		v           string
		err         string
		expectMajor int
		expectMinor int
	}{{
		v:           "1.2",
		expectMajor: 1,
		expectMinor: 2,
	}, {
		v:           "1",
		expectMajor: 1,
		expectMinor: -1,
	}, {
		v:   "1.2.3",
		err: "invalid major.minor version number 1.2.3",
	}, {
		v: "blah",
		// NOTE: This error won't match on <=go1.9
		err: `invalid major version number blah: strconv.Atoi: parsing "blah": invalid syntax`,
	}}

	for i, test := range parseMajorMinorTests {
		c.Logf("test %d", i)
		major, minor, err := semversion.ParseMajorMinor(test.v)
		if test.err != "" {
			c.Check(err, tc.ErrorMatches, test.err)
		} else {
			c.Check(err, jc.ErrorIsNil)
			c.Check(major, tc.Equals, test.expectMajor)
			c.Check(minor, tc.Equals, test.expectMinor)
		}
	}
}
