// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemanager_test

import (
	"bytes"
	"io"
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/imagemanager"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/imagestorage"
)

type imageManagerSuite struct {
	jujutesting.JujuConnSuite

	imagemanager *imagemanager.ImageManagerAPI
	resources    *common.Resources
	authoriser   apiservertesting.FakeAuthorizer

	commontesting.BlockHelper
}

var _ = gc.Suite(&imageManagerSuite{})

func (s *imageManagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	var err error
	s.imagemanager, err = imagemanager.NewImageManagerAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)

	s.BlockHelper = commontesting.NewBlockHelper(s.APIState)
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })
}

func (s *imageManagerSuite) TestNewImageManagerAPIAcceptsClient(c *gc.C) {
	endPoint, err := imagemanager.NewImageManagerAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endPoint, gc.NotNil)
}

func (s *imageManagerSuite) TestNewImageManagerAPIRefusesNonClient(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Tag = names.NewUnitTag("mysql/0")
	anAuthoriser.EnvironManager = false
	endPoint, err := imagemanager.NewImageManagerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *imageManagerSuite) addImage(c *gc.C, content string) {
	var r io.Reader = bytes.NewReader([]byte(content))
	addedMetadata := &imagestorage.Metadata{
		EnvUUID:   s.State.EnvironUUID(),
		Kind:      "lxc",
		Series:    "trusty",
		Arch:      "amd64",
		Size:      int64(len(content)),
		SHA256:    "hash(" + content + ")",
		SourceURL: "http://lxc-trusty-amd64",
	}
	stor := s.State.ImageStorage()
	err := stor.AddImage(r, addedMetadata)
	c.Assert(err, gc.IsNil)
	_, rdr, err := stor.Image("lxc", "trusty", "amd64")
	c.Assert(err, jc.ErrorIsNil)
	rdr.Close()
}

func (s *imageManagerSuite) TestListAllImages(c *gc.C) {
	s.addImage(c, "image")
	args := params.ImageFilterParams{}
	result, err := s.imagemanager.ListImages(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Result, gc.HasLen, 1)
	dummyTime := time.Now()
	result.Result[0].Created = dummyTime
	c.Assert(result.Result[0], gc.Equals, params.ImageMetadata{
		Kind: "lxc", Arch: "amd64", Series: "trusty", URL: "http://lxc-trusty-amd64", Created: dummyTime,
	})
}

func (s *imageManagerSuite) TestListImagesWithSingleFilter(c *gc.C) {
	s.addImage(c, "image")
	args := params.ImageFilterParams{
		Images: []params.ImageSpec{
			{
				Kind:   "lxc",
				Series: "trusty",
				Arch:   "amd64",
			},
		},
	}
	result, err := s.imagemanager.ListImages(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Result, gc.HasLen, 1)
	dummyTime := time.Now()
	result.Result[0].Created = dummyTime
	c.Assert(result.Result[0], gc.Equals, params.ImageMetadata{
		Kind: "lxc", Arch: "amd64", Series: "trusty", URL: "http://lxc-trusty-amd64", Created: dummyTime,
	})
}

func (s *imageManagerSuite) TestListImagesWithMultipleFiltersFails(c *gc.C) {
	s.addImage(c, "image")
	args := params.ImageFilterParams{
		Images: []params.ImageSpec{
			{
				Kind:   "lxc",
				Series: "trusty",
				Arch:   "amd64",
			}, {
				Kind:   "lxc",
				Series: "precise",
				Arch:   "amd64",
			},
		},
	}
	_, err := s.imagemanager.ListImages(args)
	c.Assert(err, gc.ErrorMatches, "image filter with multiple terms not supported")
}

func (s *imageManagerSuite) TestDeleteImages(c *gc.C) {
	s.addImage(c, "image")
	args := params.ImageFilterParams{
		Images: []params.ImageSpec{
			{
				Kind:   "lxc",
				Series: "trusty",
				Arch:   "amd64",
			}, {
				Kind:   "lxc",
				Series: "precise",
				Arch:   "amd64",
			},
		},
	}
	results, err := s.imagemanager.DeleteImages(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: apiservertesting.NotFoundError("image lxc/precise/amd64")},
		},
	})
	stor := s.State.ImageStorage()
	_, _, err = stor.Image("lxc", "trusty", "amd64")
	c.Assert(err, gc.ErrorMatches, ".*-lxc-trusty-amd64 image metadata not found")
}

func (s *imageManagerSuite) TestBlockDeleteImages(c *gc.C) {
	s.addImage(c, "image")
	args := params.ImageFilterParams{
		Images: []params.ImageSpec{{
			Kind:   "lxc",
			Series: "trusty",
			Arch:   "amd64",
		}},
	}

	s.BlockAllChanges(c, "TestBlockDeleteImages")
	_, err := s.imagemanager.DeleteImages(args)
	// Check that the call is blocked
	s.AssertBlocked(c, err, "TestBlockDeleteImages")
	// Check the image still exists.
	stor := s.State.ImageStorage()
	_, rdr, err := stor.Image("lxc", "trusty", "amd64")
	c.Assert(err, jc.ErrorIsNil)
	rdr.Close()
}
