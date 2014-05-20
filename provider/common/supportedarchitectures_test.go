// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	gc "launchpad.net/gocheck"

	jc "github.com/juju/testing/checkers"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/provider/common"
	coretesting "launchpad.net/juju-core/testing"
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
	env, cloudSpec := s.setupMetadata(c, []string{"ppc64"})
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: cloudSpec,
	})
	arches, err := common.SupportedArchitectures(env, imageConstraint)
	c.Assert(err, gc.IsNil)
	c.Assert(arches, jc.SameContents, []string{"ppc64"})
}

func (s *archSuite) TestSupportedArchitecturesMany(c *gc.C) {
	env, cloudSpec := s.setupMetadata(c, []string{"ppc64", "amd64"})
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: cloudSpec,
	})
	arches, err := common.SupportedArchitectures(env, imageConstraint)
	c.Assert(err, gc.IsNil)
	c.Assert(arches, jc.SameContents, []string{"amd64", "ppc64"})
}
