// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	"context"
	"path"
	"path/filepath"

	"github.com/juju/tc"
	"github.com/juju/utils/v4"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/internal/testing"
)

type ValidateSuite struct {
	testing.BaseSuite
	metadataDir string
}

var _ = tc.Suite(&ValidateSuite{})

func (s *ValidateSuite) makeLocalMetadata(c *tc.C, ss *simplestreams.Simplestreams, id, region string, base corebase.Base, endpoint, stream string) {
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
	c.Assert(err, tc.ErrorIsNil)
	err = imagemetadata.MergeAndWriteMetadata(context.Background(), ss, base, metadata, &cloudSpec, targetStorage)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ValidateSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.metadataDir = c.MkDir()
}

func (s *ValidateSuite) assertMatch(c *tc.C, ss *simplestreams.Simplestreams, stream string) {
	base := corebase.MustParseBaseFromString("ubuntu@13.04")
	s.makeLocalMetadata(c, ss, "1234", "region-2", base, "some-auth-url", stream)
	metadataPath := filepath.Join(s.metadataDir, "images")
	params := &simplestreams.MetadataLookupParams{
		Region:        "region-2",
		Release:       base.Channel.Track,
		Architectures: []string{"amd64"},
		Endpoint:      "some-auth-url",
		Stream:        stream,
		Sources: []simplestreams.DataSource{
			sstesting.VerifyDefaultCloudDataSource("test", utils.MakeFileURL(metadataPath))},
	}
	imageIds, resolveInfo, err := imagemetadata.ValidateImageMetadata(context.Background(), ss, params)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(imageIds, tc.DeepEquals, []string{"1234"})
	c.Check(resolveInfo, tc.DeepEquals, &simplestreams.ResolveInfo{
		Source:    "test",
		Signed:    false,
		IndexURL:  utils.MakeFileURL(path.Join(metadataPath, "streams/v1/index.json")),
		MirrorURL: "",
	})
}

func (s *ValidateSuite) TestMatch(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	s.assertMatch(c, ss, "")
	s.assertMatch(c, ss, imagemetadata.ReleasedStream)
	s.assertMatch(c, ss, "daily")
}

func (s *ValidateSuite) assertNoMatch(c *tc.C, ss *simplestreams.Simplestreams, stream string) {
	base := corebase.MustParseBaseFromString("ubuntu@13.04")
	s.makeLocalMetadata(c, ss, "1234", "region-2", base, "some-auth-url", stream)
	params := &simplestreams.MetadataLookupParams{
		Region:        "region-2",
		Release:       "12.04",
		Architectures: []string{"amd64"},
		Endpoint:      "some-auth-url",
		Stream:        stream,
		Sources: []simplestreams.DataSource{
			sstesting.VerifyDefaultCloudDataSource("test", "file://"+s.metadataDir)},
	}
	_, _, err := imagemetadata.ValidateImageMetadata(context.Background(), ss, params)
	c.Assert(err, tc.Not(tc.IsNil))
}

func (s *ValidateSuite) TestNoMatch(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	s.assertNoMatch(c, ss, "")
	s.assertNoMatch(c, ss, imagemetadata.ReleasedStream)
	s.assertNoMatch(c, ss, "daily")
}
