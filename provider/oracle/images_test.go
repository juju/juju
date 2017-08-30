// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle_test

import (
	"errors"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/oracle"
	oracletesting "github.com/juju/juju/provider/oracle/testing"
)

type imageSuite struct{}

var _ = gc.Suite(&imageSuite{})

func (i imageSuite) TestGetImageName(c *gc.C) {
	name, err := oracle.GetImageName(oracletesting.DefaultEnvironAPI, "0")
	c.Assert(err, gc.IsNil)
	ok := len(name) > 0
	c.Assert(ok, gc.Equals, true)
}

func (i imageSuite) TestGetImageNameWithErrors(c *gc.C) {
	_, err := oracle.GetImageName(oracletesting.DefaultEnvironAPI, "")
	c.Assert(err, gc.NotNil)

	_, err = oracle.GetImageName(&oracletesting.FakeEnvironAPI{
		FakeImager: oracletesting.FakeImager{
			AllErr: errors.New("FakeImageListErr"),
		}}, "0")
	c.Assert(err, gc.NotNil)
}

func (i imageSuite) TestCheckImageList(c *gc.C) {
	images, err := oracle.CheckImageList(oracletesting.DefaultEnvironAPI)

	c.Assert(err, gc.IsNil)
	c.Assert(images, gc.NotNil)
}

func (i imageSuite) TestCheckImageListWithErrors(c *gc.C) {
	_, err := oracle.CheckImageList(&oracletesting.FakeEnvironAPI{
		FakeImager: oracletesting.FakeImager{
			AllErr: errors.New("FakeImageListErr"),
		},
	})
	c.Assert(err, gc.NotNil)

	_, err = oracle.CheckImageList(nil)
	c.Assert(err, gc.NotNil)
}

func (i imageSuite) TestFindInstanceSpec(c *gc.C) {
	spec, imagelist, err := oracle.FindInstanceSpec(
		oracletesting.DefaultEnvironAPI,
		TestImageMetadata,
		[]instances.InstanceType{
			instances.InstanceType{
				Id:     "win2018r2",
				Name:   "",
				Arches: []string{"amd64"},
			},
		},

		&instances.InstanceConstraint{
			Region: "uscom-central-1",
			Series: "xenial",
		},
	)

	c.Assert(err, gc.IsNil)
	c.Assert(spec, gc.NotNil)
	c.Assert((len(imagelist) > 0), gc.Equals, true)
}

func (i imageSuite) TestFindInstanceSpecWithSeriesError(c *gc.C) {
	_, _, err := oracle.FindInstanceSpec(
		oracletesting.DefaultEnvironAPI,
		TestImageMetadata,
		[]instances.InstanceType{
			instances.InstanceType{
				Id:     "win2018r2",
				Name:   "",
				Arches: []string{"amd64"},
			},
		},

		&instances.InstanceConstraint{
			Region: "uscom-central-1",
			Series: "non-supported-series",
		},
	)

	c.Assert(err, gc.NotNil)
}

func (i imageSuite) TestFindInstanceSpecWithError(c *gc.C) {
	_, _, err := oracle.FindInstanceSpec(
		oracletesting.DefaultEnvironAPI,
		[]*imagemetadata.ImageMetadata{},
		[]instances.InstanceType{
			instances.InstanceType{
				Id:     "win2018r2",
				Name:   "",
				Arches: []string{"amd64"},
			},
		},

		&instances.InstanceConstraint{
			Region: "uscom-central-1",
			Series: "xenial",
		},
	)
	c.Assert(err, gc.NotNil)
}

func (i imageSuite) TestInstanceTypes(c *gc.C) {
	types, err := oracle.InstanceTypes(oracletesting.DefaultEnvironAPI)
	c.Assert(err, gc.IsNil)
	c.Assert(types, gc.NotNil)
}

func (i imageSuite) TestInstanceTypesWithErrrors(c *gc.C) {
	for _, fake := range []*oracletesting.FakeEnvironAPI{
		&oracletesting.FakeEnvironAPI{
			FakeShaper: oracletesting.FakeShaper{
				AllErr: errors.New("FakeShaperErr"),
			},
		},
	} {
		_, err := oracle.InstanceTypes(fake)
		c.Assert(err, gc.NotNil)
	}
}

var TestImageMetadata = []*imagemetadata.ImageMetadata{
	&imagemetadata.ImageMetadata{
		Id:          "win2012r2",
		Storage:     "",
		VirtType:    "",
		Arch:        "amd64",
		Version:     "win2012r2",
		RegionAlias: "",
		RegionName:  "uscom-central-1",
		Endpoint:    "https://compute.uscom-central-1.oraclecloud.com",
		Stream:      "",
	},
	&imagemetadata.ImageMetadata{
		Id:          "20170307",
		Storage:     "",
		VirtType:    "",
		Arch:        "amd64",
		Version:     "16.04",
		RegionAlias: "",
		RegionName:  "uscom-central-1",
		Endpoint:    "https://compute.uscom-central-1.oraclecloud.com",
		Stream:      "",
	},
	&imagemetadata.ImageMetadata{
		Id:          "20170307",
		Storage:     "",
		VirtType:    "",
		Arch:        "amd64",
		Version:     "14.04",
		RegionAlias: "",
		RegionName:  "uscom-central-1",
		Endpoint:    "https://compute.uscom-central-1.oraclecloud.com",
		Stream:      "",
	},
	&imagemetadata.ImageMetadata{
		Id:          "20170307",
		Storage:     "",
		VirtType:    "",
		Arch:        "amd64",
		Version:     "12.04",
		RegionAlias: "",
		RegionName:  "uscom-central-1",
		Endpoint:    "https://compute.uscom-central-1.oraclecloud.com",
		Stream:      "",
	},
	&imagemetadata.ImageMetadata{
		Id:          "win2018r2",
		Storage:     "",
		VirtType:    "",
		Arch:        "amd64",
		Version:     "win2018r2",
		RegionAlias: "",
		RegionName:  "uscom-central-1",
		Endpoint:    "https://compute.uscom-central-1.oraclecloud.com",
		Stream:      "",
	},
}
