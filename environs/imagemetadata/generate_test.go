// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	"context"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/imagemetadata/testing"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/environs/storage"
	coretesting "github.com/juju/juju/internal/testing"
)

var (
	testBase    = corebase.MustParseBaseFromString("ubuntu@22.04")
	testVersion = "22.04"
)

var _ = tc.Suite(&generateSuite{})

type generateSuite struct {
	coretesting.BaseSuite
}

func assertFetch(c *tc.C, ss *simplestreams.Simplestreams, stor storage.Storage, version, arch, region, endpoint string, ids ...string) {
	cons, err := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: simplestreams.CloudSpec{region, endpoint},
		Releases:  []string{version},
		Arches:    []string{arch},
	})
	c.Assert(err, jc.ErrorIsNil)
	dataSource := storage.NewStorageSimpleStreamsDataSource("test datasource", stor, "images", simplestreams.DEFAULT_CLOUD_DATA, false)
	metadata, _, err := imagemetadata.Fetch(context.Background(), ss, []simplestreams.DataSource{dataSource}, cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, tc.HasLen, len(ids))
	for i, id := range ids {
		c.Assert(metadata[i].Id, tc.Equals, id)
	}
}

func (s *generateSuite) TestWriteMetadata(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	im := []*imagemetadata.ImageMetadata{
		{
			Id:      "1234",
			Arch:    "amd64",
			Version: testVersion,
		},
	}
	cloudSpec := &simplestreams.CloudSpec{
		Region:   "region",
		Endpoint: "endpoint",
	}
	dir := c.MkDir()
	targetStorage, err := filestorage.NewFileStorageWriter(dir)
	c.Assert(err, jc.ErrorIsNil)
	err = imagemetadata.MergeAndWriteMetadata(context.Background(), ss, testBase, im, cloudSpec, targetStorage)
	c.Assert(err, jc.ErrorIsNil)
	metadata := testing.ParseMetadataFromDir(c, dir)
	c.Assert(metadata, tc.HasLen, 1)
	im[0].RegionName = cloudSpec.Region
	im[0].Endpoint = cloudSpec.Endpoint
	c.Assert(im[0], tc.DeepEquals, metadata[0])
	assertFetch(c, ss, targetStorage, testVersion, "amd64", "region", "endpoint", "1234")
}

func (s *generateSuite) TestWriteMetadataMergeOverwriteSameArch(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	existingImageMetadata := []*imagemetadata.ImageMetadata{
		{
			Id:      "1234",
			Arch:    "amd64",
			Version: testVersion,
		},
	}
	cloudSpec := &simplestreams.CloudSpec{
		Region:   "region",
		Endpoint: "endpoint",
	}
	dir := c.MkDir()
	targetStorage, err := filestorage.NewFileStorageWriter(dir)
	c.Assert(err, jc.ErrorIsNil)
	err = imagemetadata.MergeAndWriteMetadata(context.Background(), ss, testBase, existingImageMetadata, cloudSpec, targetStorage)
	c.Assert(err, jc.ErrorIsNil)
	newImageMetadata := []*imagemetadata.ImageMetadata{
		{
			Id:      "abcd",
			Arch:    "amd64",
			Version: testVersion,
		},
		{
			Id:      "xyz",
			Arch:    "arm",
			Version: testVersion,
		},
	}
	err = imagemetadata.MergeAndWriteMetadata(context.Background(), ss, testBase, newImageMetadata, cloudSpec, targetStorage)
	c.Assert(err, jc.ErrorIsNil)
	metadata := testing.ParseMetadataFromDir(c, dir)
	c.Assert(metadata, tc.HasLen, 2)
	for _, im := range newImageMetadata {
		im.RegionName = cloudSpec.Region
		im.Endpoint = cloudSpec.Endpoint
	}
	c.Assert(metadata, tc.DeepEquals, newImageMetadata)
	assertFetch(c, ss, targetStorage, testVersion, "amd64", "region", "endpoint", "abcd")
	assertFetch(c, ss, targetStorage, testVersion, "arm", "region", "endpoint", "xyz")
}

func (s *generateSuite) TestWriteMetadataMergeDifferentSeries(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	existingImageMetadata := []*imagemetadata.ImageMetadata{
		{
			Id:      "1234",
			Arch:    "amd64",
			Version: testVersion,
		},
	}
	cloudSpec := &simplestreams.CloudSpec{
		Region:   "region",
		Endpoint: "endpoint",
	}
	dir := c.MkDir()
	targetStorage, err := filestorage.NewFileStorageWriter(dir)
	c.Assert(err, jc.ErrorIsNil)
	err = imagemetadata.MergeAndWriteMetadata(context.Background(), ss, testBase, existingImageMetadata, cloudSpec, targetStorage)
	c.Assert(err, jc.ErrorIsNil)
	newImageMetadata := []*imagemetadata.ImageMetadata{
		{
			Id:      "abcd",
			Arch:    "amd64",
			Version: "12.04",
		},
		{
			Id:      "xyz",
			Arch:    "arm",
			Version: "12.04",
		},
	}
	base := corebase.MustParseBaseFromString("ubuntu@12.04")
	err = imagemetadata.MergeAndWriteMetadata(context.Background(), ss, base, newImageMetadata, cloudSpec, targetStorage)
	c.Assert(err, jc.ErrorIsNil)
	metadata := testing.ParseMetadataFromDir(c, dir)
	c.Assert(metadata, tc.HasLen, 3)
	newImageMetadata = append(newImageMetadata, existingImageMetadata[0])
	for _, im := range newImageMetadata {
		im.RegionName = cloudSpec.Region
		im.Endpoint = cloudSpec.Endpoint
	}
	imagemetadata.Sort(newImageMetadata)
	c.Assert(metadata, tc.DeepEquals, newImageMetadata)
	assertFetch(c, ss, targetStorage, testVersion, "amd64", "region", "endpoint", "1234")
	assertFetch(c, ss, targetStorage, "12.04", "amd64", "region", "endpoint", "abcd")
}

func (s *generateSuite) TestWriteMetadataMergeDifferentRegion(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	existingImageMetadata := []*imagemetadata.ImageMetadata{
		{
			Id:      "1234",
			Arch:    "amd64",
			Version: testVersion,
		},
	}
	cloudSpec := &simplestreams.CloudSpec{
		Region:   "region",
		Endpoint: "endpoint",
	}
	dir := c.MkDir()
	targetStorage, err := filestorage.NewFileStorageWriter(dir)
	c.Assert(err, jc.ErrorIsNil)
	err = imagemetadata.MergeAndWriteMetadata(context.Background(), ss, testBase, existingImageMetadata, cloudSpec, targetStorage)
	c.Assert(err, jc.ErrorIsNil)
	newImageMetadata := []*imagemetadata.ImageMetadata{
		{
			Id:      "abcd",
			Arch:    "amd64",
			Version: testVersion,
		},
		{
			Id:      "xyz",
			Arch:    "arm",
			Version: testVersion,
		},
	}
	cloudSpec = &simplestreams.CloudSpec{
		Region:   "region2",
		Endpoint: "endpoint2",
	}
	err = imagemetadata.MergeAndWriteMetadata(context.Background(), ss, testBase, newImageMetadata, cloudSpec, targetStorage)
	c.Assert(err, jc.ErrorIsNil)
	metadata := testing.ParseMetadataFromDir(c, dir)
	c.Assert(metadata, tc.HasLen, 3)
	for _, im := range newImageMetadata {
		im.RegionName = "region2"
		im.Endpoint = "endpoint2"
	}
	existingImageMetadata[0].RegionName = "region"
	existingImageMetadata[0].Endpoint = "endpoint"
	newImageMetadata = append(newImageMetadata, existingImageMetadata[0])
	imagemetadata.Sort(newImageMetadata)
	c.Assert(metadata, tc.DeepEquals, newImageMetadata)
	assertFetch(c, ss, targetStorage, testVersion, "amd64", "region", "endpoint", "1234")
	assertFetch(c, ss, targetStorage, testVersion, "amd64", "region2", "endpoint2", "abcd")
}

// lp#1600054
func (s *generateSuite) TestWriteMetadataMergeDifferentVirtType(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	existingImageMetadata := []*imagemetadata.ImageMetadata{
		{
			Id:       "1234",
			Arch:     "amd64",
			Version:  testVersion,
			VirtType: "kvm",
		},
	}
	cloudSpec := &simplestreams.CloudSpec{
		Region:   "region",
		Endpoint: "endpoint",
	}
	dir := c.MkDir()
	targetStorage, err := filestorage.NewFileStorageWriter(dir)
	c.Assert(err, jc.ErrorIsNil)
	err = imagemetadata.MergeAndWriteMetadata(context.Background(), ss, testBase, existingImageMetadata, cloudSpec, targetStorage)
	c.Assert(err, jc.ErrorIsNil)
	newImageMetadata := []*imagemetadata.ImageMetadata{
		{
			Id:       "abcd",
			Arch:     "amd64",
			Version:  testVersion,
			VirtType: "lxd",
		},
	}
	err = imagemetadata.MergeAndWriteMetadata(context.Background(), ss, testBase, newImageMetadata, cloudSpec, targetStorage)
	c.Assert(err, jc.ErrorIsNil)

	foundMetadata := testing.ParseMetadataFromDir(c, dir)

	expectedMetadata := append(newImageMetadata, existingImageMetadata...)
	c.Assert(len(foundMetadata), tc.Equals, len(expectedMetadata))
	for _, im := range expectedMetadata {
		im.RegionName = cloudSpec.Region
		im.Endpoint = cloudSpec.Endpoint
	}
	imagemetadata.Sort(expectedMetadata)
	c.Assert(foundMetadata, tc.DeepEquals, expectedMetadata)
	assertFetch(c, ss, targetStorage, testVersion, "amd64", "region", "endpoint", "1234", "abcd")
}

func (s *generateSuite) TestWriteIndexRegionOnce(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	existingImageMetadata := []*imagemetadata.ImageMetadata{
		{
			Id:       "1234",
			Arch:     "amd64",
			Version:  testVersion,
			VirtType: "kvm",
		},
	}
	cloudSpec := &simplestreams.CloudSpec{
		Region:   "region",
		Endpoint: "endpoint",
	}
	dir := c.MkDir()
	targetStorage, err := filestorage.NewFileStorageWriter(dir)
	c.Assert(err, jc.ErrorIsNil)
	err = imagemetadata.MergeAndWriteMetadata(context.Background(), ss, testBase, existingImageMetadata, cloudSpec, targetStorage)
	c.Assert(err, jc.ErrorIsNil)
	newImageMetadata := []*imagemetadata.ImageMetadata{
		{
			Id:       "abcd",
			Arch:     "amd64",
			Version:  testVersion,
			VirtType: "lxd",
		},
	}
	err = imagemetadata.MergeAndWriteMetadata(context.Background(), ss, testBase, newImageMetadata, cloudSpec, targetStorage)
	c.Assert(err, jc.ErrorIsNil)

	foundIndex, _ := testing.ParseIndexMetadataFromStorage(c, targetStorage)
	expectedCloudSpecs := []simplestreams.CloudSpec{*cloudSpec}
	c.Assert(foundIndex.Clouds, jc.SameContents, expectedCloudSpecs)
}

func (s *generateSuite) TestWriteDistinctIndexRegions(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	existingImageMetadata := []*imagemetadata.ImageMetadata{
		{
			Id:       "1234",
			Arch:     "amd64",
			Version:  testVersion,
			VirtType: "kvm",
		},
	}
	cloudSpec := &simplestreams.CloudSpec{
		Region:   "region",
		Endpoint: "endpoint",
	}
	dir := c.MkDir()
	targetStorage, err := filestorage.NewFileStorageWriter(dir)
	c.Assert(err, jc.ErrorIsNil)
	err = imagemetadata.MergeAndWriteMetadata(context.Background(), ss, testBase, existingImageMetadata, cloudSpec, targetStorage)
	c.Assert(err, jc.ErrorIsNil)

	expectedCloudSpecs := []simplestreams.CloudSpec{*cloudSpec}

	newImageMetadata := []*imagemetadata.ImageMetadata{
		{
			Id:       "abcd",
			Arch:     "amd64",
			Version:  testVersion,
			VirtType: "lxd",
		},
	}
	cloudSpec = &simplestreams.CloudSpec{
		Region:   "region2",
		Endpoint: "endpoint2",
	}
	err = imagemetadata.MergeAndWriteMetadata(context.Background(), ss, testBase, newImageMetadata, cloudSpec, targetStorage)
	c.Assert(err, jc.ErrorIsNil)

	foundIndex, _ := testing.ParseIndexMetadataFromStorage(c, targetStorage)
	expectedCloudSpecs = append(expectedCloudSpecs, *cloudSpec)
	c.Assert(foundIndex.Clouds, jc.SameContents, expectedCloudSpecs)
}
