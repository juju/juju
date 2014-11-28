// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/provider/common"
	coretesting "github.com/juju/juju/testing"
)

type archSuite struct {
	coretesting.FakeJujuHomeSuite
}

var _ = gc.Suite(&archSuite{})

func (s *archSuite) setupMetadata(c *gc.C, arches []string) (environs.Environ, simplestreams.CloudSpec) {
	s.PatchValue(&imagemetadata.DefaultBaseURL, "")
	env := &mockEnviron{
		config: configGetter(c),
	}

	var images []*imagemetadata.ImageMetadata
	for _, arch := range arches {
		images = append(images, &imagemetadata.ImageMetadata{
			Id:         "image-id",
			Arch:       arch,
			RegionName: "Region",
			Endpoint:   "https://endpoint/",
		})
	}
	// Append an image from another region with some other arch to ensure it is ignored.
	images = append(images, &imagemetadata.ImageMetadata{
		Id:         "image-id",
		Arch:       "arch",
		RegionName: "Region-Two",
		Endpoint:   "https://endpoint/",
	})
	cloudSpec := simplestreams.CloudSpec{
		Region:   "Region",
		Endpoint: "https://endpoint/",
	}

	metadataDir := c.MkDir()
	stor, err := filestorage.NewFileStorageWriter(metadataDir)
	c.Assert(err, jc.ErrorIsNil)
	err = imagemetadata.MergeAndWriteMetadata("precise", images, &cloudSpec, stor)
	c.Assert(err, jc.ErrorIsNil)

	id := "SupportedArchitectures"
	environs.RegisterImageDataSourceFunc(id, func(environs.Environ) (simplestreams.DataSource, error) {
		return simplestreams.NewURLDataSource(id, "file://"+metadataDir+"/images", false), nil
	})
	s.AddCleanup(func(*gc.C) {
		environs.UnregisterImageDataSourceFunc(id)
	})

	return env, cloudSpec
}

func (s *archSuite) TestSupportedArchitecturesNone(c *gc.C) {
	env, cloudSpec := s.setupMetadata(c, nil)
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: cloudSpec,
	})
	arches, err := common.SupportedArchitectures(env, imageConstraint)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(arches, gc.HasLen, 0)
}

func (s *archSuite) TestSupportedArchitecturesOne(c *gc.C) {
	env, cloudSpec := s.setupMetadata(c, []string{"ppc64el"})
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: cloudSpec,
	})
	arches, err := common.SupportedArchitectures(env, imageConstraint)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(arches, jc.SameContents, []string{"ppc64el"})
}

func (s *archSuite) TestSupportedArchitecturesMany(c *gc.C) {
	env, cloudSpec := s.setupMetadata(c, []string{"ppc64el", "amd64"})
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: cloudSpec,
	})
	arches, err := common.SupportedArchitectures(env, imageConstraint)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(arches, jc.SameContents, []string{"amd64", "ppc64el"})
}
