// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	loggertesting "github.com/juju/juju/internal/logger/testing"
)

var _ = gc.Suite(&FacadeSuite{})

type FacadeSuite struct {
	BaseSuite
}

func (s *FacadeSuite) TestNewFacadeOkay(c *gc.C) {
	defer s.setupMocks(c).Finish()
	_, err := NewResourcesAPI(s.applicationService, s.resourceService, s.factory, loggertesting.WrapCheckLog(c))
	c.Check(err, jc.ErrorIsNil)
}

func (s *FacadeSuite) TestNewFacadeMissingApplicationService(c *gc.C) {
	defer s.setupMocks(c).Finish()
	_, err := NewResourcesAPI(nil, s.resourceService, s.factory, loggertesting.WrapCheckLog(c))
	c.Check(err, gc.ErrorMatches, ".*missing application service.*")
}

func (s *FacadeSuite) TestNewFacadeMissingResourceService(c *gc.C) {
	defer s.setupMocks(c).Finish()
	_, err := NewResourcesAPI(s.applicationService, nil, s.factory, loggertesting.WrapCheckLog(c))
	c.Check(err, gc.ErrorMatches, ".*missing resource service.*")
}

func (s *FacadeSuite) TestNewFacadeMissingFactory(c *gc.C) {
	defer s.setupMocks(c).Finish()
	_, err := NewResourcesAPI(s.applicationService, s.resourceService, nil, loggertesting.WrapCheckLog(c))
	c.Check(err, gc.ErrorMatches, ".*missing factory for new repository.*")
}
