// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"flag"
	"reflect"
	"testing"

	"launchpad.net/goamz/aws"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/simplestreams"
	sstesting "launchpad.net/juju-core/environs/simplestreams/testing"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/version"
)

var live = flag.Bool("live", false, "Include live simplestreams tests")
var vendor = flag.String("vendor", "", "The vendor representing the source of the simplestream data")

type liveTestData struct {
	baseURL        string
	requireSigned  bool
	validCloudSpec simplestreams.CloudSpec
}

var liveUrls = map[string]liveTestData{
	"ec2": {
		baseURL:        tools.DefaultBaseURL,
		requireSigned:  true,
		validCloudSpec: simplestreams.CloudSpec{"us-east-1", aws.Regions["us-east-1"].EC2Endpoint},
	},
	"canonistack": {
		baseURL:        "https://swift.canonistack.canonical.com/v1/AUTH_526ad877f3e3464589dc1145dfeaac60/juju-tools",
		requireSigned:  false,
		validCloudSpec: simplestreams.CloudSpec{"lcy01", "https://keystone.canonistack.canonical.com:443/v2.0/"},
	},
}

func setupSimpleStreamsTests(t *testing.T) {
	if *live {
		if *vendor == "" {
			t.Fatal("missing vendor")
		}
		var ok bool
		var testData liveTestData
		if testData, ok = liveUrls[*vendor]; !ok {
			keys := reflect.ValueOf(liveUrls).MapKeys()
			t.Fatalf("Unknown vendor %s. Must be one of %s", *vendor, keys)
		}
		registerLiveSimpleStreamsTests(testData.baseURL,
			tools.NewVersionedToolsConstraint("1.13.0", simplestreams.LookupParams{
				CloudSpec: testData.validCloudSpec,
				Series:    version.Current.Series,
				Arches:    []string{"amd64"},
			}), testData.requireSigned)
	}
	registerSimpleStreamsTests()
}

func registerSimpleStreamsTests() {
	gc.Suite(&simplestreamsSuite{
		LocalLiveSimplestreamsSuite: sstesting.LocalLiveSimplestreamsSuite{
			BaseURL:       "test:",
			RequireSigned: false,
			DataType:      tools.ContentDownload,
			ValidConstraint: tools.NewVersionedToolsConstraint("1.13.0", simplestreams.LookupParams{
				CloudSpec: simplestreams.CloudSpec{
					Region:   "us-east-1",
					Endpoint: "https://ec2.us-east-1.amazonaws.com",
				},
				Series: "precise",
				Arches: []string{"amd64", "arm"},
			}),
		},
	})
}

func registerLiveSimpleStreamsTests(baseURL string, validToolsConstraint simplestreams.LookupConstraint, requireSigned bool) {
	gc.Suite(&sstesting.LocalLiveSimplestreamsSuite{
		BaseURL:         baseURL,
		RequireSigned:   requireSigned,
		DataType:        tools.ContentDownload,
		ValidConstraint: validToolsConstraint,
	})
}

type simplestreamsSuite struct {
	sstesting.LocalLiveSimplestreamsSuite
	sstesting.TestDataSuite
}

func (s *simplestreamsSuite) SetUpSuite(c *gc.C) {
	s.LocalLiveSimplestreamsSuite.SetUpSuite(c)
	s.TestDataSuite.SetUpSuite(c)
}

func (s *simplestreamsSuite) TearDownSuite(c *gc.C) {
	s.TestDataSuite.TearDownSuite(c)
	s.LocalLiveSimplestreamsSuite.TearDownSuite(c)
}

var fetchTests = []struct {
	region  string
	series  string
	version string
	major   int
	minor   int
	released bool
	arches  []string
	tools   []*tools.ToolsMetadata
}{{
	series:  "precise",
	arches:  []string{"amd64", "arm"},
	version: "1.13.0",
	tools: []*tools.ToolsMetadata{
		{
			Release:  "precise",
			Version:  "1.13.0",
			Arch:     "amd64",
			Size:     2973595,
			Path:     "tools/releases/20130806/juju-1.13.0-precise-amd64.tgz",
			FileType: "tar.gz",
			SHA256:   "447aeb6a934a5eaec4f703eda4ef2dde",
		},
	},
}, {
	series:  "raring",
	arches:  []string{"amd64", "arm"},
	version: "1.13.0",
	tools: []*tools.ToolsMetadata{
		{
			Release:  "raring",
			Version:  "1.13.0",
			Arch:     "amd64",
			Size:     2973173,
			Path:     "tools/releases/20130806/juju-1.13.0-raring-amd64.tgz",
			FileType: "tar.gz",
			SHA256:   "df07ac5e1fb4232d4e9aa2effa57918a",
		},
	},
}, {
	series:  "raring",
	arches:  []string{"amd64", "arm"},
	version: "1.11.4",
	tools: []*tools.ToolsMetadata{
		{
			Release:  "raring",
			Version:  "1.11.4",
			Arch:     "arm",
			Size:     1950327,
			Path:     "tools/releases/20130806/juju-1.11.4-raring-arm.tgz",
			FileType: "tar.gz",
			SHA256:   "6472014e3255e3fe7fbd3550ef3f0a11",
		},
	},
}, {
	series: "precise",
	arches: []string{"amd64", "arm"},
	major:  2,
	tools: []*tools.ToolsMetadata{
		{
			Release:  "precise",
			Version:  "2.0.1",
			Arch:     "arm",
			Size:     1951096,
			Path:     "tools/releases/20130806/juju-2.0.1-precise-arm.tgz",
			FileType: "tar.gz",
			SHA256:   "f65a92b3b41311bdf398663ee1c5cd0c",
		},
	},
}, {
	series: "precise",
	arches: []string{"amd64", "arm"},
	major:  1,
	minor:  11,
	tools: []*tools.ToolsMetadata{
		{
			Release:  "precise",
			Version:  "1.11.4",
			Arch:     "arm",
			Size:     1951096,
			Path:     "tools/releases/20130806/juju-1.11.4-precise-arm.tgz",
			FileType: "tar.gz",
			SHA256:   "f65a92b3b41311bdf398663ee1c5cd0c",
		},
		{
			Release:  "precise",
			Version:  "1.11.5",
			Arch:     "arm",
			Size:     2031281,
			Path:     "tools/releases/20130803/juju-1.11.5-precise-arm.tgz",
			FileType: "tar.gz",
			SHA256:   "df07ac5e1fb4232d4e9aa2effa57918a",
		},
	},
}, {
	series:  "raring",
	arches:  []string{"amd64", "arm"},
	major:  1,
	minor: -1,
	released: true,
	tools: []*tools.ToolsMetadata{
		{
			Release:  "raring",
			Version:  "1.14.0",
			Arch:     "amd64",
			Size:     2973173,
			Path:     "tools/releases/20130806/juju-1.14.0-raring-amd64.tgz",
			FileType: "tar.gz",
			SHA256:   "df07ac5e1fb4232d4e9aa2effa57918a",
		},
	},
}}

func (s *simplestreamsSuite) TestFetch(c *gc.C) {
	for i, t := range fetchTests {
		c.Logf("test %d", i)
		var toolsConstraint *tools.ToolsConstraint
		if t.version == "" {
			toolsConstraint = tools.NewGeneralToolsConstraint(t.major, t.minor, t.released, simplestreams.LookupParams{
				CloudSpec: simplestreams.CloudSpec{"us-east-1", "https://ec2.us-east-1.amazonaws.com"},
				Series:    t.series,
				Arches:    t.arches,
			})
		} else {
			toolsConstraint = tools.NewVersionedToolsConstraint(t.version, simplestreams.LookupParams{
				CloudSpec: simplestreams.CloudSpec{"us-east-1", "https://ec2.us-east-1.amazonaws.com"},
				Series:    t.series,
				Arches:    t.arches,
			})
		}
		tools, err := tools.Fetch([]string{s.BaseURL}, simplestreams.DefaultIndexPath, toolsConstraint, s.RequireSigned)
		if !c.Check(err, gc.IsNil) {
			continue
		}
		c.Check(tools, gc.DeepEquals, t.tools)
	}
}

type productSpecSuite struct{}

var _ = gc.Suite(&productSpecSuite{})

func (s *productSpecSuite) TestId(c *gc.C) {
	toolsConstraint := tools.NewVersionedToolsConstraint("1.13.0", simplestreams.LookupParams{
		Series: "precise",
		Arches: []string{"amd64"},
	})
	ids, err := toolsConstraint.Ids()
	c.Assert(err, gc.IsNil)
	c.Assert(ids, gc.DeepEquals, []string{"com.ubuntu.juju:12.04:amd64"})
}

func (s *productSpecSuite) TestIdMultiArch(c *gc.C) {
	toolsConstraint := tools.NewVersionedToolsConstraint("1.11.3", simplestreams.LookupParams{
		Series: "precise",
		Arches: []string{"amd64", "arm"},
	})
	ids, err := toolsConstraint.Ids()
	c.Assert(err, gc.IsNil)
	c.Assert(ids, gc.DeepEquals, []string{
		"com.ubuntu.juju:12.04:amd64",
		"com.ubuntu.juju:12.04:arm"})
}

func (s *productSpecSuite) TestIdWithNonDefaultRelease(c *gc.C) {
	toolsConstraint := tools.NewVersionedToolsConstraint("1.10.1", simplestreams.LookupParams{
		Series: "lucid",
		Arches: []string{"amd64"},
	})
	ids, err := toolsConstraint.Ids()
	if err != nil && err.Error() == `invalid series "lucid"` {
		c.Fatalf(`Unable to lookup series "lucid", you may need to: apt-get install distro-info`)
	}
	c.Assert(err, gc.IsNil)
	c.Assert(ids, gc.DeepEquals, []string{"com.ubuntu.juju:10.04:amd64"})
}

func (s *productSpecSuite) TestIdWithMajorVersionOnly(c *gc.C) {
	toolsConstraint := tools.NewGeneralToolsConstraint(1, -1, false, simplestreams.LookupParams{
		Series: "precise",
		Arches: []string{"amd64"},
	})
	ids, err := toolsConstraint.Ids()
	c.Assert(err, gc.IsNil)
	c.Assert(ids, gc.DeepEquals, []string{`com.ubuntu.juju:12.04:amd64`})
}

func (s *productSpecSuite) TestIdWithMajorMinorVersion(c *gc.C) {
	toolsConstraint := tools.NewGeneralToolsConstraint(1, 2, false, simplestreams.LookupParams{
		Series: "precise",
		Arches: []string{"amd64"},
	})
	ids, err := toolsConstraint.Ids()
	c.Assert(err, gc.IsNil)
	c.Assert(ids, gc.DeepEquals, []string{`com.ubuntu.juju:12.04:amd64`})
}

func (s *productSpecSuite) TestLargeNumber(c *gc.C) {
	json := `{
        "updated": "Fri, 30 Aug 2013 16:12:58 +0800",
        "format": "products:1.0",
        "products": {
            "com.ubuntu.juju:1.10.0:amd64": {
                "version": "1.10.0",
                "arch": "amd64",
                "versions": {
                    "20133008": {
                        "items": {
                            "1.10.0-precise-amd64": {
                                "release": "precise",
                                "version": "1.10.0",
                                "arch": "amd64",
                                "size": 9223372036854775807,
                                "path": "releases/juju-1.10.0-precise-amd64.tgz",
                                "ftype": "tar.gz",
                                "sha256": ""
                            }
                        }
                    }
                }
            }
        }
    }`
	cloudMetadata, err := simplestreams.ParseCloudMetadata([]byte(json), "products:1.0", "", tools.ToolsMetadata{})
	c.Assert(err, gc.IsNil)
	c.Assert(cloudMetadata.Products, gc.HasLen, 1)
	product := cloudMetadata.Products["com.ubuntu.juju:1.10.0:amd64"]
	c.Assert(product, gc.NotNil)
	c.Assert(product.Items, gc.HasLen, 1)
	version := product.Items["20133008"]
	c.Assert(version, gc.NotNil)
	c.Assert(version.Items, gc.HasLen, 1)
	item := version.Items["1.10.0-precise-amd64"]
	c.Assert(item, gc.NotNil)
	c.Assert(item, gc.FitsTypeOf, &tools.ToolsMetadata{})
	c.Assert(item.(*tools.ToolsMetadata).Size, gc.Equals, int64(9223372036854775807))
}
