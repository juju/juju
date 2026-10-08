// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/description/v12"
	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	networktesting "github.com/juju/juju/core/network/testing"
	domainnetwork "github.com/juju/juju/domain/network"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSubnetsSuite struct {
	importService *MockSubnetImportService
	spaceService  *MockSpaceImportService
}

func TestImportSubnetsSuite(t *testing.T) {
	tc.Run(t, &importSubnetsSuite{})
}

func (s *importSubnetsSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.importService = NewMockSubnetImportService(ctrl)
	s.spaceService = NewMockSpaceImportService(ctrl)

	c.Cleanup(func() {
		s.importService = nil
		s.spaceService = nil
	})

	return ctrl
}

func (s *importSubnetsSuite) newImportOperation(c *tc.C) *importSubnetsOperation {
	return &importSubnetsOperation{
		spaceService:  s.spaceService,
		importService: s.importService,
		logger:        loggertesting.WrapCheckLog(c),
	}
}

func (s *importSubnetsSuite) TestImportIAASSubnetWithoutSpacesNotLXD(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.importService.EXPECT().GetModelCloudType(c.Context()).Return("ec1", nil)
	model := description.NewModel(description.ModelArgs{
		Type: description.IAAS,
	})
	model.AddSubnet(description.SubnetArgs{
		ID:                "previousID",
		CIDR:              "192.0.2.0/24",
		ProviderId:        "network-provider-id",
		ProviderNetworkId: "subnet-provider-network-id",
		VLANTag:           42,
		AvailabilityZones: []string{"az1", "az2"},
		FanLocalUnderlay:  "198.51.100.0/24",
		FanOverlay:        "203.0.113.0/24",
	})
	s.importService.EXPECT().ImportSubnets(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, args []domainnetwork.ImportSubnetArgs) error {
			c.Assert(args, tc.HasLen, 1)
			c.Check(args[0].CIDR, tc.Equals, "192.0.2.0/24")
			c.Check(args[0].ProviderId, tc.Equals, network.Id("network-provider-id"))
			c.Check(args[0].ProviderNetworkId, tc.Equals, network.Id("subnet-provider-network-id"))
			c.Check(args[0].VLANTag, tc.Equals, 42)
			c.Check(args[0].AvailabilityZones, tc.SameContents, []string{"az1", "az2"})
			return nil
		})

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSubnetsSuite) TestImportIAASSubnetWithoutSpacesLXD(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.importService.EXPECT().GetModelCloudType(c.Context()).Return("lxd", nil)
	model := description.NewModel(description.ModelArgs{
		Type: description.IAAS,
	})
	model.AddSubnet(description.SubnetArgs{
		ID:                "previousID",
		CIDR:              "192.0.2.0/24",
		ProviderId:        "subnet-docker0-192.0.2.0/24",
		ProviderNetworkId: "net-docker",
		VLANTag:           42,
		AvailabilityZones: []string{"az1", "az2"},
		FanLocalUnderlay:  "198.51.100.0/24",
		FanOverlay:        "203.0.113.0/24",
	})
	s.importService.EXPECT().ImportSubnets(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, args []domainnetwork.ImportSubnetArgs) error {
			c.Assert(args, tc.HasLen, 1)
			c.Check(args[0].CIDR, tc.Equals, "192.0.2.0/24")
			c.Check(args[0].ProviderNetworkId, tc.Equals, network.Id("docker"))
			c.Check(args[0].VLANTag, tc.Equals, 42)
			c.Check(args[0].AvailabilityZones, tc.SameContents, []string{"az1", "az2"})
			return nil
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

	s.importService.EXPECT().GetModelCloudType(c.Context()).Return("ec2", nil)
	model.AddSubnet(description.SubnetArgs{
		ID:                "previous-subnet-id",
		CIDR:              "192.0.2.0/24",
		ProviderId:        "subnet-provider-id",
		ProviderNetworkId: "subnet-provider-network-id",
		VLANTag:           42,
		AvailabilityZones: []string{"az1", "az2"},
	})
	s.importService.EXPECT().ImportSubnets(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, args []domainnetwork.ImportSubnetArgs) error {
			c.Assert(args, tc.HasLen, 1)
			c.Check(args[0].CIDR, tc.Equals, "192.0.2.0/24")
			c.Check(args[0].ProviderId, tc.Equals, network.Id("subnet-provider-id"))
			c.Check(args[0].ProviderNetworkId, tc.Equals, network.Id("subnet-provider-network-id"))
			c.Check(args[0].VLANTag, tc.Equals, 42)
			c.Check(args[0].AvailabilityZones, tc.SameContents, []string{"az1", "az2"})
			return nil
		})
	model.AddSpace(description.SpaceArgs{
		Id:         "previous-space-id",
		Name:       "space-name",
		ProviderID: "space-provider-id",
	})
	s.spaceService.EXPECT().AddSpace(gomock.Any(), domainnetwork.AddSpaceArgs{
		Name:       "space-name",
		ProviderID: "space-provider-id",
	})

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSubnetsSuite) TestImportIAASSpaceWithSubnet(c *tc.C) {
	defer s.setupMocks(c).Finish()

	spUUID := networktesting.GenSpaceUUID(c)

	s.importService.EXPECT().GetModelCloudType(c.Context()).Return("ec2", nil)
	model := description.NewModel(description.ModelArgs{
		Type: description.IAAS,
	})
	model.AddSpace(description.SpaceArgs{
		Id:         "previous-space-id",
		Name:       "space-name",
		ProviderID: "space-provider-id",
	})
	s.spaceService.EXPECT().AddSpace(gomock.Any(), domainnetwork.AddSpaceArgs{
		Name:       "space-name",
		ProviderID: "space-provider-id",
	}).Return(spUUID, nil)
	model.AddSubnet(description.SubnetArgs{
		ID:                "previous-subnet-id",
		CIDR:              "192.0.2.0/24",
		ProviderId:        "subnet-provider-id",
		ProviderNetworkId: "subnet-provider-network-id",
		VLANTag:           42,
		AvailabilityZones: []string{"az1", "az2"},
		SpaceID:           "previous-space-id",
	})
	s.importService.EXPECT().ImportSubnets(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, args []domainnetwork.ImportSubnetArgs) error {
			c.Assert(args, tc.HasLen, 1)
			c.Check(args[0].CIDR, tc.Equals, "192.0.2.0/24")
			c.Check(args[0].ProviderId, tc.Equals, network.Id("subnet-provider-id"))
			c.Check(args[0].ProviderNetworkId, tc.Equals, network.Id("subnet-provider-network-id"))
			c.Check(args[0].VLANTag, tc.Equals, 42)
			c.Check(args[0].AvailabilityZones, tc.SameContents, []string{"az1", "az2"})
			c.Check(args[0].SpaceID, tc.Equals, spUUID)
			return nil
		})

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSubnetsSuite) TestImportSpaces(c *tc.C) {
	defer s.setupMocks(c).Finish()

	spUUID := networktesting.GenSpaceUUID(c)

	s.importService.EXPECT().GetModelCloudType(c.Context()).Return("ec2", nil)
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

	// don't import the alpha space
	s.spaceService.EXPECT().AddSpace(gomock.Any(), domainnetwork.AddSpaceArgs{
		Name:       "space-name",
		ProviderID: "space-provider-id",
	}).Return(spUUID, nil)

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSubnetsSuite) TestImportIASSubnetsWithoutUUIDsGeneratesDistinctUUIDs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// 3.6 exports subnets at schema version 6, which has no uuid field.
	// The import must generate a UUID for each subnet.
	s.importService.EXPECT().GetModelCloudType(c.Context()).Return("ec2", nil)
	model := description.NewModel(description.ModelArgs{
		Type: description.IAAS,
	})
	model.AddSubnet(description.SubnetArgs{
		ID:   "subnet-id-1",
		CIDR: "192.0.2.0/24",
	})
	model.AddSubnet(description.SubnetArgs{
		ID:   "subnet-id-2",
		CIDR: "198.51.100.0/24",
	})
	s.importService.EXPECT().ImportSubnets(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, args []domainnetwork.ImportSubnetArgs) error {
			c.Assert(args, tc.HasLen, 2)
			c.Check(args[0].UUID, tc.Not(tc.Equals), "")
			c.Check(args[1].UUID, tc.Not(tc.Equals), "")
			c.Check(args[0].UUID, tc.Not(tc.Equals), args[1].UUID)
			return nil
		})

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSubnetsSuite) TestImportIASSubnetWithUUIDPreservesUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// 4.x exports subnets at schema version 7, which includes a uuid
	// field. The import must preserve the source-model UUID.
	subnetUUID := tc.Must(c, domainnetwork.NewSubnetUUID)
	s.importService.EXPECT().GetModelCloudType(c.Context()).Return("ec2", nil)
	model := description.NewModel(description.ModelArgs{
		Type: description.IAAS,
	})
	model.AddSubnet(description.SubnetArgs{
		ID:   "subnet-id-1",
		UUID: subnetUUID.String(),
		CIDR: "192.0.2.0/24",
	})
	s.importService.EXPECT().ImportSubnets(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, args []domainnetwork.ImportSubnetArgs) error {
			c.Assert(args, tc.HasLen, 1)
			c.Check(args[0].UUID, tc.Equals, subnetUUID)
			return nil
		})

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSubnetsSuite) TestImportCAASSubnet(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{
		Type: description.CAAS,
	})
	model.AddSubnet(description.SubnetArgs{
		CIDR: network.FallbackSubnetInfo[0].CIDR,
	})
	model.AddSubnet(description.SubnetArgs{
		CIDR: network.FallbackSubnetInfo[1].CIDR,
	})
	s.importService.EXPECT().ImportSubnets(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, args []domainnetwork.ImportSubnetArgs) error {
			c.Assert(args, tc.HasLen, 2)
			c.Check(args[0].CIDR, tc.Equals, network.FallbackSubnetInfo[0].CIDR)
			c.Check(args[1].CIDR, tc.Equals, network.FallbackSubnetInfo[1].CIDR)
			return nil
		})

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}
