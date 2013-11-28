// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/testing/testbase"
)

type ValidateSuite struct {
	testbase.LoggingSuite
	metadataDir string
}

var _ = gc.Suite(&ValidateSuite{})

func (s *ValidateSuite) makeLocalMetadata(c *gc.C, id, region, series, endpoint string) error {
	metadata := []*imagemetadata.ImageMetadata{
		{
			Id:   id,
			Arch: "amd64",
		},
	}
	cloudSpec := simplestreams.CloudSpec{
		Region:   region,
		Endpoint: endpoint,
	}
	targetStorage, err := filestorage.NewFileStorageWriter(s.metadataDir, filestorage.UseDefaultTmpDir)
	c.Assert(err, gc.IsNil)
	err = imagemetadata.MergeAndWriteMetadata(series, metadata, &cloudSpec, targetStorage)
	if err != nil {
		return err
	}
	return nil
}

func (s *ValidateSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.metadataDir = c.MkDir()
}

func (s *ValidateSuite) TestMatch(c *gc.C) {
	s.makeLocalMetadata(c, "1234", "region-2", "raring", "some-auth-url")
	metadataPath := filepath.Join(s.metadataDir, "images")
	params := &simplestreams.MetadataLookupParams{
		Region:        "region-2",
		Series:        "raring",
		Architectures: []string{"amd64"},
		Endpoint:      "some-auth-url",
		Sources: []simplestreams.DataSource{
			simplestreams.NewURLDataSource("file://"+metadataPath, simplestreams.VerifySSLHostnames)},
	}
	imageIds, err := imagemetadata.ValidateImageMetadata(params)
	c.Assert(err, gc.IsNil)
	c.Assert(imageIds, gc.DeepEquals, []string{"1234"})
}

func (s *ValidateSuite) TestNoMatch(c *gc.C) {
	s.makeLocalMetadata(c, "1234", "region-2", "raring", "some-auth-url")
	params := &simplestreams.MetadataLookupParams{
		Region:        "region-2",
		Series:        "precise",
		Architectures: []string{"amd64"},
		Endpoint:      "some-auth-url",
		Sources: []simplestreams.DataSource{
			simplestreams.NewURLDataSource("file://"+s.metadataDir, simplestreams.VerifySSLHostnames)},
	}
	_, err := imagemetadata.ValidateImageMetadata(params)
	c.Assert(err, gc.Not(gc.IsNil))
}
