// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type exportSuite struct {
	coordinator   *MockCoordinator
	exportService *MockExportService
}

var _ = gc.Suite(&exportSuite{})

func (s *exportSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.exportService = NewMockExportService(ctrl)

	return ctrl
}

func (s *exportSuite) newExportOperation(c *gc.C) *exportOperation {
	return &exportOperation{
		exportService: s.exportService,
		logger:        loggertesting.WrapCheckLog(c),
	}
}
func (s *exportSuite) TestExport(c *gc.C) {
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
	err := op.Execute(context.Background(), dst)
	c.Assert(err, jc.ErrorIsNil)

	actualSpaces := dst.Spaces()
	c.Assert(len(actualSpaces), gc.Equals, 1)
	c.Assert(actualSpaces[0].Name(), gc.Equals, string(spaces[0].Name))
	c.Assert(actualSpaces[0].ProviderID(), gc.Equals, string(spaces[0].ProviderId))

	actualSubnets := dst.Subnets()
	c.Assert(len(actualSubnets), gc.Equals, 1)
	c.Assert(actualSubnets[0].CIDR(), gc.Equals, subnets[0].CIDR)
	c.Assert(actualSubnets[0].VLANTag(), gc.Equals, subnets[0].VLANTag)
	c.Assert(actualSubnets[0].AvailabilityZones(), jc.SameContents, subnets[0].AvailabilityZones)
	c.Assert(actualSubnets[0].SpaceID(), gc.Equals, subnets[0].SpaceID)
	c.Assert(actualSubnets[0].SpaceName(), gc.Equals, subnets[0].SpaceName)
	c.Assert(actualSubnets[0].ProviderId(), gc.Equals, string(subnets[0].ProviderId))
	c.Assert(actualSubnets[0].ProviderSpaceId(), gc.Equals, string(subnets[0].ProviderSpaceId))
	c.Assert(actualSubnets[0].ProviderNetworkId(), gc.Equals, string(subnets[0].ProviderNetworkId))
}

func (s *exportSuite) TestExportSpacesNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})

	s.exportService.EXPECT().GetAllSpaces(gomock.Any()).
		Return(nil, coreerrors.NotFound)

	op := s.newExportOperation(c)
	err := op.Execute(context.Background(), dst)
	c.Assert(err, gc.ErrorMatches, ".*not found")
}

func (s *exportSuite) TestExportSubnetsNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})

	s.exportService.EXPECT().GetAllSpaces(gomock.Any()).
		Return(nil, nil)
	s.exportService.EXPECT().GetAllSubnets(gomock.Any()).
		Return(nil, coreerrors.NotFound)

	op := s.newExportOperation(c)
	err := op.Execute(context.Background(), dst)
	c.Assert(err, gc.ErrorMatches, ".*not found")
}
