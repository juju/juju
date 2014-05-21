// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	"path"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
)

type ValidateSuite struct {
	testing.BaseSuite
	metadataDir string
}

var _ = gc.Suite(&ValidateSuite{})

func (s *ValidateSuite) makeLocalMetadata(c *gc.C, id, region, series, endpoint, stream string) error {
	metadata := []*imagemetadata.ImageMetadata{
		{
			Id:     id,
			Arch:   "amd64",
			Stream: stream,
		},
	}
	cloudSpec := simplestreams.CloudSpec{
		Region:   region,
		Endpoint: endpoint,
	}
	targetStorage, err := filestorage.NewFileStorageWriter(s.metadataDir)
	c.Assert(err, gc.IsNil)
	err = imagemetadata.MergeAndWriteMetadata(series, metadata, &cloudSpec, targetStorage)
	if err != nil {
		return err
	}
	return nil
}

func (s *ValidateSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.metadataDir = c.MkDir()
}

func (s *ValidateSuite) assertMatch(c *gc.C, stream string) {
	s.makeLocalMetadata(c, "1234", "region-2", "raring", "some-auth-url", stream)
	metadataPath := filepath.Join(s.metadataDir, "images")
	params := &simplestreams.MetadataLookupParams{
		Region:        "region-2",
		Series:        "raring",
		Architectures: []string{"amd64"},
		Endpoint:      "some-auth-url",
		Stream:        stream,
		Sources: []simplestreams.DataSource{
			simplestreams.NewURLDataSource("test", "file://"+metadataPath, utils.VerifySSLHostnames)},
	}
	imageIds, resolveInfo, err := imagemetadata.ValidateImageMetadata(params)
	c.Assert(err, gc.IsNil)
	c.Assert(imageIds, gc.DeepEquals, []string{"1234"})
	c.Check(resolveInfo, gc.DeepEquals, &simplestreams.ResolveInfo{
		Source:    "test",
		Signed:    false,
		IndexURL:  "file://" + path.Join(metadataPath, "streams/v1/index.json"),
		MirrorURL: "",
	})
}

func (s *ValidateSuite) TestMatch(c *gc.C) {
	s.assertMatch(c, "")
	s.assertMatch(c, imagemetadata.ReleasedStream)
	s.assertMatch(c, "daily")
}

func (s *ValidateSuite) assertNoMatch(c *gc.C, stream string) {
	s.makeLocalMetadata(c, "1234", "region-2", "raring", "some-auth-url", stream)
	params := &simplestreams.MetadataLookupParams{
		Region:        "region-2",
		Series:        "precise",
		Architectures: []string{"amd64"},
		Endpoint:      "some-auth-url",
		Stream:        stream,
		Sources: []simplestreams.DataSource{
			simplestreams.NewURLDataSource("test", "file://"+s.metadataDir, utils.VerifySSLHostnames)},
	}
	_, _, err := imagemetadata.ValidateImageMetadata(params)
	c.Assert(err, gc.Not(gc.IsNil))
}

func (s *ValidateSuite) TestNoMatch(c *gc.C) {
	s.assertNoMatch(c, "")
	s.assertNoMatch(c, imagemetadata.ReleasedStream)
	s.assertNoMatch(c, "daily")
}
