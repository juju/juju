// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"launchpad.net/goamz/aws"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/environs/simplestreams"
	sstesting "launchpad.net/juju-core/environs/simplestreams/testing"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/environs/tools"
	ttesting "launchpad.net/juju-core/environs/tools/testing"
	"launchpad.net/juju-core/testing/testbase"
	coretools "launchpad.net/juju-core/tools"
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
				Series:    []string{version.Current.Series},
				Arches:    []string{"amd64"},
			}), testData.requireSigned)
	}
	registerSimpleStreamsTests()
}

func registerSimpleStreamsTests() {
	gc.Suite(&simplestreamsSuite{
		LocalLiveSimplestreamsSuite: sstesting.LocalLiveSimplestreamsSuite{
			Source:        simplestreams.NewURLDataSource("test:", simplestreams.VerifySSLHostnames),
			RequireSigned: false,
			DataType:      tools.ContentDownload,
			ValidConstraint: tools.NewVersionedToolsConstraint("1.13.0", simplestreams.LookupParams{
				CloudSpec: simplestreams.CloudSpec{
					Region:   "us-east-1",
					Endpoint: "https://ec2.us-east-1.amazonaws.com",
				},
				Series: []string{"precise"},
				Arches: []string{"amd64", "arm"},
			}),
		},
	})
	gc.Suite(&signedSuite{})
}

func registerLiveSimpleStreamsTests(baseURL string, validToolsConstraint simplestreams.LookupConstraint, requireSigned bool) {
	gc.Suite(&sstesting.LocalLiveSimplestreamsSuite{
		Source:          simplestreams.NewURLDataSource(baseURL, simplestreams.VerifySSLHostnames),
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
	region   string
	series   string
	version  string
	major    int
	minor    int
	released bool
	arches   []string
	tools    []*tools.ToolsMetadata
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
	series:   "raring",
	arches:   []string{"amd64", "arm"},
	major:    1,
	minor:    -1,
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
				Series:    []string{t.series},
				Arches:    t.arches,
			})
		} else {
			toolsConstraint = tools.NewVersionedToolsConstraint(t.version, simplestreams.LookupParams{
				CloudSpec: simplestreams.CloudSpec{"us-east-1", "https://ec2.us-east-1.amazonaws.com"},
				Series:    []string{t.series},
				Arches:    t.arches,
			})
		}
		tools, err := tools.Fetch(
			[]simplestreams.DataSource{s.Source}, simplestreams.DefaultIndexPath, toolsConstraint, s.RequireSigned)
		if !c.Check(err, gc.IsNil) {
			continue
		}
		for _, tm := range t.tools {
			tm.FullPath, err = s.Source.URL(tm.Path)
			c.Assert(err, gc.IsNil)
		}
		c.Check(tools, gc.DeepEquals, t.tools)
	}
}

func (s *simplestreamsSuite) TestFetchWithMirror(c *gc.C) {
	toolsConstraint := tools.NewGeneralToolsConstraint(1, 13, false, simplestreams.LookupParams{
		CloudSpec: simplestreams.CloudSpec{"us-west-2", "https://ec2.us-west-2.amazonaws.com"},
		Series:    []string{"precise"},
		Arches:    []string{"amd64"},
	})
	toolsMetadata, err := tools.Fetch(
		[]simplestreams.DataSource{s.Source}, simplestreams.DefaultIndexPath, toolsConstraint, s.RequireSigned)
	c.Assert(err, gc.IsNil)
	c.Assert(len(toolsMetadata), gc.Equals, 1)

	expectedMetadata := &tools.ToolsMetadata{
		Release:  "precise",
		Version:  "1.13.0",
		Arch:     "amd64",
		Size:     2973595,
		Path:     "mirrored-path/juju-1.13.0-precise-amd64.tgz",
		FullPath: "test:/mirrored-path/juju-1.13.0-precise-amd64.tgz",
		FileType: "tar.gz",
		SHA256:   "447aeb6a934a5eaec4f703eda4ef2dde",
	}
	c.Assert(err, gc.IsNil)
	c.Assert(toolsMetadata[0], gc.DeepEquals, expectedMetadata)
}

func assertMetadataMatches(c *gc.C, storageDir string, toolList coretools.List, metadata []*tools.ToolsMetadata) {
	var expectedMetadata []*tools.ToolsMetadata = make([]*tools.ToolsMetadata, len(toolList))
	for i, tool := range toolList {
		expectedMetadata[i] = &tools.ToolsMetadata{
			Release:  tool.Version.Series,
			Version:  tool.Version.Number.String(),
			Arch:     tool.Version.Arch,
			Size:     tool.Size,
			Path:     fmt.Sprintf("releases/juju-%s.tgz", tool.Version.String()),
			FileType: "tar.gz",
			SHA256:   tool.SHA256,
		}
	}
	c.Assert(metadata, gc.DeepEquals, expectedMetadata)
}

func (s *simplestreamsSuite) TestWriteMetadataNoFetch(c *gc.C) {
	toolsList := coretools.List{
		{
			Version: version.MustParseBinary("1.2.3-precise-amd64"),
			Size:    123,
			SHA256:  "abcd",
		}, {
			Version: version.MustParseBinary("2.0.1-raring-amd64"),
			Size:    456,
			SHA256:  "xyz",
		},
	}
	dir := c.MkDir()
	writer, err := filestorage.NewFileStorageWriter(dir, filestorage.UseDefaultTmpDir)
	c.Assert(err, gc.IsNil)
	err = tools.MergeAndWriteMetadata(writer, toolsList, tools.DoNotWriteMirrors)
	c.Assert(err, gc.IsNil)
	metadata := ttesting.ParseMetadata(c, dir, false)
	assertMetadataMatches(c, dir, toolsList, metadata)
}

func (s *simplestreamsSuite) assertWriteMetadata(c *gc.C, withMirrors bool) {
	var versionStrings = []string{
		"1.2.3-precise-amd64",
		"2.0.1-raring-amd64",
	}
	dir := c.MkDir()
	ttesting.MakeTools(c, dir, "releases", versionStrings)

	toolsList := coretools.List{
		{
			// If sha256/size is already known, do not recalculate
			Version: version.MustParseBinary("1.2.3-precise-amd64"),
			Size:    123,
			SHA256:  "abcd",
		}, {
			Version: version.MustParseBinary("2.0.1-raring-amd64"),
			// The URL is not used for generating metadata.
			URL: "bogus://",
		},
	}
	writer, err := filestorage.NewFileStorageWriter(dir, filestorage.UseDefaultTmpDir)
	c.Assert(err, gc.IsNil)
	writeMirrors := tools.DoNotWriteMirrors
	if withMirrors {
		writeMirrors = tools.WriteMirrors
	}
	err = tools.MergeAndWriteMetadata(writer, toolsList, writeMirrors)
	c.Assert(err, gc.IsNil)
	metadata := ttesting.ParseMetadata(c, dir, withMirrors)
	assertMetadataMatches(c, dir, toolsList, metadata)
}

func (s *simplestreamsSuite) TestWriteMetadata(c *gc.C) {
	s.assertWriteMetadata(c, false)
}

func (s *simplestreamsSuite) TestWriteMetadataWithMirrors(c *gc.C) {
	s.assertWriteMetadata(c, true)
}

func (s *simplestreamsSuite) TestWriteMetadataMergeWithExisting(c *gc.C) {
	dir := c.MkDir()
	existingToolsList := coretools.List{
		{
			Version: version.MustParseBinary("1.2.3-precise-amd64"),
			Size:    123,
			SHA256:  "abc",
		}, {
			Version: version.MustParseBinary("2.0.1-raring-amd64"),
			Size:    456,
			SHA256:  "xyz",
		},
	}
	writer, err := filestorage.NewFileStorageWriter(dir, filestorage.UseDefaultTmpDir)
	c.Assert(err, gc.IsNil)
	err = tools.MergeAndWriteMetadata(writer, existingToolsList, tools.DoNotWriteMirrors)
	c.Assert(err, gc.IsNil)
	newToolsList := coretools.List{
		existingToolsList[0],
		{
			Version: version.MustParseBinary("2.1.0-raring-amd64"),
			Size:    789,
			SHA256:  "def",
		},
	}
	err = tools.MergeAndWriteMetadata(writer, newToolsList, tools.DoNotWriteMirrors)
	c.Assert(err, gc.IsNil)
	requiredToolsList := append(existingToolsList, newToolsList[1])
	metadata := ttesting.ParseMetadata(c, dir, false)
	assertMetadataMatches(c, dir, requiredToolsList, metadata)
}

type productSpecSuite struct{}

var _ = gc.Suite(&productSpecSuite{})

func (s *productSpecSuite) TestId(c *gc.C) {
	toolsConstraint := tools.NewVersionedToolsConstraint("1.13.0", simplestreams.LookupParams{
		Series: []string{"precise"},
		Arches: []string{"amd64"},
	})
	ids, err := toolsConstraint.Ids()
	c.Assert(err, gc.IsNil)
	c.Assert(ids, gc.DeepEquals, []string{"com.ubuntu.juju:12.04:amd64"})
}

func (s *productSpecSuite) TestIdMultiArch(c *gc.C) {
	toolsConstraint := tools.NewVersionedToolsConstraint("1.11.3", simplestreams.LookupParams{
		Series: []string{"precise"},
		Arches: []string{"amd64", "arm"},
	})
	ids, err := toolsConstraint.Ids()
	c.Assert(err, gc.IsNil)
	c.Assert(ids, gc.DeepEquals, []string{
		"com.ubuntu.juju:12.04:amd64",
		"com.ubuntu.juju:12.04:arm"})
}

func (s *productSpecSuite) TestIdMultiSeries(c *gc.C) {
	toolsConstraint := tools.NewVersionedToolsConstraint("1.11.3", simplestreams.LookupParams{
		Series: []string{"precise", "raring"},
		Arches: []string{"amd64"},
	})
	ids, err := toolsConstraint.Ids()
	c.Assert(err, gc.IsNil)
	c.Assert(ids, gc.DeepEquals, []string{
		"com.ubuntu.juju:12.04:amd64",
		"com.ubuntu.juju:13.04:amd64"})
}

func (s *productSpecSuite) TestIdWithMajorVersionOnly(c *gc.C) {
	toolsConstraint := tools.NewGeneralToolsConstraint(1, -1, false, simplestreams.LookupParams{
		Series: []string{"precise"},
		Arches: []string{"amd64"},
	})
	ids, err := toolsConstraint.Ids()
	c.Assert(err, gc.IsNil)
	c.Assert(ids, gc.DeepEquals, []string{`com.ubuntu.juju:12.04:amd64`})
}

func (s *productSpecSuite) TestIdWithMajorMinorVersion(c *gc.C) {
	toolsConstraint := tools.NewGeneralToolsConstraint(1, 2, false, simplestreams.LookupParams{
		Series: []string{"precise"},
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

type metadataHelperSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&metadataHelperSuite{})

func (*metadataHelperSuite) TestMetadataFromTools(c *gc.C) {
	metadata := tools.MetadataFromTools(nil)
	c.Assert(metadata, gc.HasLen, 0)

	toolsList := coretools.List{{
		Version: version.MustParseBinary("1.2.3-precise-amd64"),
		Size:    123,
		SHA256:  "abc",
	}, {
		Version: version.MustParseBinary("2.0.1-raring-amd64"),
		URL:     "file:///tmp/releases/juju-2.0.1-raring-amd64.tgz",
		Size:    456,
		SHA256:  "xyz",
	}}
	metadata = tools.MetadataFromTools(toolsList)
	c.Assert(metadata, gc.HasLen, len(toolsList))
	for i, t := range toolsList {
		md := metadata[i]
		c.Assert(md.Release, gc.Equals, t.Version.Series)
		c.Assert(md.Version, gc.Equals, t.Version.Number.String())
		c.Assert(md.Arch, gc.Equals, t.Version.Arch)
		c.Assert(md.FullPath, gc.Equals, t.URL)
		c.Assert(md.Path, gc.Equals, tools.StorageName(t.Version)[len("tools/"):])
		c.Assert(md.FileType, gc.Equals, "tar.gz")
		c.Assert(md.Size, gc.Equals, t.Size)
		c.Assert(md.SHA256, gc.Equals, t.SHA256)
	}
}

type countingStorage struct {
	storage.StorageReader
	counter int
}

func (c *countingStorage) Get(name string) (io.ReadCloser, error) {
	c.counter++
	return c.StorageReader.Get(name)
}

func (*metadataHelperSuite) TestResolveMetadata(c *gc.C) {
	var versionStrings = []string{"1.2.3-precise-amd64"}
	dir := c.MkDir()
	ttesting.MakeTools(c, dir, "releases", versionStrings)
	toolsList := coretools.List{{
		Version: version.MustParseBinary(versionStrings[0]),
		Size:    123,
		SHA256:  "abc",
	}}

	stor, err := filestorage.NewFileStorageReader(dir)
	c.Assert(err, gc.IsNil)
	err = tools.ResolveMetadata(stor, nil)
	c.Assert(err, gc.IsNil)

	// We already have size/sha256, so ensure that storage isn't consulted.
	countingStorage := &countingStorage{StorageReader: stor}
	metadata := tools.MetadataFromTools(toolsList)
	err = tools.ResolveMetadata(countingStorage, metadata)
	c.Assert(err, gc.IsNil)
	c.Assert(countingStorage.counter, gc.Equals, 0)

	// Now clear size/sha256, and check that it is called, and
	// the size/sha256 sum are updated.
	metadata[0].Size = 0
	metadata[0].SHA256 = ""
	err = tools.ResolveMetadata(countingStorage, metadata)
	c.Assert(err, gc.IsNil)
	c.Assert(countingStorage.counter, gc.Equals, 1)
	c.Assert(metadata[0].Size, gc.Not(gc.Equals), 0)
	c.Assert(metadata[0].SHA256, gc.Not(gc.Equals), "")
}

func (*metadataHelperSuite) TestMergeMetadata(c *gc.C) {
	md1 := &tools.ToolsMetadata{
		Release: "precise",
		Version: "1.2.3",
		Arch:    "amd64",
		Path:    "path1",
	}
	md2 := &tools.ToolsMetadata{
		Release: "precise",
		Version: "1.2.3",
		Arch:    "amd64",
		Path:    "path2",
	}
	md3 := &tools.ToolsMetadata{
		Release: "raring",
		Version: "1.2.3",
		Arch:    "amd64",
		Path:    "path3",
	}

	withSize := func(md *tools.ToolsMetadata, size int64) *tools.ToolsMetadata {
		clone := *md
		clone.Size = size
		return &clone
	}
	withSHA256 := func(md *tools.ToolsMetadata, sha256 string) *tools.ToolsMetadata {
		clone := *md
		clone.SHA256 = sha256
		return &clone
	}

	type mdlist []*tools.ToolsMetadata
	type test struct {
		name             string
		lhs, rhs, merged []*tools.ToolsMetadata
		err              string
	}
	tests := []test{{
		name:   "non-empty lhs, empty rhs",
		lhs:    mdlist{md1},
		rhs:    nil,
		merged: mdlist{md1},
	}, {
		name:   "empty lhs, non-empty rhs",
		lhs:    nil,
		rhs:    mdlist{md2},
		merged: mdlist{md2},
	}, {
		name:   "identical lhs, rhs",
		lhs:    mdlist{md1},
		rhs:    mdlist{md1},
		merged: mdlist{md1},
	}, {
		name:   "same tools in lhs and rhs, neither have size: prefer lhs",
		lhs:    mdlist{md1},
		rhs:    mdlist{md2},
		merged: mdlist{md1},
	}, {
		name:   "same tools in lhs and rhs, only lhs has a size: prefer lhs",
		lhs:    mdlist{withSize(md1, 123)},
		rhs:    mdlist{md2},
		merged: mdlist{withSize(md1, 123)},
	}, {
		name:   "same tools in lhs and rhs, only rhs has a size: prefer rhs",
		lhs:    mdlist{md1},
		rhs:    mdlist{withSize(md2, 123)},
		merged: mdlist{withSize(md2, 123)},
	}, {
		name:   "same tools in lhs and rhs, both have the same size: prefer lhs",
		lhs:    mdlist{withSize(md1, 123)},
		rhs:    mdlist{withSize(md2, 123)},
		merged: mdlist{withSize(md1, 123)},
	}, {
		name: "same tools in lhs and rhs, both have different sizes: error",
		lhs:  mdlist{withSize(md1, 123)},
		rhs:  mdlist{withSize(md2, 456)},
		err:  "metadata mismatch for 1\\.2\\.3-precise-amd64: sizes=\\(123,456\\) sha256=\\(,\\)",
	}, {
		name: "same tools in lhs and rhs, both have same size but different sha256: error",
		lhs:  mdlist{withSHA256(withSize(md1, 123), "a")},
		rhs:  mdlist{withSHA256(withSize(md2, 123), "b")},
		err:  "metadata mismatch for 1\\.2\\.3-precise-amd64: sizes=\\(123,123\\) sha256=\\(a,b\\)",
	}, {
		name:   "lhs is a proper superset of rhs: union of lhs and rhs",
		lhs:    mdlist{md1, md3},
		rhs:    mdlist{md1},
		merged: mdlist{md1, md3},
	}, {
		name:   "rhs is a proper superset of lhs: union of lhs and rhs",
		lhs:    mdlist{md1},
		rhs:    mdlist{md1, md3},
		merged: mdlist{md1, md3},
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.name)
		merged, err := tools.MergeMetadata(test.lhs, test.rhs)
		if test.err == "" {
			c.Assert(err, gc.IsNil)
			c.Assert(merged, gc.DeepEquals, test.merged)
		} else {
			c.Assert(err, gc.ErrorMatches, test.err)
			c.Assert(merged, gc.IsNil)
		}
	}
}

func (*metadataHelperSuite) TestReadWriteMetadata(c *gc.C) {
	metadata := []*tools.ToolsMetadata{{
		Release: "precise",
		Version: "1.2.3",
		Arch:    "amd64",
		Path:    "path1",
	}, {
		Release: "raring",
		Version: "1.2.3",
		Arch:    "amd64",
		Path:    "path2",
	}}

	stor, err := filestorage.NewFileStorageWriter(c.MkDir(), filestorage.UseDefaultTmpDir)
	c.Assert(err, gc.IsNil)
	out, err := tools.ReadMetadata(stor)
	c.Assert(out, gc.HasLen, 0)
	c.Assert(err, gc.IsNil) // non-existence is not an error
	err = tools.WriteMetadata(stor, metadata, tools.DoNotWriteMirrors)
	c.Assert(err, gc.IsNil)
	out, err = tools.ReadMetadata(stor)
	for _, md := range out {
		// FullPath is set by ReadMetadata.
		c.Assert(md.FullPath, gc.Not(gc.Equals), "")
		md.FullPath = ""
	}
	c.Assert(out, gc.DeepEquals, metadata)
}

type signedSuite struct {
	origKey string
}

var testRoundTripper *jujutest.ProxyRoundTripper

func init() {
	testRoundTripper = &jujutest.ProxyRoundTripper{}
	simplestreams.RegisterProtocol("signedtest", testRoundTripper)
}

func (s *signedSuite) SetUpSuite(c *gc.C) {
	var imageData = map[string]string{
		"/unsigned/streams/v1/index.json":          unsignedIndex,
		"/unsigned/streams/v1/tools_metadata.json": unsignedProduct,
	}

	// Set up some signed data from the unsigned data.
	// Overwrite the product path to use the sjson suffix.
	rawUnsignedIndex := strings.Replace(
		unsignedIndex, "streams/v1/tools_metadata.json", "streams/v1/tools_metadata.sjson", -1)
	r := bytes.NewReader([]byte(rawUnsignedIndex))
	signedData, err := simplestreams.Encode(
		r, sstesting.SignedMetadataPrivateKey, sstesting.PrivateKeyPassphrase)
	c.Assert(err, gc.IsNil)
	imageData["/signed/streams/v1/index.sjson"] = string(signedData)

	// Replace the tools path in the unsigned data with a different one so we can test that the right
	// tools path is used.
	rawUnsignedProduct := strings.Replace(
		unsignedProduct, "juju-1.13.0", "juju-1.13.1", -1)
	r = bytes.NewReader([]byte(rawUnsignedProduct))
	signedData, err = simplestreams.Encode(
		r, sstesting.SignedMetadataPrivateKey, sstesting.PrivateKeyPassphrase)
	c.Assert(err, gc.IsNil)
	imageData["/signed/streams/v1/tools_metadata.sjson"] = string(signedData)
	testRoundTripper.Sub = jujutest.NewCannedRoundTripper(
		imageData, map[string]int{"signedtest://unauth": http.StatusUnauthorized})
	s.origKey = tools.SetSigningPublicKey(sstesting.SignedMetadataPublicKey)
}

func (s *signedSuite) TearDownSuite(c *gc.C) {
	testRoundTripper.Sub = nil
	tools.SetSigningPublicKey(s.origKey)
}

func (s *signedSuite) TestSignedToolsMetadata(c *gc.C) {
	signedSource := simplestreams.NewURLDataSource("signedtest://host/signed", simplestreams.VerifySSLHostnames)
	toolsConstraint := tools.NewVersionedToolsConstraint("1.13.0", simplestreams.LookupParams{
		CloudSpec: simplestreams.CloudSpec{"us-east-1", "https://ec2.us-east-1.amazonaws.com"},
		Series:    []string{"precise"},
		Arches:    []string{"amd64"},
	})
	toolsMetadata, err := tools.Fetch(
		[]simplestreams.DataSource{signedSource}, simplestreams.DefaultIndexPath, toolsConstraint, true)
	c.Assert(err, gc.IsNil)
	c.Assert(len(toolsMetadata), gc.Equals, 1)
	c.Assert(toolsMetadata[0].Path, gc.Equals, "tools/releases/20130806/juju-1.13.1-precise-amd64.tgz")
}

var unsignedIndex = `
{
 "index": {
  "com.ubuntu.juju:released:tools": {
   "updated": "Mon, 05 Aug 2013 11:07:04 +0000",
   "datatype": "content-download",
   "format": "products:1.0",
   "products": [
     "com.ubuntu.juju:12.04:amd64"
   ],
   "path": "streams/v1/tools_metadata.json"
  }
 },
 "updated": "Wed, 01 May 2013 13:31:26 +0000",
 "format": "index:1.0"
}
`
var unsignedProduct = `
{
 "updated": "Wed, 01 May 2013 13:31:26 +0000",
 "content_id": "com.ubuntu.cloud:released:aws",
 "datatype": "content-download",
 "products": {
   "com.ubuntu.juju:12.04:amd64": {
    "arch": "amd64",
    "release": "precise",
    "versions": {
     "20130806": {
      "items": {
       "1130preciseamd64": {
        "version": "1.13.0",
        "size": 2973595,
        "path": "tools/releases/20130806/juju-1.13.0-precise-amd64.tgz",
        "ftype": "tar.gz",
        "sha256": "447aeb6a934a5eaec4f703eda4ef2dde"
       }
      }
     }
    }
   }
 },
 "format": "products:1.0"
}
`
