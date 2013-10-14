// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/imagemetadata/testing"
	"launchpad.net/juju-core/environs/simplestreams"
	"sort"
)

var _ = gc.Suite(&generateSuite{})

type generateSuite struct{}

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
	err = imagemetadata.WriteMetadata("raring", im, cloudSpec, targetStorage)
	c.Assert(err, gc.IsNil)
	metadata := testing.ParseMetadata(c, dir)
	c.Assert(len(metadata), gc.Equals, 1)
	im[0].RegionName = cloudSpec.Region
	c.Assert(im[0], gc.DeepEquals, metadata[0])
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
	err = imagemetadata.WriteMetadata("raring", existingImageMetadata, cloudSpec, targetStorage)
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
	err = imagemetadata.WriteMetadata("raring", newImageMetadata, cloudSpec, targetStorage)
	c.Assert(err, gc.IsNil)
	metadata := testing.ParseMetadata(c, dir)
	c.Assert(len(metadata), gc.Equals, 2)
	for _, im := range newImageMetadata {
		im.RegionName = cloudSpec.Region
	}
	c.Assert(metadata, gc.DeepEquals, newImageMetadata)
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
	err = imagemetadata.WriteMetadata("raring", existingImageMetadata, cloudSpec, targetStorage)
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
	err = imagemetadata.WriteMetadata("precise", newImageMetadata, cloudSpec, targetStorage)
	c.Assert(err, gc.IsNil)
	metadata := testing.ParseMetadata(c, dir)
	c.Assert(len(metadata), gc.Equals, 3)
	newImageMetadata = append(newImageMetadata, existingImageMetadata[0])
	for _, im := range newImageMetadata {
		im.RegionName = cloudSpec.Region
	}
	sort.Sort(byId(metadata))
	sort.Sort(byId(newImageMetadata))
	c.Assert(metadata, gc.DeepEquals, newImageMetadata)
}

type byId []*imagemetadata.ImageMetadata

func (b byId) Len() int           { return len(b) }
func (b byId) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byId) Less(i, j int) bool { return b[i].Id < b[j].Id }
