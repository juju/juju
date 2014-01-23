// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/juju/osenv"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
)

type ValidateSuite struct {
	testbase.LoggingSuite
	home *coretesting.FakeHome
}

var _ = gc.Suite(&ValidateSuite{})

func (s *ValidateSuite) makeLocalMetadata(c *gc.C, version, region, series, endpoint string) error {
	tm := ToolsMetadata{
		Version:  version,
		Release:  series,
		Arch:     "amd64",
		Path:     "/tools/tools.tar.gz",
		Size:     1234,
		FileType: "tar.gz",
		SHA256:   "f65a92b3b41311bdf398663ee1c5cd0c",
	}
	cloudSpec := simplestreams.CloudSpec{
		Region:   region,
		Endpoint: endpoint,
	}
	_, err := MakeBoilerplate(&tm, &cloudSpec, false)
	if err != nil {
		return err
	}
	return nil
}

func (s *ValidateSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.home = coretesting.MakeEmptyFakeHome(c)
}

func (s *ValidateSuite) TearDownTest(c *gc.C) {
	s.home.Restore()
	s.LoggingSuite.TearDownTest(c)
}

func (s *ValidateSuite) TestExactVersionMatch(c *gc.C) {
	s.makeLocalMetadata(c, "1.11.2", "region-2", "raring", "some-auth-url")
	metadataDir := osenv.JujuHomePath("")
	params := &ToolsMetadataLookupParams{
		Version: "1.11.2",
		MetadataLookupParams: simplestreams.MetadataLookupParams{
			Region:        "region-2",
			Series:        "raring",
			Architectures: []string{"amd64"},
			Endpoint:      "some-auth-url",
			Sources:       []simplestreams.DataSource{simplestreams.NewURLDataSource("file://"+metadataDir, simplestreams.VerifySSLHostnames)},
		},
	}
	versions, err := ValidateToolsMetadata(params)
	c.Assert(err, gc.IsNil)
	c.Assert(versions, gc.DeepEquals, []string{"1.11.2-raring-amd64"})
}

func (s *ValidateSuite) TestMajorVersionMatch(c *gc.C) {
	s.makeLocalMetadata(c, "1.11.2", "region-2", "raring", "some-auth-url")
	metadataDir := osenv.JujuHomePath("")
	params := &ToolsMetadataLookupParams{
		Major: 1,
		Minor: -1,
		MetadataLookupParams: simplestreams.MetadataLookupParams{
			Region:        "region-2",
			Series:        "raring",
			Architectures: []string{"amd64"},
			Endpoint:      "some-auth-url",
			Sources:       []simplestreams.DataSource{simplestreams.NewURLDataSource("file://"+metadataDir, simplestreams.VerifySSLHostnames)},
		},
	}
	versions, err := ValidateToolsMetadata(params)
	c.Assert(err, gc.IsNil)
	c.Assert(versions, gc.DeepEquals, []string{"1.11.2-raring-amd64"})
}

func (s *ValidateSuite) TestMajorMinorVersionMatch(c *gc.C) {
	s.makeLocalMetadata(c, "1.11.2", "region-2", "raring", "some-auth-url")
	metadataDir := osenv.JujuHomePath("")
	params := &ToolsMetadataLookupParams{
		Major: 1,
		Minor: 11,
		MetadataLookupParams: simplestreams.MetadataLookupParams{
			Region:        "region-2",
			Series:        "raring",
			Architectures: []string{"amd64"},
			Endpoint:      "some-auth-url",
			Sources:       []simplestreams.DataSource{simplestreams.NewURLDataSource("file://"+metadataDir, simplestreams.VerifySSLHostnames)},
		},
	}
	versions, err := ValidateToolsMetadata(params)
	c.Assert(err, gc.IsNil)
	c.Assert(versions, gc.DeepEquals, []string{"1.11.2-raring-amd64"})
}

func (s *ValidateSuite) TestNoMatch(c *gc.C) {
	s.makeLocalMetadata(c, "1.11.2", "region-2", "raring", "some-auth-url")
	metadataDir := osenv.JujuHomePath("")
	params := &ToolsMetadataLookupParams{
		Version: "1.11.2",
		MetadataLookupParams: simplestreams.MetadataLookupParams{
			Region:        "region-2",
			Series:        "precise",
			Architectures: []string{"amd64"},
			Endpoint:      "some-auth-url",
			Sources:       []simplestreams.DataSource{simplestreams.NewURLDataSource("file://"+metadataDir, simplestreams.VerifySSLHostnames)},
		},
	}
	_, err := ValidateToolsMetadata(params)
	c.Assert(err, gc.Not(gc.IsNil))
}
