// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagerepo

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/docker"
)

type ImageRepoSuite struct {
	testing.IsolationSuite

	registry *MockRegistry
}

var _ = gc.Suite(&ImageRepoSuite{})

func (*ImageRepoSuite) TestImageRepoNoPath(c *gc.C) {
	_, err := NewImageRepo("")
	c.Assert(errors.Is(err, errors.NotValid), gc.Equals, true)
}

func (*ImageRepoSuite) TestImageRepoPath(c *gc.C) {
	imageRepo, err := NewImageRepo("blah")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(imageRepo.Path(), gc.Equals, "blah")
}

func (s *ImageRepoSuite) TestPing(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.registry.EXPECT().Ping().Return(nil)
	s.registry.EXPECT().Close().Return(nil)

	imageRepo, err := NewImageRepo("blah", WithRegistry(func(docker.ImageRepoDetails) (Registry, error) {
		return s.registry, nil
	}))
	c.Assert(err, jc.ErrorIsNil)

	err = imageRepo.Ping()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ImageRepoSuite) TestRequestDetails(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.registry.EXPECT().Ping().Return(nil)
	s.registry.EXPECT().ImageRepoDetails().Return(docker.ImageRepoDetails{
		Repository: "foobar",
	})
	s.registry.EXPECT().Close().Return(nil)

	imageRepo, err := NewImageRepo("blah", WithRegistry(func(docker.ImageRepoDetails) (Registry, error) {
		return s.registry, nil
	}))
	c.Assert(err, jc.ErrorIsNil)

	details, err := imageRepo.RequestDetails()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details, gc.DeepEquals, docker.ImageRepoDetails{
		Repository: "foobar",
	})
}

func (s *ImageRepoSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.registry = NewMockRegistry(ctrl)

	return ctrl
}
