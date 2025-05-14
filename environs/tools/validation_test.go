// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"path"

	"github.com/juju/tc"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/internal/testing"
)

type ValidateSuite struct {
	testing.BaseSuite
	metadataDir string
	dataSource  simplestreams.DataSource
}

var _ = tc.Suite(&ValidateSuite{})

func (s *ValidateSuite) makeLocalMetadata(c *tc.C, stream, version, osType string) {
	tm := []*ToolsMetadata{{
		Version:  version,
		Release:  osType,
		Arch:     "amd64",
		Path:     "/tools/tools.tar.gz",
		Size:     1234,
		FileType: "tar.gz",
		SHA256:   "f65a92b3b41311bdf398663ee1c5cd0c",
	}}

	stor, err := filestorage.NewFileStorageWriter(s.metadataDir)
	c.Assert(err, tc.ErrorIsNil)
	streamMetadata := map[string][]*ToolsMetadata{
		stream: tm,
	}
	err = WriteMetadata(stor, streamMetadata, []string{stream}, false)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ValidateSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.metadataDir = c.MkDir()
	s.dataSource = sstesting.VerifyDefaultCloudDataSource("test", s.toolsURL())
}

func (s *ValidateSuite) toolsURL() string {
	return utils.MakeFileURL(path.Join(s.metadataDir, "tools"))
}

func (s *ValidateSuite) TestExactVersionMatch(c *tc.C) {
	s.makeLocalMetadata(c, "released", "1.11.2", "ubuntu")
	params := &ToolsMetadataLookupParams{
		Version: "1.11.2",
		MetadataLookupParams: simplestreams.MetadataLookupParams{
			Region:        "region-2",
			Release:       "ubuntu",
			Architectures: []string{"amd64"},
			Endpoint:      "some-auth-url",
			Stream:        "released",
			Sources:       []simplestreams.DataSource{s.dataSource},
		},
	}
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	versions, resolveInfo, err := ValidateToolsMetadata(c.Context(), ss, params)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(versions, tc.DeepEquals, []string{"1.11.2-ubuntu-amd64"})
	c.Check(resolveInfo, tc.DeepEquals, &simplestreams.ResolveInfo{
		Source:    "test",
		Signed:    false,
		IndexURL:  utils.MakeFileURL(path.Join(s.metadataDir, "tools/streams/v1/index2.json")),
		MirrorURL: "",
	})
}

func (s *ValidateSuite) TestMajorVersionMatch(c *tc.C) {
	s.makeLocalMetadata(c, "released", "1.11.2", "ubuntu")
	params := &ToolsMetadataLookupParams{
		Major: 1,
		Minor: -1,
		MetadataLookupParams: simplestreams.MetadataLookupParams{
			Region:        "region-2",
			Release:       "ubuntu",
			Architectures: []string{"amd64"},
			Endpoint:      "some-auth-url",
			Stream:        "released",
			Sources:       []simplestreams.DataSource{s.dataSource},
		},
	}
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	versions, resolveInfo, err := ValidateToolsMetadata(c.Context(), ss, params)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(versions, tc.DeepEquals, []string{"1.11.2-ubuntu-amd64"})
	c.Check(resolveInfo, tc.DeepEquals, &simplestreams.ResolveInfo{
		Source:    "test",
		Signed:    false,
		IndexURL:  utils.MakeFileURL(path.Join(s.metadataDir, "tools/streams/v1/index2.json")),
		MirrorURL: "",
	})
}

func (s *ValidateSuite) TestMajorMinorVersionMatch(c *tc.C) {
	s.makeLocalMetadata(c, "released", "1.11.2", "ubuntu")
	params := &ToolsMetadataLookupParams{
		Major: 1,
		Minor: 11,
		MetadataLookupParams: simplestreams.MetadataLookupParams{
			Region:        "region-2",
			Release:       "ubuntu",
			Architectures: []string{"amd64"},
			Endpoint:      "some-auth-url",
			Stream:        "released",
			Sources:       []simplestreams.DataSource{s.dataSource}},
	}
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	versions, resolveInfo, err := ValidateToolsMetadata(c.Context(), ss, params)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(versions, tc.DeepEquals, []string{"1.11.2-ubuntu-amd64"})
	c.Check(resolveInfo, tc.DeepEquals, &simplestreams.ResolveInfo{
		Source:    "test",
		Signed:    false,
		IndexURL:  utils.MakeFileURL(path.Join(s.metadataDir, "tools/streams/v1/index2.json")),
		MirrorURL: "",
	})
}

func (s *ValidateSuite) TestNoMatch(c *tc.C) {
	s.makeLocalMetadata(c, "released", "1.11.2", "ubuntu")
	params := &ToolsMetadataLookupParams{
		Version: "1.11.2",
		MetadataLookupParams: simplestreams.MetadataLookupParams{
			Region:        "region-2",
			Release:       "precise",
			Architectures: []string{"amd64"},
			Endpoint:      "some-auth-url",
			Stream:        "released",
			Sources:       []simplestreams.DataSource{s.dataSource}},
	}
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	_, _, err := ValidateToolsMetadata(c.Context(), ss, params)
	c.Assert(err, tc.Not(tc.IsNil))
}

func (s *ValidateSuite) TestStreamsNoMatch(c *tc.C) {
	s.makeLocalMetadata(c, "proposed", "1.11.2", "ubuntu")
	params := &ToolsMetadataLookupParams{
		Version: "1.11.2",
		MetadataLookupParams: simplestreams.MetadataLookupParams{
			Region:        "region-2",
			Release:       "ubuntu",
			Architectures: []string{"amd64"},
			Endpoint:      "some-auth-url",
			Stream:        "testing",
			Sources:       []simplestreams.DataSource{s.dataSource}},
	}
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	_, _, err := ValidateToolsMetadata(c.Context(), ss, params)
	c.Assert(err, tc.Not(tc.IsNil))
}
