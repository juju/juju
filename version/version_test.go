// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version_test

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"runtime"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&suite{})

// N.B. The FORCE-VERSION logic is tested in the environs package.

func (*suite) TestCompare(c *gc.C) {
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
		c.Logf("test %d", i)
		v1, err := version.Parse(test.v1)
		c.Assert(err, jc.ErrorIsNil)
		v2, err := version.Parse(test.v2)
		c.Assert(err, jc.ErrorIsNil)
		compare := v1.Compare(v2)
		c.Check(compare, gc.Equals, test.compare)
		// Check that reversing the operands has
		// the expected result.
		compare = v2.Compare(v1)
		c.Check(compare, gc.Equals, -test.compare)
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
	expect: version.Number{Major: 0, Minor: 0, Patch: 1},
}, {
	v:      "0.0.2",
	expect: version.Number{Major: 0, Minor: 0, Patch: 2},
}, {
	v:      "0.1.0",
	expect: version.Number{Major: 0, Minor: 1, Patch: 0},
	dev:    true,
}, {
	v:      "0.2.3",
	expect: version.Number{Major: 0, Minor: 2, Patch: 3},
}, {
	v:      "1.0.0",
	expect: version.Number{Major: 1, Minor: 0, Patch: 0},
}, {
	v:      "10.234.3456",
	expect: version.Number{Major: 10, Minor: 234, Patch: 3456},
}, {
	v:      "10.234.3456.1",
	expect: version.Number{Major: 10, Minor: 234, Patch: 3456, Build: 1},
	dev:    true,
}, {
	v:      "10.234.3456.64",
	expect: version.Number{Major: 10, Minor: 234, Patch: 3456, Build: 64},
	dev:    true,
}, {
	v:      "10.235.3456",
	expect: version.Number{Major: 10, Minor: 235, Patch: 3456},
}, {
	v:      "1.21-alpha1",
	expect: version.Number{Major: 1, Minor: 21, Patch: 1, Tag: "alpha"},
	dev:    true,
}, {
	v:      "1.21-alpha1.1",
	expect: version.Number{Major: 1, Minor: 21, Patch: 1, Tag: "alpha", Build: 1},
	dev:    true,
}, {
	v:      "1.21.0",
	expect: version.Number{Major: 1, Minor: 21},
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
}}

func (*suite) TestParse(c *gc.C) {
	for i, test := range parseTests {
		c.Logf("test %d", i)
		got, err := version.Parse(test.v)
		if test.err != "" {
			c.Assert(err, gc.ErrorMatches, test.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(got, gc.Equals, test.expect)
			c.Check(got.IsDev(), gc.Equals, test.dev)
			c.Check(got.String(), gc.Equals, test.v)
		}
	}
}

func binaryVersion(major, minor, patch, build int, tag, series, arch string) version.Binary {
	return version.Binary{
		Number: version.Number{
			Major: major,
			Minor: minor,
			Patch: patch,
			Build: build,
			Tag:   tag,
		},
		Series: series,
		Arch:   arch,
		OS:     version.Ubuntu,
	}
}

func (*suite) TestParseBinary(c *gc.C) {
	parseBinaryTests := []struct {
		v      string
		err    string
		expect version.Binary
	}{{
		v:      "1.2.3-trusty-amd64",
		expect: binaryVersion(1, 2, 3, 0, "", "trusty", "amd64"),
	}, {
		v:      "1.2.3.4-trusty-amd64",
		expect: binaryVersion(1, 2, 3, 4, "", "trusty", "amd64"),
	}, {
		v:      "1.2-alpha3-trusty-amd64",
		expect: binaryVersion(1, 2, 3, 0, "alpha", "trusty", "amd64"),
	}, {
		v:      "1.2-alpha3.4-trusty-amd64",
		expect: binaryVersion(1, 2, 3, 4, "alpha", "trusty", "amd64"),
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
		v:   "1.2.3-trusty-",
		err: "invalid binary version.*",
	}}

	for i, test := range parseBinaryTests {
		c.Logf("test 1: %d", i)
		got, err := version.ParseBinary(test.v)
		if test.err != "" {
			c.Assert(err, gc.ErrorMatches, test.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(got, gc.Equals, test.expect)
		}
	}

	for i, test := range parseTests {
		c.Logf("test 2: %d", i)
		v := test.v + "-trusty-amd64"
		got, err := version.ParseBinary(v)
		expect := version.Binary{
			Number: test.expect,
			Series: "trusty",
			Arch:   "amd64",
			OS:     version.Ubuntu,
		}
		if test.err != "" {
			c.Assert(err, gc.ErrorMatches, strings.Replace(test.err, "version", "binary version", 1))
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(got, gc.Equals, expect)
			c.Check(got.IsDev(), gc.Equals, test.dev)
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
	"bson",
	bson.Marshal,
	bson.Unmarshal,
}, {
	"yaml",
	goyaml.Marshal,
	goyaml.Unmarshal,
}}

func (*suite) TestBinaryMarshalUnmarshal(c *gc.C) {
	for _, m := range marshallers {
		c.Logf("encoding %v", m.name)
		type doc struct {
			Version *version.Binary
		}
		// Work around goyaml bug #1096149
		// SetYAML is not called for non-pointer fields.
		bp := version.MustParseBinary("1.2.3-trusty-amd64")
		v := doc{&bp}
		data, err := m.marshal(&v)
		c.Assert(err, jc.ErrorIsNil)
		var bv doc
		err = m.unmarshal(data, &bv)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(bv, gc.DeepEquals, v)
	}
}

func (*suite) TestNumberMarshalUnmarshal(c *gc.C) {
	for _, m := range marshallers {
		c.Logf("encoding %v", m.name)
		type doc struct {
			Version *version.Number
		}
		// Work around goyaml bug #1096149
		// SetYAML is not called for non-pointer fields.
		np := version.MustParse("1.2.3")
		v := doc{&np}
		data, err := m.marshal(&v)
		c.Assert(err, jc.ErrorIsNil)
		var nv doc
		err = m.unmarshal(data, &nv)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(nv, gc.DeepEquals, v)
	}
}

func (*suite) TestParseMajorMinor(c *gc.C) {
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
		v:   "blah",
		err: `invalid major version number blah: strconv.ParseInt: parsing "blah": invalid syntax`,
	}}

	for i, test := range parseMajorMinorTests {
		c.Logf("test %d", i)
		major, minor, err := version.ParseMajorMinor(test.v)
		if test.err != "" {
			c.Check(err, gc.ErrorMatches, test.err)
		} else {
			c.Check(err, jc.ErrorIsNil)
			c.Check(major, gc.Equals, test.expectMajor)
			c.Check(minor, gc.Equals, test.expectMinor)
		}
	}
}

func (s *suite) TestUseFastLXC(c *gc.C) {
	for i, test := range []struct {
		message        string
		releaseContent string
		expected       string
	}{{
		message: "missing release file",
	}, {
		message:        "missing prefix in file",
		releaseContent: "some junk\nand more junk",
	}, {
		message: "precise release",
		releaseContent: `
DISTRIB_ID=Ubuntu
DISTRIB_RELEASE=12.04
DISTRIB_CODENAME=precise
DISTRIB_DESCRIPTION="Ubuntu 12.04.3 LTS"
`,
		expected: "12.04",
	}, {
		message: "trusty release",
		releaseContent: `
DISTRIB_ID=Ubuntu
DISTRIB_RELEASE=14.04
DISTRIB_CODENAME=trusty
DISTRIB_DESCRIPTION="Ubuntu Trusty Tahr (development branch)"
`,
		expected: "14.04",
	}, {
		message:        "minimal trusty release",
		releaseContent: `DISTRIB_RELEASE=14.04`,
		expected:       "14.04",
	}, {
		message:        "minimal unstable unicorn",
		releaseContent: `DISTRIB_RELEASE=14.10`,
		expected:       "14.10",
	}, {
		message:        "minimal jaunty",
		releaseContent: `DISTRIB_RELEASE=9.10`,
		expected:       "9.10",
	}} {
		c.Logf("%v: %v", i, test.message)
		filename := filepath.Join(c.MkDir(), "lsbRelease")
		s.PatchValue(version.LSBReleaseFileVar, filename)
		if test.releaseContent != "" {
			err := ioutil.WriteFile(filename, []byte(test.releaseContent+"\n"), 0644)
			c.Assert(err, jc.ErrorIsNil)
		}
		value := version.ReleaseVersion()
		c.Assert(value, gc.Equals, test.expected)
	}
}

func (s *suite) TestCompiler(c *gc.C) {
	c.Assert(version.Compiler, gc.Equals, runtime.Compiler)
}
