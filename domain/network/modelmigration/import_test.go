// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/description/v9"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network/internal"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	importService    *MockImportService
	migrationService *MockMigrationService
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.importService = NewMockImportService(ctrl)
	s.migrationService = NewMockMigrationService(ctrl)

	c.Cleanup(func() {
		s.importService = nil
		s.migrationService = nil
	})

	return ctrl
}

func (s *importSuite) newImportOperation(c *tc.C) *importOperation {
	return &importOperation{
		importService:    s.importService,
		migrationService: s.migrationService,
		logger:           loggertesting.WrapCheckLog(c),
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
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
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
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
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
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportLinkLayerDevices(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	model := description.NewModel(description.ModelArgs{})
	dArgs := description.LinkLayerDeviceArgs{
		Name:        "test-device",
		MTU:         1500,
		ProviderID:  "net-lxdbr0",
		MachineID:   "77",
		Type:        "ethernet",
		MACAddress:  "00:16:3e:ad:4e:01",
		IsAutoStart: true,
		IsUp:        true,
	}
	model.AddLinkLayerDevice(dArgs)

	args := []internal.ImportLinkLayerDevice{
		{
			IsAutoStart: dArgs.IsAutoStart,
			IsEnabled:   dArgs.IsUp,
			MTU:         ptr(int64(dArgs.MTU)),
			MachineID:   dArgs.MachineID,
			MACAddress:  ptr(dArgs.MACAddress),
			Name:        dArgs.Name,
			ProviderID:  ptr(dArgs.ProviderID),
			Type:        network.EthernetDevice,
		},
	}
	s.migrationService.EXPECT().ImportLinkLayerDevices(gomock.Any(), lldArgMatcher{c: c, expected: args}).Return(nil)

	// Act
	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportLinkLayerDevicesOptionalValues(c *tc.C) {
	// Arrange: ensure input not containing an MTU, ProviderID, nor
	// MACAddress values to see nil values in the data passed to the
	// service.
	defer s.setupMocks(c).Finish()
	model := description.NewModel(description.ModelArgs{})
	dArgs := description.LinkLayerDeviceArgs{
		Name:        "test-device",
		MachineID:   "77",
		Type:        "ethernet",
		IsAutoStart: true,
		IsUp:        true,
	}
	model.AddLinkLayerDevice(dArgs)

	args := []internal.ImportLinkLayerDevice{
		{
			IsAutoStart: dArgs.IsAutoStart,
			IsEnabled:   dArgs.IsUp,
			MachineID:   dArgs.MachineID,
			Name:        dArgs.Name,
			Type:        network.EthernetDevice,
		},
	}
	s.migrationService.EXPECT().ImportLinkLayerDevices(gomock.Any(), lldArgMatcher{c: c, expected: args}).Return(nil)

	// Act
	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestRollbackLinkLayerDevices(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	model := description.NewModel(description.ModelArgs{})
	dArgs := description.LinkLayerDeviceArgs{}
	model.AddLinkLayerDevice(dArgs)
	s.migrationService.EXPECT().DeleteImportedLinkLayerDevices(gomock.Any()).Return(nil)

	// Act
	op := s.newImportOperation(c)
	err := op.Rollback(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestRollbackLinkLayerDevicesNoData(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	model := description.NewModel(description.ModelArgs{})

	// Act
	op := s.newImportOperation(c)
	err := op.Rollback(c.Context(), model)

	// Assert: with no link layer device data, there is no failure.
	c.Assert(err, tc.ErrorIsNil)
}

// lldArgMatcher verifies the args for ImportLinkLayerDevice.
type lldArgMatcher struct {
	c        *tc.C
	expected []internal.ImportLinkLayerDevice
}

func (m lldArgMatcher) Matches(x interface{}) bool {
	input, ok := x.([]internal.ImportLinkLayerDevice)
	if !ok {
		return false
	}
	// UUIDs are assigned in the code under test. Ensure they exist, then
	// remove it to enable SameContents checks over the other fields.
	for i, in := range input {
		m.c.Check(in.UUID, tc.Not(tc.Equals), "")
		out := in
		out.UUID = ""
		input[i] = out
	}
	return m.c.Check(input, tc.SameContents, m.expected)
}

func (lldArgMatcher) String() string {
	return "matches args for ImportLinkLayerDevice"
}
