// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	stdtesting "testing"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/environs/tools"
	toolstesting "github.com/juju/juju/environs/tools/testing"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/juju/keys"
)

var live = flag.Bool("live", false, "Include live simplestreams tests")

var vendor = flag.String("vendor", "", "The vendor representing the source of the simplestream data")

type liveTestData struct {
	baseURL        string
	requireSigned  bool
	validCloudSpec simplestreams.CloudSpec
}

func getLiveURLs() (map[string]liveTestData, error) {
	resolver := ec2.NewDefaultEndpointResolver()
	ep, err := resolver.ResolveEndpoint("us-east-1", ec2.EndpointResolverOptions{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	return map[string]liveTestData{
		"ec2": {
			baseURL:       tools.DefaultBaseURL,
			requireSigned: true,
			validCloudSpec: simplestreams.CloudSpec{
				Region:   "us-east-1",
				Endpoint: ep.URL,
			},
		},
		"canonistack": {
			baseURL:       "https://swift.canonistack.canonical.com/v1/AUTH_526ad877f3e3464589dc1145dfeaac60/juju-tools",
			requireSigned: false,
			validCloudSpec: simplestreams.CloudSpec{
				Region:   "lcy01",
				Endpoint: "https://keystone.canonistack.canonical.com:443/v1.0/",
			},
		},
	}, nil
}

func TestSimplestreamsSuite(t *stdtesting.T) {
	tc.Run(t, &simplestreamsSuite{
		LocalLiveSimplestreamsSuite: sstesting.LocalLiveSimplestreamsSuite{
			Source:         sstesting.VerifyDefaultCloudDataSource("test", "test:"),
			RequireSigned:  false,
			DataType:       tools.ContentDownload,
			StreamsVersion: tools.CurrentStreamsVersion,
			ValidConstraint: tools.NewVersionedToolsConstraint(semversion.MustParse("1.13.0"), simplestreams.LookupParams{
				CloudSpec: simplestreams.CloudSpec{
					Region:   "us-east-1",
					Endpoint: "https://ec2.us-east-1.amazonaws.com",
				},
				Releases: []string{"ubuntu"},
				Arches:   []string{"amd64", "arm"},
				Stream:   "released",
			}),
		},
	})
}

type simplestreamsSuite struct {
	sstesting.LocalLiveSimplestreamsSuite
	sstesting.TestDataSuite
}

func (s *simplestreamsSuite) SetUpSuite(c *tc.C) {
	s.LocalLiveSimplestreamsSuite.SetUpSuite(c)
	s.TestDataSuite.SetUpSuite(c)
}

func (s *simplestreamsSuite) TearDownSuite(c *tc.C) {
	s.TestDataSuite.TearDownSuite(c)
	s.LocalLiveSimplestreamsSuite.TearDownSuite(c)
}

var fetchTests = []struct {
	region  string
	osType  string
	version string
	stream  string
	major   int
	minor   int
	arches  []string
	tools   []*tools.ToolsMetadata
}{{
	osType:  "ubuntu",
	arches:  []string{"amd64", "arm"},
	version: "1.13.0",
	tools: []*tools.ToolsMetadata{
		{
			Release:  "ubuntu",
			Version:  "1.13.0",
			Arch:     "amd64",
			Size:     2973595,
			Path:     "tools/released/20130806/juju-1.13.0-ubuntu-amd64.tgz",
			FileType: "tar.gz",
			SHA256:   "447aeb6a934a5eaec4f703eda4ef2dde",
		},
	},
}, {
	osType:  "ubuntu",
	arches:  []string{"amd64", "arm"},
	version: "1.11.4",
	tools: []*tools.ToolsMetadata{
		{
			Release:  "ubuntu",
			Version:  "1.11.4",
			Arch:     "arm",
			Size:     1951096,
			Path:     "tools/released/20130806/juju-1.11.4-ubuntu-arm.tgz",
			FileType: "tar.gz",
			SHA256:   "f65a92b3b41311bdf398663ee1c5cd0c",
		},
	},
}, {
	osType: "ubuntu",
	arches: []string{"amd64", "arm"},
	major:  2,
	tools: []*tools.ToolsMetadata{
		{
			Release:  "ubuntu",
			Version:  "2.0.1",
			Arch:     "arm",
			Size:     1951096,
			Path:     "tools/released/20130806/juju-2.0.1-ubuntu-arm.tgz",
			FileType: "tar.gz",
			SHA256:   "f65a92b3b41311bdf398663ee1c5cd0c",
		},
	},
}, {
	osType: "ubuntu",
	arches: []string{"amd64", "arm"},
	major:  1,
	minor:  11,
	tools: []*tools.ToolsMetadata{
		{
			Release:  "ubuntu",
			Version:  "1.11.4",
			Arch:     "arm",
			Size:     1951096,
			Path:     "tools/released/20130806/juju-1.11.4-ubuntu-arm.tgz",
			FileType: "tar.gz",
			SHA256:   "f65a92b3b41311bdf398663ee1c5cd0c",
		},
		{
			Release:  "ubuntu",
			Version:  "1.11.5",
			Arch:     "arm",
			Size:     2031281,
			Path:     "tools/released/20130803/juju-1.11.5-ubuntu-arm.tgz",
			FileType: "tar.gz",
			SHA256:   "df07ac5e1fb4232d4e9aa2effa57918a",
		},
	},
}, {
	osType:  "ubuntu",
	arches:  []string{"amd64"},
	version: "1.16.0",
	stream:  "testing",
	tools: []*tools.ToolsMetadata{
		{
			Release:  "ubuntu",
			Version:  "1.16.0",
			Arch:     "amd64",
			Size:     2973512,
			Path:     "tools/testing/20130806/juju-1.16.0-ubuntu-amd64.tgz",
			FileType: "tar.gz",
			SHA256:   "447aeb6a934a5eaec4f703eda4ef2dac",
		},
	},
}}

func (s *simplestreamsSuite) TestFetch(c *tc.C) {
	for i, t := range fetchTests {
		c.Logf("test %d", i)
		if t.stream == "" {
			t.stream = "released"
		}
		var toolsConstraint *tools.ToolsConstraint
		if t.version == "" {
			toolsConstraint = tools.NewGeneralToolsConstraint(t.major, t.minor, simplestreams.LookupParams{
				CloudSpec: simplestreams.CloudSpec{"us-east-1", "https://ec2.us-east-1.amazonaws.com"},
				Releases:  []string{t.osType},
				Arches:    t.arches,
				Stream:    t.stream,
			})
		} else {
			toolsConstraint = tools.NewVersionedToolsConstraint(semversion.MustParse(t.version),
				simplestreams.LookupParams{
					CloudSpec: simplestreams.CloudSpec{"us-east-1", "https://ec2.us-east-1.amazonaws.com"},
					Releases:  []string{t.osType},
					Arches:    t.arches,
					Stream:    t.stream,
				})
		}
		// Add invalid datasource and check later that resolveInfo is correct.
		invalidSource := sstesting.InvalidDataSource(s.RequireSigned)
		ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
		toolsMetadata, resolveInfo, err := tools.Fetch(c.Context(), ss, []simplestreams.DataSource{invalidSource, s.Source}, toolsConstraint)
		if !c.Check(err, tc.ErrorIsNil) {
			continue
		}
		for _, tm := range t.tools {
			tm.FullPath, err = s.Source.URL(tm.Path)
			c.Assert(err, tc.ErrorIsNil)
		}
		c.Check(toolsMetadata, tc.DeepEquals, t.tools)
		c.Check(resolveInfo, tc.DeepEquals, &simplestreams.ResolveInfo{
			Source:    "test",
			Signed:    s.RequireSigned,
			IndexURL:  "test:/streams/v1/index.json",
			MirrorURL: "",
		})
	}
}

func (s *simplestreamsSuite) TestFetchNoMatchingStream(c *tc.C) {
	toolsConstraint := tools.NewGeneralToolsConstraint(2, -1, simplestreams.LookupParams{
		CloudSpec: simplestreams.CloudSpec{"us-east-1", "https://ec2.us-east-1.amazonaws.com"},
		Releases:  []string{"ubuntu"},
		Arches:    []string{},
		Stream:    "proposed",
	})
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	_, _, err := tools.Fetch(c.Context(), ss,
		[]simplestreams.DataSource{s.Source}, toolsConstraint)
	c.Assert(err, tc.ErrorMatches, `"content-download" data not found`)
}

func (s *simplestreamsSuite) TestFetchWithMirror(c *tc.C) {
	toolsConstraint := tools.NewGeneralToolsConstraint(1, 13, simplestreams.LookupParams{
		CloudSpec: simplestreams.CloudSpec{"us-west-2", "https://ec2.us-west-2.amazonaws.com"},
		Releases:  []string{"ubuntu"},
		Arches:    []string{"amd64"},
		Stream:    "released",
	})
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	toolsMetadata, resolveInfo, err := tools.Fetch(c.Context(), ss,
		[]simplestreams.DataSource{s.Source}, toolsConstraint)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(toolsMetadata), tc.Equals, 1)

	expectedMetadata := &tools.ToolsMetadata{
		Release:  "ubuntu",
		Version:  "1.13.0",
		Arch:     "amd64",
		Size:     2973595,
		Path:     "mirrored-path/juju-1.13.0-ubuntu-amd64.tgz",
		FullPath: "test:/mirrored-path/juju-1.13.0-ubuntu-amd64.tgz",
		FileType: "tar.gz",
		SHA256:   "447aeb6a934a5eaec4f703eda4ef2dde",
	}
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(toolsMetadata[0], tc.DeepEquals, expectedMetadata)
	c.Assert(resolveInfo, tc.DeepEquals, &simplestreams.ResolveInfo{
		Source:    "test",
		Signed:    s.RequireSigned,
		IndexURL:  "test:/streams/v1/index.json",
		MirrorURL: "test:/",
	})
}

func assertMetadataMatches(c *tc.C, stream string, toolList coretools.List, metadata []*tools.ToolsMetadata) {
	var expectedMetadata = make([]*tools.ToolsMetadata, len(toolList))
	for i, tool := range toolList {
		expectedMetadata[i] = &tools.ToolsMetadata{
			Release:  tool.Version.Release,
			Version:  tool.Version.Number.String(),
			Arch:     tool.Version.Arch,
			Size:     tool.Size,
			Path:     fmt.Sprintf("%s/juju-%s.tgz", stream, tool.Version.String()),
			FileType: "tar.gz",
			SHA256:   tool.SHA256,
		}
	}
	c.Assert(metadata, tc.DeepEquals, expectedMetadata)
}

func (s *simplestreamsSuite) TestWriteMetadataNoFetch(c *tc.C) {
	toolsList := coretools.List{
		{
			Version: semversion.MustParseBinary("1.2.3-ubuntu-amd64"),
			Size:    123,
			SHA256:  "abcd",
		}, {
			Version: semversion.MustParseBinary("2.0.1-windows-amd64"),
			Size:    456,
			SHA256:  "xyz",
		},
	}
	expected := toolsList

	// Add tools with an unknown osType.
	// We need to support this case for times when a new Ubuntu os type
	// is released and jujud does not know about it yet.
	vers, err := semversion.ParseBinary("3.2.1-xuanhuaceratops-amd64")
	c.Assert(err, tc.ErrorIsNil)
	toolsList = append(toolsList, &coretools.Tools{
		Version: vers,
		Size:    456,
		SHA256:  "wqe",
	})

	dir := c.MkDir()
	writer, err := filestorage.NewFileStorageWriter(dir)
	c.Assert(err, tc.ErrorIsNil)
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	err = tools.MergeAndWriteMetadata(c.Context(), ss, writer, "proposed", "proposed", toolsList, tools.DoNotWriteMirrors)
	c.Assert(err, tc.ErrorIsNil)
	metadata := toolstesting.ParseMetadataFromDir(c, dir, "proposed", false)
	assertMetadataMatches(c, "proposed", expected, metadata)
}

func (s *simplestreamsSuite) assertWriteMetadata(c *tc.C, withMirrors bool) {
	var versionStrings = []string{
		"1.2.3-ubuntu-amd64",
		"2.0.1-ubuntu-amd64",
	}
	dir := c.MkDir()
	toolstesting.MakeTools(c, dir, "proposed", versionStrings)

	toolsList := coretools.List{
		{
			// If sha256/size is already known, do not recalculate
			Version: semversion.MustParseBinary("1.2.3-ubuntu-amd64"),
			Size:    123,
			SHA256:  "abcd",
		}, {
			Version: semversion.MustParseBinary("2.0.1-ubuntu-amd64"),
			// The URL is not used for generating metadata.
			URL: "bogus://",
		},
	}
	writer, err := filestorage.NewFileStorageWriter(dir)
	c.Assert(err, tc.ErrorIsNil)
	writeMirrors := tools.DoNotWriteMirrors
	if withMirrors {
		writeMirrors = tools.WriteMirrors
	}
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	err = tools.MergeAndWriteMetadata(c.Context(), ss, writer, "proposed", "proposed", toolsList, writeMirrors)
	c.Assert(err, tc.ErrorIsNil)

	metadata := toolstesting.ParseMetadataFromDir(c, dir, "proposed", withMirrors)
	assertMetadataMatches(c, "proposed", toolsList, metadata)

	// No release stream generated so there will not be a legacy index file created.
	_, err = writer.Get("tools/streams/v1/index.json")
	c.Assert(err, tc.NotNil)
}

func (s *simplestreamsSuite) TestWriteMetadata(c *tc.C) {
	s.assertWriteMetadata(c, false)
}

func (s *simplestreamsSuite) TestWriteMetadataWithMirrors(c *tc.C) {
	s.assertWriteMetadata(c, true)
}

func (s *simplestreamsSuite) TestWriteMetadataMergeWithExisting(c *tc.C) {
	dir := c.MkDir()
	existingToolsList := coretools.List{
		{
			Version: semversion.MustParseBinary("1.2.3-ubuntu-amd64"),
			Size:    123,
			SHA256:  "abc",
		}, {
			Version: semversion.MustParseBinary("2.0.1-ubuntu-amd64"),
			Size:    456,
			SHA256:  "xyz",
		},
	}
	writer, err := filestorage.NewFileStorageWriter(dir)
	c.Assert(err, tc.ErrorIsNil)

	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	err = tools.MergeAndWriteMetadata(c.Context(), ss, writer, "testing", "testing", existingToolsList, tools.WriteMirrors)
	c.Assert(err, tc.ErrorIsNil)

	newToolsList := coretools.List{
		existingToolsList[0],
		{
			Version: semversion.MustParseBinary("2.1.0-ubuntu-amd64"),
			Size:    789,
			SHA256:  "def",
		},
	}
	err = tools.MergeAndWriteMetadata(c.Context(), ss, writer, "testing", "testing", newToolsList, tools.WriteMirrors)
	c.Assert(err, tc.ErrorIsNil)
	requiredToolsList := append(existingToolsList, newToolsList[1])
	metadata := toolstesting.ParseMetadataFromDir(c, dir, "testing", true)
	assertMetadataMatches(c, "testing", requiredToolsList, metadata)

	err = tools.MergeAndWriteMetadata(c.Context(), ss, writer, "devel", "devel", newToolsList, tools.WriteMirrors)
	c.Assert(err, tc.ErrorIsNil)

	metadata = toolstesting.ParseMetadataFromDir(c, dir, "testing", true)
	assertMetadataMatches(c, "testing", requiredToolsList, metadata)

	metadata = toolstesting.ParseMetadataFromDir(c, dir, "devel", true)
	assertMetadataMatches(c, "devel", newToolsList, metadata)
}

type productSpecSuite struct{}

func TestProductSpecSuite(t *stdtesting.T) { tc.Run(t, &productSpecSuite{}) }
func (s *productSpecSuite) TestIndexIdNoStream(c *tc.C) {
	toolsConstraint := tools.NewVersionedToolsConstraint(semversion.MustParse("1.13.0"), simplestreams.LookupParams{
		Releases: []string{"ubuntu"},
		Arches:   []string{"amd64"},
	})
	ids := toolsConstraint.IndexIds()
	c.Assert(ids, tc.HasLen, 0)
}

func (s *productSpecSuite) TestIndexId(c *tc.C) {
	toolsConstraint := tools.NewVersionedToolsConstraint(semversion.MustParse("1.13.0"), simplestreams.LookupParams{
		Releases: []string{"ubuntu"},
		Arches:   []string{"amd64"},
		Stream:   "proposed",
	})
	ids := toolsConstraint.IndexIds()
	c.Assert(ids, tc.DeepEquals, []string{"com.ubuntu.juju:proposed:agents"})
}

func (s *productSpecSuite) TestProductId(c *tc.C) {
	toolsConstraint := tools.NewVersionedToolsConstraint(semversion.MustParse("1.13.0"), simplestreams.LookupParams{
		Releases: []string{"ubuntu"},
		Arches:   []string{"amd64"},
	})
	ids, err := toolsConstraint.ProductIds()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ids, tc.DeepEquals, []string{"com.ubuntu.juju:ubuntu:amd64"})
}

func (s *productSpecSuite) TestIdMultiArch(c *tc.C) {
	toolsConstraint := tools.NewVersionedToolsConstraint(semversion.MustParse("1.11.3"), simplestreams.LookupParams{
		Releases: []string{"ubuntu"},
		Arches:   []string{"amd64", "arm"},
	})
	ids, err := toolsConstraint.ProductIds()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ids, tc.DeepEquals, []string{
		"com.ubuntu.juju:ubuntu:amd64",
		"com.ubuntu.juju:ubuntu:arm"})
}

func (s *productSpecSuite) TestIdMultiOSType(c *tc.C) {
	toolsConstraint := tools.NewVersionedToolsConstraint(semversion.MustParse("1.11.3"), simplestreams.LookupParams{
		Releases: []string{"ubuntu", "windows"},
		Arches:   []string{"amd64"},
		Stream:   "released",
	})
	ids, err := toolsConstraint.ProductIds()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ids, tc.DeepEquals, []string{
		"com.ubuntu.juju:ubuntu:amd64",
		"com.ubuntu.juju:windows:amd64"})
}

func (s *productSpecSuite) TestIdIgnoresInvalidOSType(c *tc.C) {
	toolsConstraint := tools.NewVersionedToolsConstraint(semversion.MustParse("1.11.3"), simplestreams.LookupParams{
		Releases: []string{"ubuntu", "foobar"},
		Arches:   []string{"amd64"},
		Stream:   "released",
	})
	ids, err := toolsConstraint.ProductIds()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ids, tc.DeepEquals, []string{"com.ubuntu.juju:ubuntu:amd64"})
}

func (s *productSpecSuite) TestIdWithMajorVersionOnly(c *tc.C) {
	toolsConstraint := tools.NewGeneralToolsConstraint(1, -1, simplestreams.LookupParams{
		Releases: []string{"ubuntu"},
		Arches:   []string{"amd64"},
		Stream:   "released",
	})
	ids, err := toolsConstraint.ProductIds()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ids, tc.DeepEquals, []string{`com.ubuntu.juju:ubuntu:amd64`})
}

func (s *productSpecSuite) TestIdWithMajorMinorVersion(c *tc.C) {
	toolsConstraint := tools.NewGeneralToolsConstraint(1, 2, simplestreams.LookupParams{
		Releases: []string{"ubuntu"},
		Arches:   []string{"amd64"},
		Stream:   "released",
	})
	ids, err := toolsConstraint.ProductIds()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ids, tc.DeepEquals, []string{`com.ubuntu.juju:ubuntu:amd64`})
}

func (s *productSpecSuite) TestLargeNumber(c *tc.C) {
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
                            "1.10.0-ubuntu-amd64": {
                                "release": "ubuntu",
                                "version": "1.10.0",
                                "arch": "amd64",
                                "size": 9223372036854775807,
                                "path": "releases/juju-1.10.0-ubuntu-amd64.tgz",
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cloudMetadata.Products, tc.HasLen, 1)
	product := cloudMetadata.Products["com.ubuntu.juju:1.10.0:amd64"]
	c.Assert(product, tc.NotNil)
	c.Assert(product.Items, tc.HasLen, 1)
	version := product.Items["20133008"]
	c.Assert(version, tc.NotNil)
	c.Assert(version.Items, tc.HasLen, 1)
	item := version.Items["1.10.0-ubuntu-amd64"]
	c.Assert(item, tc.NotNil)
	c.Assert(item, tc.FitsTypeOf, &tools.ToolsMetadata{})
	c.Assert(item.(*tools.ToolsMetadata).Size, tc.Equals, int64(9223372036854775807))
}

type metadataHelperSuite struct {
	coretesting.BaseSuite
}

func TestMetadataHelperSuite(t *stdtesting.T) { tc.Run(t, &metadataHelperSuite{}) }
func (*metadataHelperSuite) TestMetadataFromTools(c *tc.C) {
	metadata := tools.MetadataFromTools(nil, "proposed")
	c.Assert(metadata, tc.HasLen, 0)

	toolsList := coretools.List{{
		Version: semversion.MustParseBinary("1.2.3-ubuntu-amd64"),
		Size:    123,
		SHA256:  "abc",
	}, {
		Version: semversion.MustParseBinary("2.0.1-ubuntu-amd64"),
		URL:     "file:///tmp/proposed/juju-2.0.1-ubuntu-amd64.tgz",
		Size:    456,
		SHA256:  "xyz",
	}}
	metadata = tools.MetadataFromTools(toolsList, "proposed")
	c.Assert(metadata, tc.HasLen, len(toolsList))
	for i, t := range toolsList {
		md := metadata[i]
		c.Assert(md.Release, tc.Equals, "ubuntu")
		c.Assert(md.Version, tc.Equals, t.Version.Number.String())
		c.Assert(md.Arch, tc.Equals, t.Version.Arch)
		// FullPath is only filled out when reading tools using simplestreams.
		// It's not needed elsewhere and requires a URL() call.
		c.Assert(md.FullPath, tc.Equals, "")
		c.Assert(md.Path, tc.Equals, tools.StorageName(t.Version, "proposed")[len("tools/"):])
		c.Assert(md.FileType, tc.Equals, "tar.gz")
		c.Assert(md.Size, tc.Equals, t.Size)
		c.Assert(md.SHA256, tc.Equals, t.SHA256)
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

func (*metadataHelperSuite) TestResolveMetadata(c *tc.C) {
	var versionStrings = []string{"1.2.3-ubuntu-amd64"}
	dir := c.MkDir()
	toolstesting.MakeTools(c, dir, "released", versionStrings)
	toolsList := coretools.List{{
		Version: semversion.MustParseBinary(versionStrings[0]),
		Size:    123,
		SHA256:  "abc",
	}}

	stor, err := filestorage.NewFileStorageReader(dir)
	c.Assert(err, tc.ErrorIsNil)
	err = tools.ResolveMetadata(stor, "released", nil)
	c.Assert(err, tc.ErrorIsNil)

	// We already have size/sha256, so ensure that storage isn't consulted.
	countingStorage := &countingStorage{StorageReader: stor}
	metadata := tools.MetadataFromTools(toolsList, "released")
	err = tools.ResolveMetadata(countingStorage, "released", metadata)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(countingStorage.counter, tc.Equals, 0)

	// Now clear size/sha256, and check that it is called, and
	// the size/sha256 sum are updated.
	metadata[0].Size = 0
	metadata[0].SHA256 = ""
	err = tools.ResolveMetadata(countingStorage, "released", metadata)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(countingStorage.counter, tc.Equals, 1)
	c.Assert(metadata[0].Size, tc.Not(tc.Equals), 0)
	c.Assert(metadata[0].SHA256, tc.Not(tc.Equals), "")
}

func (*metadataHelperSuite) TestMergeMetadata(c *tc.C) {
	md1 := &tools.ToolsMetadata{
		Release: "ubuntu",
		Version: "1.2.3",
		Arch:    "amd64",
		Path:    "path1",
	}
	md2 := &tools.ToolsMetadata{
		Release: "ubuntu",
		Version: "1.2.3",
		Arch:    "amd64",
		Path:    "path2",
	}
	md3 := &tools.ToolsMetadata{
		Release: "windows",
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
		err:  "metadata mismatch for 1\\.2\\.3-ubuntu-amd64: sizes=\\(123,456\\) sha256=\\(,\\)",
	}, {
		name: "same tools in lhs and rhs, both have same size but different sha256: error",
		lhs:  mdlist{withSHA256(withSize(md1, 123), "a")},
		rhs:  mdlist{withSHA256(withSize(md2, 123), "b")},
		err:  "metadata mismatch for 1\\.2\\.3-ubuntu-amd64: sizes=\\(123,123\\) sha256=\\(a,b\\)",
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
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(merged, tc.DeepEquals, test.merged)
		} else {
			c.Assert(err, tc.ErrorMatches, test.err)
			c.Assert(merged, tc.IsNil)
		}
	}
}

func (*metadataHelperSuite) TestReadWriteMetadataSingleStream(c *tc.C) {
	metadata := map[string][]*tools.ToolsMetadata{
		"released": {{
			Release: "ubuntu",
			Version: "1.2.3",
			Arch:    "amd64",
			Path:    "path1",
		}},
	}

	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	store, err := filestorage.NewFileStorageWriter(c.MkDir())
	c.Assert(err, tc.ErrorIsNil)

	out, err := tools.ReadAllMetadata(c.Context(), ss, store)
	c.Assert(err, tc.ErrorIsNil) // non-existence is not an error
	c.Assert(out, tc.HasLen, 0)

	err = tools.WriteMetadata(store, metadata, []string{"released"}, tools.DoNotWriteMirrors)
	c.Assert(err, tc.ErrorIsNil)

	// Read back what was just written.
	out, err = tools.ReadAllMetadata(c.Context(), ss, store)
	c.Assert(err, tc.ErrorIsNil)
	for _, outMetadata := range out {
		for _, md := range outMetadata {
			// FullPath is set by ReadAllMetadata.
			c.Assert(md.FullPath, tc.Not(tc.Equals), "")
			md.FullPath = ""
		}
	}
	c.Assert(out, tc.DeepEquals, metadata)
}

func (*metadataHelperSuite) writeMetadataMultipleStream(c *tc.C) (*simplestreams.Simplestreams, storage.StorageReader, map[string][]*tools.ToolsMetadata) {
	metadata := map[string][]*tools.ToolsMetadata{
		"released": {{
			Release: "ubuntu",
			Version: "1.2.3",
			Arch:    "amd64",
			Path:    "path1",
		}},
		"proposed": {{
			Release: "ubuntu",
			Version: "1.2.3",
			Arch:    "amd64",
			Path:    "path2",
		}},
	}

	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	store, err := filestorage.NewFileStorageWriter(c.MkDir())
	c.Assert(err, tc.ErrorIsNil)

	out, err := tools.ReadAllMetadata(c.Context(), ss, store)
	c.Assert(out, tc.HasLen, 0)
	c.Assert(err, tc.ErrorIsNil) // non-existence is not an error

	err = tools.WriteMetadata(store, metadata, []string{"released", "proposed"}, tools.DoNotWriteMirrors)
	c.Assert(err, tc.ErrorIsNil)
	return ss, store, metadata
}

func (s *metadataHelperSuite) TestReadWriteMetadataMultipleStream(c *tc.C) {
	ss, store, metadata := s.writeMetadataMultipleStream(c)
	// Read back what was just written.
	out, err := tools.ReadAllMetadata(c.Context(), ss, store)
	c.Assert(err, tc.ErrorIsNil)
	for _, outMetadata := range out {
		for _, md := range outMetadata {
			// FullPath is set by ReadAllMetadata.
			c.Assert(md.FullPath, tc.Not(tc.Equals), "")
			md.FullPath = ""
		}
	}
	c.Assert(out, tc.DeepEquals, metadata)
}

func (s *metadataHelperSuite) TestWriteMetadataLegacyIndex(c *tc.C) {
	_, stor, _ := s.writeMetadataMultipleStream(c)
	// Read back the legacy index
	rdr, err := stor.Get("tools/streams/v1/index.json")
	defer rdr.Close()
	c.Assert(err, tc.ErrorIsNil)
	data, err := io.ReadAll(rdr)
	c.Assert(err, tc.ErrorIsNil)
	var indices simplestreams.Indices
	err = json.Unmarshal(data, &indices)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(indices.Indexes, tc.HasLen, 1)
	indices.Updated = ""
	c.Assert(indices.Indexes["com.ubuntu.juju:released:agents"], tc.NotNil)
	indices.Indexes["com.ubuntu.juju:released:agents"].Updated = ""
	expected := simplestreams.Indices{
		Format: "index:1.0",
		Indexes: map[string]*simplestreams.IndexMetadata{
			"com.ubuntu.juju:released:agents": {
				Format:           "products:1.0",
				DataType:         "content-download",
				ProductsFilePath: "streams/v1/com.ubuntu.juju-released-agents.json",
				ProductIds:       []string{"com.ubuntu.juju:ubuntu:amd64"},
			},
		},
	}
	c.Assert(indices, tc.DeepEquals, expected)
}

func (s *metadataHelperSuite) TestReadWriteMetadataUnchanged(c *tc.C) {
	metadata := map[string][]*tools.ToolsMetadata{
		"released": {{
			Release: "ubuntu",
			Version: "1.2.3",
			Arch:    "amd64",
			Path:    "path1",
		}, {
			Release: "ubuntu",
			Version: "1.2.4",
			Arch:    "amd64",
			Path:    "path2",
		}},
	}

	stor, err := filestorage.NewFileStorageWriter(c.MkDir())
	c.Assert(err, tc.ErrorIsNil)
	err = tools.WriteMetadata(stor, metadata, []string{"released"}, tools.DoNotWriteMirrors)
	c.Assert(err, tc.ErrorIsNil)

	s.PatchValue(tools.WriteMetadataFiles, func(stor storage.Storage, metadataInfo []tools.MetadataFile) error {
		// The product data is the same, we only write the indices.
		c.Assert(metadataInfo, tc.HasLen, 2)
		c.Assert(metadataInfo[0].Path, tc.Equals, "streams/v1/index2.json")
		c.Assert(metadataInfo[1].Path, tc.Equals, "streams/v1/index.json")
		return nil
	})
	err = tools.WriteMetadata(stor, metadata, []string{"released"}, tools.DoNotWriteMirrors)
	c.Assert(err, tc.ErrorIsNil)
}

func (*metadataHelperSuite) TestReadMetadataPrefersNewIndex(c *tc.C) {
	metadataDir := c.MkDir()

	// Generate metadata and rename index to index.json
	metadata := map[string][]*tools.ToolsMetadata{
		"proposed": {{
			Release: "ubuntu",
			Version: "1.2.3",
			Arch:    "amd64",
			Path:    "path1",
		}},
		"released": {{
			Release: "ubuntu",
			Version: "1.2.3",
			Arch:    "amd64",
			Path:    "path1",
		}},
	}
	store, err := filestorage.NewFileStorageWriter(metadataDir)
	c.Assert(err, tc.ErrorIsNil)
	err = tools.WriteMetadata(store, metadata, []string{"proposed", "released"}, tools.DoNotWriteMirrors)
	c.Assert(err, tc.ErrorIsNil)
	err = os.Rename(
		filepath.Join(metadataDir, "tools", "streams", "v1", "index2.json"),
		filepath.Join(metadataDir, "tools", "streams", "v1", "index.json"),
	)
	c.Assert(err, tc.ErrorIsNil)

	// Generate different metadata with index2.json
	metadata = map[string][]*tools.ToolsMetadata{
		"released": {{
			Release: "ubuntu",
			Version: "1.2.3",
			Arch:    "amd64",
			Path:    "path1",
		}},
	}
	err = tools.WriteMetadata(store, metadata, []string{"released"}, tools.DoNotWriteMirrors)
	c.Assert(err, tc.ErrorIsNil)

	// Read back all metadata, expecting to find metadata in index2.json.
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	out, err := tools.ReadAllMetadata(c.Context(), ss, store)
	c.Assert(err, tc.ErrorIsNil)
	for _, outMetadata := range out {
		for _, md := range outMetadata {
			// FullPath is set by ReadAllMetadata.
			c.Assert(md.FullPath, tc.Not(tc.Equals), "")
			md.FullPath = ""
		}
	}
	c.Assert(out, tc.DeepEquals, metadata)
}

type signedSuite struct {
	coretesting.BaseSuite
}

func TestSignedSuite(t *stdtesting.T) {
	tc.Run(t, &signedSuite{})
}

func (s *signedSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
}

func (s *signedSuite) TearDownSuite(c *tc.C) {
	s.BaseSuite.TearDownSuite(c)
}

func (s *signedSuite) TestSignedToolsMetadata(c *tc.C) {
	ts := httptest.NewServer(&sstreamsHandler{})
	defer ts.Close()
	signedSource := simplestreams.NewDataSource(
		simplestreams.Config{
			Description:          "test",
			BaseURL:              fmt.Sprintf("%s/signed", ts.URL),
			PublicSigningKey:     sstesting.SignedMetadataPublicKey,
			HostnameVerification: true,
			Priority:             simplestreams.DEFAULT_CLOUD_DATA,
			RequireSigned:        true,
		},
	)
	toolsConstraint := tools.NewVersionedToolsConstraint(semversion.MustParse("1.13.0"), simplestreams.LookupParams{
		CloudSpec: simplestreams.CloudSpec{"us-east-1", "https://ec2.us-east-1.amazonaws.com"},
		Releases:  []string{"ubuntu"},
		Arches:    []string{"amd64"},
		Stream:    "released",
	})
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	toolsMetadata, resolveInfo, err := tools.Fetch(c.Context(), ss,
		[]simplestreams.DataSource{signedSource}, toolsConstraint)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(toolsMetadata), tc.Equals, 1)
	c.Assert(toolsMetadata[0].Path, tc.Equals, "tools/releases/20130806/juju-1.13.1-ubuntu-amd64.tgz")
	c.Assert(resolveInfo, tc.DeepEquals, &simplestreams.ResolveInfo{
		Source:    "test",
		Signed:    true,
		IndexURL:  fmt.Sprintf("%s/signed/streams/v1/index.sjson", ts.URL),
		MirrorURL: "",
	})
}

type sstreamsHandler struct{}

func (h *sstreamsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/unsigned/streams/v1/index.json":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, unsignedIndex)
	case "/unsigned/streams/v1/image_metadata.json":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, unsignedProduct)
	case "/signed/streams/v1/tools_metadata.sjson":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		rawUnsignedProduct := strings.Replace(
			unsignedProduct, "juju-1.13.0", "juju-1.13.1", -1)
		_, _ = io.WriteString(w, encode(rawUnsignedProduct))
		return
	case "/signed/streams/v1/index.sjson":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		rawUnsignedIndex := strings.Replace(
			unsignedIndex, "streams/v1/tools_metadata.json", "streams/v1/tools_metadata.sjson", -1)
		_, _ = io.WriteString(w, encode(rawUnsignedIndex))
		return
	default:
		http.Error(w, r.URL.Path, 404)
		return
	}
}

func encode(data string) string {
	reader := bytes.NewReader([]byte(data))
	signedData, _ := simplestreams.Encode(
		reader, sstesting.SignedMetadataPrivateKey, sstesting.PrivateKeyPassphrase)
	return string(signedData)
}

var unsignedIndex = `
{
 "index": {
  "com.ubuntu.juju:released:agents": {
   "updated": "Mon, 05 Aug 2013 11:07:04 +0000",
   "datatype": "content-download",
   "format": "products:1.0",
   "products": [
     "com.ubuntu.juju:ubuntu:amd64"
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
 "content_id": "com.ubuntu.juju:released:aws",
 "datatype": "content-download",
 "products": {
   "com.ubuntu.juju:ubuntu:amd64": {
    "arch": "amd64",
    "release": "ubuntu",
    "versions": {
     "20130806": {
      "items": {
       "1130ubuntuamd64": {
        "version": "1.13.0",
        "size": 2973595,
        "path": "tools/releases/20130806/juju-1.13.0-ubuntu-amd64.tgz",
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
