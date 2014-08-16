// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/environs"
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
	stor := newStorage(s, c)
	env := &mockEnviron{
		storage: stor,
		config:  configGetter(c),
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
	err := imagemetadata.MergeAndWriteMetadata("precise", images, &cloudSpec, env.Storage())
	c.Assert(err, gc.IsNil)
	return env, cloudSpec
}

func (s *archSuite) TestSupportedArchitecturesNone(c *gc.C) {
	env, cloudSpec := s.setupMetadata(c, nil)
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: cloudSpec,
	})
	arches, err := common.SupportedArchitectures(env, imageConstraint)
	c.Assert(err, gc.IsNil)
	c.Assert(arches, gc.HasLen, 0)
}

func (s *archSuite) TestSupportedArchitecturesOne(c *gc.C) {
	env, cloudSpec := s.setupMetadata(c, []string{"ppc64el"})
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: cloudSpec,
	})
	arches, err := common.SupportedArchitectures(env, imageConstraint)
	c.Assert(err, gc.IsNil)
	c.Assert(arches, jc.SameContents, []string{"ppc64el"})
}

func (s *archSuite) TestSupportedArchitecturesMany(c *gc.C) {
	env, cloudSpec := s.setupMetadata(c, []string{"ppc64el", "amd64"})
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: cloudSpec,
	})
	arches, err := common.SupportedArchitectures(env, imageConstraint)
	c.Assert(err, gc.IsNil)
	c.Assert(arches, jc.SameContents, []string{"amd64", "ppc64el"})
}
