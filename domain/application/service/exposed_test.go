// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"errors"
	"testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
)

type exposedServiceSuite struct {
	baseSuite
}

func TestExposedServiceSuite(t *testing.T) {
	tc.Run(t, &exposedServiceSuite{})
}

func (s *exposedServiceSuite) TestApplicationExposedNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(coreapplication.ID(""), applicationerrors.ApplicationNotFound)

	_, err := s.service.IsApplicationExposed(c.Context(), "foo")
	c.Assert(err, tc.ErrorMatches, "application not found")
}

func (s *exposedServiceSuite) TestApplicationExposed(c *tc.C) {
	defer s.setupMocks(c).Finish()

	applicationUUID := coreapplication.GenID(c)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)
	s.state.EXPECT().IsApplicationExposed(gomock.Any(), applicationUUID).Return(true, nil)

	exposed, err := s.service.IsApplicationExposed(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exposed, tc.IsTrue)
}

func (s *exposedServiceSuite) TestExposedEndpointsNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(coreapplication.ID(""), applicationerrors.ApplicationNotFound)

	_, err := s.service.GetExposedEndpoints(c.Context(), "foo")
	c.Assert(err, tc.ErrorMatches, "application not found")
}

func (s *exposedServiceSuite) TestExposedEndpoints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	applicationUUID := coreapplication.GenID(c)
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

	obtained, err := s.service.GetExposedEndpoints(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained, tc.DeepEquals, expected)
}

func (s *exposedServiceSuite) TestUnsetExposeSettingsNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(coreapplication.ID(""), applicationerrors.ApplicationNotFound)

	err := s.service.UnsetExposeSettings(c.Context(), "foo", set.NewStrings("endpoint0"))
	c.Assert(err, tc.ErrorMatches, "application not found")
}

func (s *exposedServiceSuite) TestUnsetExposeSettings(c *tc.C) {
	defer s.setupMocks(c).Finish()

	applicationUUID := coreapplication.GenID(c)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)
	exposedEndpoints := set.NewStrings("endpoint0", "endpoint1")
	s.state.EXPECT().UnsetExposeSettings(gomock.Any(), applicationUUID, exposedEndpoints).Return(nil)

	err := s.service.UnsetExposeSettings(c.Context(), "foo", exposedEndpoints)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *exposedServiceSuite) TestMergeExposeSettingsNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(coreapplication.ID(""), applicationerrors.ApplicationNotFound)

	err := s.service.MergeExposeSettings(c.Context(), "foo", map[string]application.ExposedEndpoint{
		"endpoint0": {
			ExposeToSpaceIDs: set.NewStrings("space0", "space1"),
		},
		"endpoint1": {
			ExposeToCIDRs: set.NewStrings("10.0.0.0/24", "10.0.1.0/24"),
		},
	})
	c.Assert(err, tc.ErrorMatches, "application not found")
}

func (s *exposedServiceSuite) TestMergeExposeSettingsEndpointsExistError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	applicationUUID := coreapplication.GenID(c)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)
	s.state.EXPECT().EndpointsExist(gomock.Any(), applicationUUID, set.NewStrings("endpoint0", "endpoint1")).Return(errors.New("boom"))

	err := s.service.MergeExposeSettings(c.Context(), "foo", map[string]application.ExposedEndpoint{
		"endpoint0": {
			ExposeToSpaceIDs: set.NewStrings("space0", "space1"),
		},
		"endpoint1": {
			ExposeToCIDRs: set.NewStrings("10.0.0.0/24", "10.0.1.0/24"),
		},
	})
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *exposedServiceSuite) TestMergeExposeSettingsSpacesNotExist(c *tc.C) {
	defer s.setupMocks(c).Finish()

	applicationUUID := coreapplication.GenID(c)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)
	s.state.EXPECT().EndpointsExist(gomock.Any(), applicationUUID, set.NewStrings("endpoint0", "endpoint1")).Return(nil)
	s.state.EXPECT().SpacesExist(gomock.Any(), set.NewStrings("space0", "space1")).Return(errors.New("one or more of the provided spaces \\[space0 space1\\] do not exist"))

	err := s.service.MergeExposeSettings(c.Context(), "foo", map[string]application.ExposedEndpoint{
		"endpoint0": {
			ExposeToSpaceIDs: set.NewStrings("space0", "space1"),
		},
		"endpoint1": {
			ExposeToCIDRs: set.NewStrings("10.0.0.0/24", "10.0.1.0/24"),
		},
	})
	c.Assert(err, tc.ErrorMatches, "validating exposed endpoints to spaces .*: one or more of the provided spaces .* do not exist")
}

func (s *exposedServiceSuite) TestMergeExposeSettingsEmptyEndpointsList(c *tc.C) {
	defer s.setupMocks(c).Finish()

	applicationUUID := coreapplication.GenID(c)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)
	s.state.EXPECT().EndpointsExist(gomock.Any(), applicationUUID, set.NewStrings()).Return(nil)
	s.state.EXPECT().SpacesExist(gomock.Any(), set.NewStrings()).Return(nil)
	s.state.EXPECT().MergeExposeSettings(gomock.Any(), applicationUUID, map[string]application.ExposedEndpoint{
		"": {
			ExposeToCIDRs: set.NewStrings(firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		},
	}).Return(nil)

	err := s.service.MergeExposeSettings(c.Context(), "foo", map[string]application.ExposedEndpoint{})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *exposedServiceSuite) TestMergeExposeSettingsWildcard(c *tc.C) {
	defer s.setupMocks(c).Finish()

	applicationUUID := coreapplication.GenID(c)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)
	s.state.EXPECT().EndpointsExist(gomock.Any(), applicationUUID, set.NewStrings("endpoint1")).Return(nil)
	s.state.EXPECT().SpacesExist(gomock.Any(), set.NewStrings("space0", "space1")).Return(nil)
	s.state.EXPECT().MergeExposeSettings(gomock.Any(), applicationUUID, map[string]application.ExposedEndpoint{
		"": {
			ExposeToSpaceIDs: set.NewStrings("space0", "space1"),
		},
		"endpoint1": {
			ExposeToCIDRs: set.NewStrings("10.0.0.0/24", "10.0.1.0/24"),
		},
	}).Return(nil)

	err := s.service.MergeExposeSettings(c.Context(), "foo", map[string]application.ExposedEndpoint{
		"": {
			ExposeToSpaceIDs: set.NewStrings("space0", "space1"),
		},
		"endpoint1": {
			ExposeToCIDRs: set.NewStrings("10.0.0.0/24", "10.0.1.0/24"),
		},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *exposedServiceSuite) TestMergeExposeSettings(c *tc.C) {
	defer s.setupMocks(c).Finish()

	applicationUUID := coreapplication.GenID(c)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)
	s.state.EXPECT().EndpointsExist(gomock.Any(), applicationUUID, set.NewStrings("endpoint0", "endpoint1")).Return(nil)
	s.state.EXPECT().SpacesExist(gomock.Any(), set.NewStrings("space0", "space1")).Return(nil)
	s.state.EXPECT().MergeExposeSettings(gomock.Any(), applicationUUID, map[string]application.ExposedEndpoint{
		"endpoint0": {
			ExposeToSpaceIDs: set.NewStrings("space0", "space1"),
		},
		"endpoint1": {
			ExposeToCIDRs: set.NewStrings("10.0.0.0/24", "10.0.1.0/24"),
		},
	}).Return(nil)

	err := s.service.MergeExposeSettings(c.Context(), "foo", map[string]application.ExposedEndpoint{
		"endpoint0": {
			ExposeToSpaceIDs: set.NewStrings("space0", "space1"),
		},
		"endpoint1": {
			ExposeToCIDRs: set.NewStrings("10.0.0.0/24", "10.0.1.0/24"),
		},
	})
	c.Assert(err, tc.ErrorIsNil)
}
