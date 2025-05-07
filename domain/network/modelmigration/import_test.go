// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/network"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	coordinator   *MockCoordinator
	importService *MockImportService
}

var _ = tc.Suite(&importSuite{})

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.importService = NewMockImportService(ctrl)

	return ctrl
}

func (s *importSuite) newImportOperation(c *tc.C) *importOperation {
	return &importOperation{
		importService: s.importService,
		logger:        loggertesting.WrapCheckLog(c),
	}
}

func (s *importSuite) TestImportSubnetWithoutSpaces(c *tc.C) {
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
	s.importService.EXPECT().AddSubnet(gomock.Any(), network.SubnetInfo{
		CIDR:              "10.0.0.0/24",
		ProviderId:        "subnet-provider-id",
		ProviderNetworkId: "subnet-provider-network-id",
		VLANTag:           42,
		AvailabilityZones: []string{"az1", "az2"},
	})

	op := s.newImportOperation(c)
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) TestImportSubnetAndSpaceNotLinked(c *tc.C) {
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
	s.importService.EXPECT().AddSubnet(gomock.Any(), network.SubnetInfo{
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
	spaceInfo := network.SpaceInfo{
		Name:       "space-name",
		ProviderId: "space-provider-id",
	}
	s.importService.EXPECT().AddSpace(gomock.Any(), spaceInfo)

	op := s.newImportOperation(c)
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) TestImportSpaceWithSubnet(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	model.AddSpace(description.SpaceArgs{
		Id:         "previous-space-id",
		Name:       "space-name",
		ProviderID: "space-provider-id",
	})
	spaceInfo := network.SpaceInfo{
		Name:       "space-name",
		ProviderId: "space-provider-id",
	}
	s.importService.EXPECT().AddSpace(gomock.Any(), spaceInfo).
		Return(network.Id("new-space-id"), nil)
	s.importService.EXPECT().Space(gomock.Any(), "new-space-id").
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
	s.importService.EXPECT().AddSubnet(gomock.Any(), network.SubnetInfo{
		CIDR:              "10.0.0.0/24",
		ProviderId:        "subnet-provider-id",
		ProviderNetworkId: "subnet-provider-network-id",
		VLANTag:           42,
		AvailabilityZones: []string{"az1", "az2"},
		SpaceID:           "new-space-id",
		SpaceName:         "space-name",
		ProviderSpaceId:   "space-provider-id",
	})

	op := s.newImportOperation(c)
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}
