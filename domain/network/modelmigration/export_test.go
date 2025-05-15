// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"github.com/juju/description/v9"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type exportSuite struct {
	coordinator   *MockCoordinator
	exportService *MockExportService
}

var _ = tc.Suite(&exportSuite{})

func (s *exportSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.exportService = NewMockExportService(ctrl)

	return ctrl
}

func (s *exportSuite) newExportOperation(c *tc.C) *exportOperation {
	return &exportOperation{
		exportService: s.exportService,
		logger:        loggertesting.WrapCheckLog(c),
	}
}
func (s *exportSuite) TestExport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})

	spaces := network.SpaceInfos{
		{
			ID:         "1",
			Name:       "space1",
			ProviderId: "provider-space-1",
		},
	}
	s.exportService.EXPECT().GetAllSpaces(gomock.Any()).
		Return(spaces, nil)
	subnets := network.SubnetInfos{
		{
			ID:                "1",
			CIDR:              "10.0.0.0/24",
			VLANTag:           42,
			AvailabilityZones: []string{"az1", "az2"},
			SpaceID:           "1",
			SpaceName:         "space1",
			ProviderId:        "provider-subnet-1",
			ProviderSpaceId:   "provider-space-1",
			ProviderNetworkId: "provider-network-1",
		},
	}
	s.exportService.EXPECT().GetAllSubnets(gomock.Any()).
		Return(subnets, nil)

	op := s.newExportOperation(c)
	err := op.Execute(c.Context(), dst)
	c.Assert(err, tc.ErrorIsNil)

	actualSpaces := dst.Spaces()
	c.Assert(len(actualSpaces), tc.Equals, 1)
	c.Assert(actualSpaces[0].Name(), tc.Equals, string(spaces[0].Name))
	c.Assert(actualSpaces[0].ProviderID(), tc.Equals, string(spaces[0].ProviderId))

	actualSubnets := dst.Subnets()
	c.Assert(len(actualSubnets), tc.Equals, 1)
	c.Assert(actualSubnets[0].CIDR(), tc.Equals, subnets[0].CIDR)
	c.Assert(actualSubnets[0].VLANTag(), tc.Equals, subnets[0].VLANTag)
	c.Assert(actualSubnets[0].AvailabilityZones(), tc.SameContents, subnets[0].AvailabilityZones)
	c.Assert(actualSubnets[0].SpaceID(), tc.Equals, subnets[0].SpaceID)
	c.Assert(actualSubnets[0].SpaceName(), tc.Equals, subnets[0].SpaceName)
	c.Assert(actualSubnets[0].ProviderId(), tc.Equals, string(subnets[0].ProviderId))
	c.Assert(actualSubnets[0].ProviderSpaceId(), tc.Equals, string(subnets[0].ProviderSpaceId))
	c.Assert(actualSubnets[0].ProviderNetworkId(), tc.Equals, string(subnets[0].ProviderNetworkId))
}

func (s *exportSuite) TestExportSpacesNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})

	s.exportService.EXPECT().GetAllSpaces(gomock.Any()).
		Return(nil, coreerrors.NotFound)

	op := s.newExportOperation(c)
	err := op.Execute(c.Context(), dst)
	c.Assert(err, tc.ErrorMatches, ".*not found")
}

func (s *exportSuite) TestExportSubnetsNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})

	s.exportService.EXPECT().GetAllSpaces(gomock.Any()).
		Return(nil, nil)
	s.exportService.EXPECT().GetAllSubnets(gomock.Any()).
		Return(nil, coreerrors.NotFound)

	op := s.newExportOperation(c)
	err := op.Execute(c.Context(), dst)
	c.Assert(err, tc.ErrorMatches, ".*not found")
}
