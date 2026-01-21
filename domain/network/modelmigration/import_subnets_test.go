// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/description/v11"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/network"
	networktesting "github.com/juju/juju/core/network/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSubnetsSuite struct {
	importService *MockSubnetsImportService
}

func TestImportSubnetsSuite(t *testing.T) {
	tc.Run(t, &importSubnetsSuite{})
}

func (s *importSubnetsSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.importService = NewMockSubnetsImportService(ctrl)

	c.Cleanup(func() {
		s.importService = nil
	})

	return ctrl
}

func (s *importSubnetsSuite) newImportOperation(c *tc.C) *importSubnetsOperation {
	return &importSubnetsOperation{
		importService: s.importService,
		logger:        loggertesting.WrapCheckLog(c),
	}
}

func (s *importSubnetsSuite) TestImportIAASSubnetWithoutSpaces(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{
		Type: description.IAAS,
	})
	model.AddSubnet(description.SubnetArgs{
		ID:                "previousID",
		CIDR:              "192.0.2.0/24",
		ProviderId:        "subnet-provider-id",
		ProviderNetworkId: "subnet-provider-network-id",
		VLANTag:           42,
		AvailabilityZones: []string{"az1", "az2"},
		FanLocalUnderlay:  "198.51.100.0/24",
		FanOverlay:        "203.0.113.0/24",
	})
	s.importService.EXPECT().AddSubnet(gomock.Any(), network.SubnetInfo{
		CIDR:              "192.0.2.0/24",
		ProviderId:        "subnet-provider-id",
		ProviderNetworkId: "subnet-provider-network-id",
		VLANTag:           42,
		AvailabilityZones: []string{"az1", "az2"},
	})

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSubnetsSuite) TestImportIAASSubnetAndSpaceNotLinked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{
		Type: description.IAAS,
	})
	model.AddSubnet(description.SubnetArgs{
		ID:                "previous-subnet-id",
		CIDR:              "192.0.2.0/24",
		ProviderId:        "subnet-provider-id",
		ProviderNetworkId: "subnet-provider-network-id",
		VLANTag:           42,
		AvailabilityZones: []string{"az1", "az2"},
	})
	s.importService.EXPECT().AddSubnet(gomock.Any(), network.SubnetInfo{
		CIDR:              "192.0.2.0/24",
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
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSubnetsSuite) TestImportIAASSpaceWithSubnet(c *tc.C) {
	defer s.setupMocks(c).Finish()

	spUUID := networktesting.GenSpaceUUID(c)

	model := description.NewModel(description.ModelArgs{
		Type: description.IAAS,
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
	s.importService.EXPECT().AddSpace(gomock.Any(), spaceInfo).
		Return(spUUID, nil)
	s.importService.EXPECT().Space(gomock.Any(), spUUID).
		Return(&network.SpaceInfo{
			ID:         spUUID,
			Name:       "space-name",
			ProviderId: network.Id("space-provider-id"),
		}, nil)
	model.AddSubnet(description.SubnetArgs{
		ID:                "previous-subnet-id",
		CIDR:              "192.0.2.0/24",
		ProviderId:        "subnet-provider-id",
		ProviderNetworkId: "subnet-provider-network-id",
		VLANTag:           42,
		AvailabilityZones: []string{"az1", "az2"},
		SpaceID:           "previous-space-id",
		SpaceName:         "space-name",
		ProviderSpaceId:   "space-provider-id",
	})
	s.importService.EXPECT().AddSubnet(gomock.Any(), network.SubnetInfo{
		CIDR:              "192.0.2.0/24",
		ProviderId:        "subnet-provider-id",
		ProviderNetworkId: "subnet-provider-network-id",
		VLANTag:           42,
		AvailabilityZones: []string{"az1", "az2"},
		SpaceID:           spUUID,
		SpaceName:         "space-name",
		ProviderSpaceId:   "space-provider-id",
	})

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSubnetsSuite) TestImportSpaces(c *tc.C) {
	defer s.setupMocks(c).Finish()

	spUUID := networktesting.GenSpaceUUID(c)

	model := description.NewModel(description.ModelArgs{})
	model.AddSpace(description.SpaceArgs{
		Id:         "0",
		Name:       network.AlphaSpaceName.String(),
		ProviderID: "alpha-provider-id",
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
	// don't import the alpha space
	s.importService.EXPECT().AddSpace(gomock.Any(), spaceInfo).
		Return(spUUID, nil)

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}
