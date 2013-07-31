// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/instances"
)

type ImageSuite struct{}

var _ = gc.Suite(&ImageSuite{})

func (*ImageSuite) TestImageMetadataToImagesAcceptsNil(c *gc.C) {
	c.Check(environs.ImageMetadataToImages(nil), gc.HasLen, 0)
}

func (*ImageSuite) TestImageMetadataToImagesConvertsSelectMetadata(c *gc.C) {
	input := []*imagemetadata.ImageMetadata{
		{
			Id:          "id",
			Storage:     "storage-is-ignored",
			VType:       "vtype",
			Arch:        "arch",
			RegionAlias: "region-alias-is-ignored",
			RegionName:  "region-name-is-ignored",
			Endpoint:    "endpoint-is-ignored",
		},
	}
	expectation := []instances.Image{
		{
			Id:    "id",
			VType: "vtype",
			Arch:  "arch",
		},
	}
	c.Check(environs.ImageMetadataToImages(input), gc.DeepEquals, expectation)
}

func (*ImageSuite) TestImageMetadataToImagesMaintainsOrdering(c *gc.C) {
	input := []*imagemetadata.ImageMetadata{
		{Id: "one", Arch: "Z80"},
		{Id: "two", Arch: "i386"},
		{Id: "three", Arch: "amd64"},
	}
	expectation := []instances.Image{
		{Id: "one", Arch: "Z80"},
		{Id: "two", Arch: "i386"},
		{Id: "three", Arch: "amd64"},
	}
	c.Check(environs.ImageMetadataToImages(input), gc.DeepEquals, expectation)
}
