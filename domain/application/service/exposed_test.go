// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
)

type exposedServiceSuite struct {
	baseSuite
}

var _ = gc.Suite(&exposedServiceSuite{})

func (s *exposedServiceSuite) TestApplicationExposedNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(coreapplication.ID(""), applicationerrors.ApplicationNotFound)

	_, err := s.service.ApplicationExposed(context.Background(), "foo")
	c.Assert(err, gc.ErrorMatches, "application not found")
}

func (s *exposedServiceSuite) TestApplicationExposed(c *gc.C) {
	defer s.setupMocks(c).Finish()

	applicationUUID := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)
	s.state.EXPECT().ApplicationExposed(gomock.Any(), applicationUUID).Return(true, nil)

	exposed, err := s.service.ApplicationExposed(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(exposed, jc.IsTrue)
}

func (s *exposedServiceSuite) TestExposedEndpointsNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(coreapplication.ID(""), applicationerrors.ApplicationNotFound)

	_, err := s.service.GetExposedEndpoints(context.Background(), "foo")
	c.Assert(err, gc.ErrorMatches, "application not found")
}

func (s *exposedServiceSuite) TestExposedEndpoints(c *gc.C) {
	defer s.setupMocks(c).Finish()

	applicationUUID := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)
	expected := map[string]application.ExposedEndpoint{
		"endpoint0": {
			ExposeToSpaceIDs: set.NewStrings("space0", "space1"),
		},
		"endpoint1": {
			ExposeToCIDRs: set.NewStrings("10.0.0.0/24", "10.0.1.0/24"),
		},
	}
	s.state.EXPECT().GetExposedEndpoints(gomock.Any(), applicationUUID).Return(expected, nil)

	obtained, err := s.service.GetExposedEndpoints(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained, gc.DeepEquals, expected)
}

func (s *exposedServiceSuite) TestUnsetExposeSettingsNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(coreapplication.ID(""), applicationerrors.ApplicationNotFound)

	err := s.service.UnsetExposeSettings(context.Background(), "foo", set.NewStrings("endpoint0"))
	c.Assert(err, gc.ErrorMatches, "application not found")
}

func (s *exposedServiceSuite) TestUnsetExposeSettings(c *gc.C) {
	defer s.setupMocks(c).Finish()

	applicationUUID := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)
	exposedEndpoints := set.NewStrings("endpoint0", "endpoint1")
	s.state.EXPECT().UnsetExposeSettings(gomock.Any(), applicationUUID, exposedEndpoints).Return(nil)

	err := s.service.UnsetExposeSettings(context.Background(), "foo", exposedEndpoints)
	c.Assert(err, jc.ErrorIsNil)
}
