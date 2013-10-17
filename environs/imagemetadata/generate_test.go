// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/imagemetadata/testing"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/testing/testbase"
)

var _ = gc.Suite(&generateSuite{})

type generateSuite struct {
	testbase.LoggingSuite
}

func (s *generateSuite) TestWriteMetadata(c *gc.C) {
	im := &imagemetadata.ImageMetadata{
		Id:   "1234",
		Arch: "amd64",
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
	c.Assert(metadata, gc.HasLen, 1)
	im.RegionName = cloudSpec.Region
	c.Assert(im, gc.DeepEquals, metadata[0])
}
