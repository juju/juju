// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"path"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
)

type ValidateSuite struct {
	testing.BaseSuite
	metadataDir string
}

var _ = gc.Suite(&ValidateSuite{})

func (s *ValidateSuite) makeLocalMetadata(c *gc.C, version, series string) error {
	tm := []*ToolsMetadata{{
		Version:  version,
		Release:  series,
		Arch:     "amd64",
		Path:     "/tools/tools.tar.gz",
		Size:     1234,
		FileType: "tar.gz",
		SHA256:   "f65a92b3b41311bdf398663ee1c5cd0c",
	}}

	stor, err := filestorage.NewFileStorageWriter(s.metadataDir)
	c.Assert(err, gc.IsNil)
	err = WriteMetadata(stor, tm, false)
	c.Assert(err, gc.IsNil)
	return nil
}

func (s *ValidateSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.metadataDir = c.MkDir()
}

func (s *ValidateSuite) toolsURL() string {
	return "file://" + path.Join(s.metadataDir, "tools")
}

func (s *ValidateSuite) TestExactVersionMatch(c *gc.C) {
	s.makeLocalMetadata(c, "1.11.2", "raring")
	params := &ToolsMetadataLookupParams{
		Version: "1.11.2",
		MetadataLookupParams: simplestreams.MetadataLookupParams{
			Region:        "region-2",
			Series:        "raring",
			Architectures: []string{"amd64"},
			Endpoint:      "some-auth-url",
			Sources: []simplestreams.DataSource{
				simplestreams.NewURLDataSource("test", s.toolsURL(), utils.VerifySSLHostnames)},
		},
	}
	versions, resolveInfo, err := ValidateToolsMetadata(params)
	c.Assert(err, gc.IsNil)
	c.Assert(versions, gc.DeepEquals, []string{"1.11.2-raring-amd64"})
	c.Check(resolveInfo, gc.DeepEquals, &simplestreams.ResolveInfo{
		Source:    "test",
		Signed:    false,
		IndexURL:  "file://" + path.Join(s.metadataDir, "tools/streams/v1/index.json"),
		MirrorURL: "",
	})
}

func (s *ValidateSuite) TestMajorVersionMatch(c *gc.C) {
	s.makeLocalMetadata(c, "1.11.2", "raring")
	params := &ToolsMetadataLookupParams{
		Major: 1,
		Minor: -1,
		MetadataLookupParams: simplestreams.MetadataLookupParams{
			Region:        "region-2",
			Series:        "raring",
			Architectures: []string{"amd64"},
			Endpoint:      "some-auth-url",
			Sources: []simplestreams.DataSource{
				simplestreams.NewURLDataSource("test", s.toolsURL(), utils.VerifySSLHostnames)},
		},
	}
	versions, resolveInfo, err := ValidateToolsMetadata(params)
	c.Assert(err, gc.IsNil)
	c.Assert(versions, gc.DeepEquals, []string{"1.11.2-raring-amd64"})
	c.Check(resolveInfo, gc.DeepEquals, &simplestreams.ResolveInfo{
		Source:    "test",
		Signed:    false,
		IndexURL:  "file://" + path.Join(s.metadataDir, "tools/streams/v1/index.json"),
		MirrorURL: "",
	})
}

func (s *ValidateSuite) TestMajorMinorVersionMatch(c *gc.C) {
	s.makeLocalMetadata(c, "1.11.2", "raring")
	params := &ToolsMetadataLookupParams{
		Major: 1,
		Minor: 11,
		MetadataLookupParams: simplestreams.MetadataLookupParams{
			Region:        "region-2",
			Series:        "raring",
			Architectures: []string{"amd64"},
			Endpoint:      "some-auth-url",
			Sources: []simplestreams.DataSource{
				simplestreams.NewURLDataSource("test", s.toolsURL(), utils.VerifySSLHostnames)},
		},
	}
	versions, resolveInfo, err := ValidateToolsMetadata(params)
	c.Assert(err, gc.IsNil)
	c.Assert(versions, gc.DeepEquals, []string{"1.11.2-raring-amd64"})
	c.Check(resolveInfo, gc.DeepEquals, &simplestreams.ResolveInfo{
		Source:    "test",
		Signed:    false,
		IndexURL:  "file://" + path.Join(s.metadataDir, "tools/streams/v1/index.json"),
		MirrorURL: "",
	})
}

func (s *ValidateSuite) TestNoMatch(c *gc.C) {
	s.makeLocalMetadata(c, "1.11.2", "raring")
	params := &ToolsMetadataLookupParams{
		Version: "1.11.2",
		MetadataLookupParams: simplestreams.MetadataLookupParams{
			Region:        "region-2",
			Series:        "precise",
			Architectures: []string{"amd64"},
			Endpoint:      "some-auth-url",
			Sources: []simplestreams.DataSource{
				simplestreams.NewURLDataSource("test", s.toolsURL(), utils.VerifySSLHostnames)},
		},
	}
	_, _, err := ValidateToolsMetadata(params)
	c.Assert(err, gc.Not(gc.IsNil))
}
