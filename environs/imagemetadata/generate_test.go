// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	"sort"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/imagemetadata/testing"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/testing/testbase"
)

var _ = gc.Suite(&generateSuite{})

type generateSuite struct {
	testbase.LoggingSuite
}

func assertFetch(c *gc.C, stor storage.Storage, series, arch, region, endpoint, id string) {
	cons := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: simplestreams.CloudSpec{region, endpoint},
		Series:    []string{series},
		Arches:    []string{arch},
	})
	dataSource := storage.NewStorageSimpleStreamsDataSource(stor, "images")
	metadata, err := imagemetadata.Fetch(
		[]simplestreams.DataSource{dataSource}, simplestreams.DefaultIndexPath, cons, false)
	c.Assert(err, gc.IsNil)
	c.Assert(metadata, gc.HasLen, 1)
	c.Assert(metadata[0].Id, gc.Equals, id)
}

func (s *generateSuite) TestWriteMetadata(c *gc.C) {
	im := []*imagemetadata.ImageMetadata{
		{
			Id:      "1234",
			Arch:    "amd64",
			Version: "13.04",
		},
	}
	cloudSpec := &simplestreams.CloudSpec{
		Region:   "region",
		Endpoint: "endpoint",
	}
	dir := c.MkDir()
	targetStorage, err := filestorage.NewFileStorageWriter(dir, filestorage.UseDefaultTmpDir)
	c.Assert(err, gc.IsNil)
	err = imagemetadata.MergeAndWriteMetadata("raring", im, cloudSpec, targetStorage)
	c.Assert(err, gc.IsNil)
	metadata := testing.ParseMetadataFromDir(c, dir)
	c.Assert(metadata, gc.HasLen, 1)
	im[0].RegionName = cloudSpec.Region
	im[0].Endpoint = cloudSpec.Endpoint
	c.Assert(im[0], gc.DeepEquals, metadata[0])
	assertFetch(c, targetStorage, "raring", "amd64", "region", "endpoint", "1234")
}

func (s *generateSuite) TestWriteMetadataMergeOverwriteSameArch(c *gc.C) {
	existingImageMetadata := []*imagemetadata.ImageMetadata{
		{
			Id:      "1234",
			Arch:    "amd64",
			Version: "13.04",
		},
	}
	cloudSpec := &simplestreams.CloudSpec{
		Region:   "region",
		Endpoint: "endpoint",
	}
	dir := c.MkDir()
	targetStorage, err := filestorage.NewFileStorageWriter(dir, filestorage.UseDefaultTmpDir)
	c.Assert(err, gc.IsNil)
	err = imagemetadata.MergeAndWriteMetadata("raring", existingImageMetadata, cloudSpec, targetStorage)
	c.Assert(err, gc.IsNil)
	newImageMetadata := []*imagemetadata.ImageMetadata{
		{
			Id:      "abcd",
			Arch:    "amd64",
			Version: "13.04",
		},
		{
			Id:      "xyz",
			Arch:    "arm",
			Version: "13.04",
		},
	}
	err = imagemetadata.MergeAndWriteMetadata("raring", newImageMetadata, cloudSpec, targetStorage)
	c.Assert(err, gc.IsNil)
	metadata := testing.ParseMetadataFromDir(c, dir)
	c.Assert(metadata, gc.HasLen, 2)
	for _, im := range newImageMetadata {
		im.RegionName = cloudSpec.Region
		im.Endpoint = cloudSpec.Endpoint
	}
	c.Assert(metadata, gc.DeepEquals, newImageMetadata)
	assertFetch(c, targetStorage, "raring", "amd64", "region", "endpoint", "abcd")
	assertFetch(c, targetStorage, "raring", "arm", "region", "endpoint", "xyz")
}

func (s *generateSuite) TestWriteMetadataMergeDifferentSeries(c *gc.C) {
	existingImageMetadata := []*imagemetadata.ImageMetadata{
		{
			Id:      "1234",
			Arch:    "amd64",
			Version: "13.04",
		},
	}
	cloudSpec := &simplestreams.CloudSpec{
		Region:   "region",
		Endpoint: "endpoint",
	}
	dir := c.MkDir()
	targetStorage, err := filestorage.NewFileStorageWriter(dir, filestorage.UseDefaultTmpDir)
	c.Assert(err, gc.IsNil)
	err = imagemetadata.MergeAndWriteMetadata("raring", existingImageMetadata, cloudSpec, targetStorage)
	c.Assert(err, gc.IsNil)
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
	err = imagemetadata.MergeAndWriteMetadata("precise", newImageMetadata, cloudSpec, targetStorage)
	c.Assert(err, gc.IsNil)
	metadata := testing.ParseMetadataFromDir(c, dir)
	c.Assert(metadata, gc.HasLen, 3)
	newImageMetadata = append(newImageMetadata, existingImageMetadata[0])
	for _, im := range newImageMetadata {
		im.RegionName = cloudSpec.Region
		im.Endpoint = cloudSpec.Endpoint
	}
	sort.Sort(byId(metadata))
	sort.Sort(byId(newImageMetadata))
	c.Assert(metadata, gc.DeepEquals, newImageMetadata)
	assertFetch(c, targetStorage, "raring", "amd64", "region", "endpoint", "1234")
	assertFetch(c, targetStorage, "precise", "amd64", "region", "endpoint", "abcd")
}

func (s *generateSuite) TestWriteMetadataMergeDifferentRegion(c *gc.C) {
	existingImageMetadata := []*imagemetadata.ImageMetadata{
		{
			Id:      "1234",
			Arch:    "amd64",
			Version: "13.04",
		},
	}
	cloudSpec := &simplestreams.CloudSpec{
		Region:   "region",
		Endpoint: "endpoint",
	}
	dir := c.MkDir()
	targetStorage, err := filestorage.NewFileStorageWriter(dir, filestorage.UseDefaultTmpDir)
	c.Assert(err, gc.IsNil)
	err = imagemetadata.MergeAndWriteMetadata("raring", existingImageMetadata, cloudSpec, targetStorage)
	c.Assert(err, gc.IsNil)
	newImageMetadata := []*imagemetadata.ImageMetadata{
		{
			Id:      "abcd",
			Arch:    "amd64",
			Version: "13.04",
		},
		{
			Id:      "xyz",
			Arch:    "arm",
			Version: "13.04",
		},
	}
	cloudSpec = &simplestreams.CloudSpec{
		Region:   "region2",
		Endpoint: "endpoint2",
	}
	err = imagemetadata.MergeAndWriteMetadata("raring", newImageMetadata, cloudSpec, targetStorage)
	c.Assert(err, gc.IsNil)
	metadata := testing.ParseMetadataFromDir(c, dir)
	c.Assert(metadata, gc.HasLen, 3)
	for _, im := range newImageMetadata {
		im.RegionName = "region2"
		im.Endpoint = "endpoint2"
	}
	existingImageMetadata[0].RegionName = "region"
	existingImageMetadata[0].Endpoint = "endpoint"
	newImageMetadata = append(newImageMetadata, existingImageMetadata[0])
	sort.Sort(byId(metadata))
	sort.Sort(byId(newImageMetadata))
	c.Assert(metadata, gc.DeepEquals, newImageMetadata)
	assertFetch(c, targetStorage, "raring", "amd64", "region", "endpoint", "1234")
	assertFetch(c, targetStorage, "raring", "amd64", "region2", "endpoint2", "abcd")
}

type byId []*imagemetadata.ImageMetadata

func (b byId) Len() int           { return len(b) }
func (b byId) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byId) Less(i, j int) bool { return b[i].Id < b[j].Id }
