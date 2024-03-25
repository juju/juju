// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v5"
	"github.com/juju/juju/core/network"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type importSuite struct {
	coordinator   *MockCoordinator
	spaceService  *MockImportSpaceService
	subnetService *MockImportSubnetService
}

var _ = gc.Suite(&importSuite{})

func (s *importSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.spaceService = NewMockImportSpaceService(ctrl)
	s.subnetService = NewMockImportSubnetService(ctrl)

	return ctrl
}

func (s *importSuite) newImportOperation() *importOperation {
	return &importOperation{
		spaceService:  s.spaceService,
		subnetService: s.subnetService,
	}
}

func (s *importSuite) TestImportSubnetWithoutSpaces(c *gc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	model.AddSubnet(description.SubnetArgs{
		ID:                "previousID",
		CIDR:              "10.0.0.0/24",
		ProviderId:        "subnet-provider-id",
		ProviderNetworkId: "subnet-provider-network-id",
		VLANTag:           42,
		AvailabilityZones: []string{"az1", "az2"},
		FanLocalUnderlay:  "192.168.0.0/12",
		FanOverlay:        "10.0.0.0/8",
	})
	s.subnetService.EXPECT().AddSubnet(gomock.Any(), network.SubnetInfo{
		CIDR:              "10.0.0.0/24",
		ProviderId:        "subnet-provider-id",
		ProviderNetworkId: "subnet-provider-network-id",
		VLANTag:           42,
		AvailabilityZones: []string{"az1", "az2"},
		FanInfo: &network.FanCIDRs{
			FanLocalUnderlay: "192.168.0.0/12",
			FanOverlay:       "10.0.0.0/8",
		},
	})

	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) TestImportSubnetAndSpaceNotLinked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	model.AddSubnet(description.SubnetArgs{
		ID:                "previous-subnet-id",
		CIDR:              "10.0.0.0/24",
		ProviderId:        "subnet-provider-id",
		ProviderNetworkId: "subnet-provider-network-id",
		VLANTag:           42,
		AvailabilityZones: []string{"az1", "az2"},
	})
	s.subnetService.EXPECT().AddSubnet(gomock.Any(), network.SubnetInfo{
		CIDR:              "10.0.0.0/24",
		ProviderId:        "subnet-provider-id",
		ProviderNetworkId: "subnet-provider-network-id",
		VLANTag:           42,
		AvailabilityZones: []string{"az1", "az2"},
	})
	model.AddSpace(description.SpaceArgs{
		Id:         "previous-space-id",
		Name:       "space-name",
		ProviderID: "space-provider-id",
	})
	s.spaceService.EXPECT().AddSpace(gomock.Any(), "space-name", network.Id("space-provider-id"), nil)

	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) TestImportSpaceWithSubnet(c *gc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	model.AddSpace(description.SpaceArgs{
		Id:         "previous-space-id",
		Name:       "space-name",
		ProviderID: "space-provider-id",
	})
	s.spaceService.EXPECT().AddSpace(gomock.Any(), "space-name", network.Id("space-provider-id"), nil).
		Return(network.Id("new-space-id"), nil)
	s.spaceService.EXPECT().Space(gomock.Any(), "new-space-id").
		Return(&network.SpaceInfo{
			ID:         "new-space-id",
			Name:       "space-name",
			ProviderId: network.Id("space-provider-id"),
		}, nil)
	model.AddSubnet(description.SubnetArgs{
		ID:                "previous-subnet-id",
		CIDR:              "10.0.0.0/24",
		ProviderId:        "subnet-provider-id",
		ProviderNetworkId: "subnet-provider-network-id",
		VLANTag:           42,
		AvailabilityZones: []string{"az1", "az2"},
		SpaceID:           "previous-space-id",
		SpaceName:         "space-name",
		ProviderSpaceId:   "space-provider-id",
	})
	s.subnetService.EXPECT().AddSubnet(gomock.Any(), network.SubnetInfo{
		CIDR:              "10.0.0.0/24",
		ProviderId:        "subnet-provider-id",
		ProviderNetworkId: "subnet-provider-network-id",
		VLANTag:           42,
		AvailabilityZones: []string{"az1", "az2"},
		SpaceID:           "new-space-id",
		SpaceName:         "space-name",
		ProviderSpaceId:   "space-provider-id",
	})

	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}
