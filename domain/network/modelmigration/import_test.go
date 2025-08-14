// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"maps"
	"slices"
	"testing"

	"github.com/juju/collections/transform"
	"github.com/juju/description/v10"
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

	spUUID := network.GenSpaceUUID(c)

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
		Return(spUUID, nil)
	s.importService.EXPECT().Space(gomock.Any(), spUUID).
		Return(&network.SpaceInfo{
			ID:         spUUID,
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
		SpaceID:           spUUID,
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

func (s *importSuite) TestImportLinkLayerDevicesWithAddresses(c *tc.C) {
	// Arrange
	// machine 0 eth 0 => 2 addr 10.0.0.1 & fd42:9102:88cb:dce3:216:3eff:fe59:a9dc, origin provider
	// machine 0 eth 1 => 1 addr 198.0.0.1, origin provider
	// machine 1 eth 0 => 1 addr 172.0.0.1 origin machine
	defer s.setupMocks(c).Finish()
	model := description.NewModel(description.ModelArgs{})

	model.AddLinkLayerDevice(description.LinkLayerDeviceArgs{
		Name:      "eth0",
		MachineID: "0",
	})
	model.AddLinkLayerDevice(description.LinkLayerDeviceArgs{
		Name:      "eth1",
		MachineID: "0",
	})
	model.AddLinkLayerDevice(description.LinkLayerDeviceArgs{
		Name:      "eth0",
		MachineID: "1",
	})
	model.AddIPAddress(description.IPAddressArgs{
		DeviceName: "eth0",
		MachineID:  "0",

		ProviderID:       "address-10.0.0.1",
		SubnetCIDR:       "10.0.0.0/24",
		ConfigMethod:     string(network.ConfigStatic),
		Value:            "10.0.0.1",
		ProviderSubnetID: "subnet-10.0.0.0/24",
		Origin:           "provider",
		IsShadow:         false,
		IsSecondary:      false,
	})
	model.AddIPAddress(description.IPAddressArgs{
		DeviceName: "eth0",
		MachineID:  "0",

		ProviderID:       "address-fd42:9102:88cb:dce3:216:3eff:fe59:a9dc",
		SubnetCIDR:       "fd42:9102:88cb:dce3::/64",
		ConfigMethod:     string(network.ConfigManual),
		Value:            "fd42:9102:88cb:dce3:216:3eff:fe59:a9dc",
		ProviderSubnetID: "subnet-fd42:9102:88cb:dce3::/64",
		Origin:           "provider",
		IsShadow:         true,
		IsSecondary:      true,
	})
	model.AddIPAddress(description.IPAddressArgs{
		DeviceName: "eth1",
		MachineID:  "0",

		ProviderID:       "address-198.0.0.1",
		SubnetCIDR:       "198.0.0.0/24",
		ConfigMethod:     string(network.ConfigDHCP),
		Value:            "198.0.0.1",
		ProviderSubnetID: "subnet-198.0.0.0/24",
		Origin:           "provider",
		IsShadow:         true,
		IsSecondary:      false,
	})
	model.AddIPAddress(description.IPAddressArgs{
		DeviceName: "eth0",
		MachineID:  "1",

		ConfigMethod: string(network.ConfigDHCP),
		Value:        "172.0.0.1",
		Origin:       "machine",
		IsShadow:     false,
		IsSecondary:  true,
	})

	model.AddIPAddress(description.IPAddressArgs{
		DeviceName: "eth0",
		MachineID:  "1",

		ConfigMethod: string(network.ConfigDHCP),
		Value:        "fd42:9102:88cb:dce3:216:3eff:dead:a9dc",
		Origin:       "machine",
		IsShadow:     false,
		IsSecondary:  true,
	})

	s.migrationService.EXPECT().ImportLinkLayerDevices(gomock.Any(), lldArgMatcher{c: c,
		expected: []internal.ImportLinkLayerDevice{{
			MachineID: "0",
			Name:      "eth0",
			Addresses: []internal.ImportIPAddress{{
				ProviderID:       ptr("address-10.0.0.1"),
				SubnetCIDR:       "10.0.0.0/24",
				ConfigType:       network.ConfigStatic,
				AddressValue:     "10.0.0.1/24",
				ProviderSubnetID: ptr("subnet-10.0.0.0/24"),
				Origin:           "provider",
				IsShadow:         false,
				IsSecondary:      false,
				// Resolved values
				Type:  network.IPv4Address,
				Scope: network.ScopeCloudLocal,
			}, {
				ProviderID:       ptr("address-fd42:9102:88cb:dce3:216:3eff:fe59:a9dc"),
				SubnetCIDR:       "fd42:9102:88cb:dce3::/64",
				ConfigType:       network.ConfigManual,
				AddressValue:     "fd42:9102:88cb:dce3:216:3eff:fe59:a9dc/64",
				ProviderSubnetID: ptr("subnet-fd42:9102:88cb:dce3::/64"),
				Origin:           "provider",
				IsShadow:         true,
				IsSecondary:      true,
				// Resolved values
				Type:  network.IPv6Address,
				Scope: network.ScopeCloudLocal,
			}},
		}, {
			MachineID: "0",
			Name:      "eth1",
			Addresses: []internal.ImportIPAddress{{
				ProviderID:       ptr("address-198.0.0.1"),
				SubnetCIDR:       "198.0.0.0/24",
				ConfigType:       network.ConfigDHCP,
				AddressValue:     "198.0.0.1/24",
				ProviderSubnetID: ptr("subnet-198.0.0.0/24"),
				Origin:           "provider",
				IsShadow:         true,
				IsSecondary:      false,
				// Resolved values
				Type:  network.IPv4Address,
				Scope: network.ScopePublic,
			}},
		}, {
			MachineID: "1",
			Name:      "eth0",
			Addresses: []internal.ImportIPAddress{{
				ConfigType:   network.ConfigDHCP,
				AddressValue: "172.0.0.1/32",
				Origin:       "machine",
				IsShadow:     false,
				IsSecondary:  true,
				// Resolved values
				Type:  network.IPv4Address,
				Scope: network.ScopePublic,
			}, {
				ConfigType:   network.ConfigDHCP,
				AddressValue: "fd42:9102:88cb:dce3:216:3eff:dead:a9dc/128",
				Origin:       "machine",
				IsShadow:     false,
				IsSecondary:  true,
				// Resolved values
				Type:  network.IPv6Address,
				Scope: network.ScopeCloudLocal,
			}}}}}).Return(nil)

	// Act
	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportLinkLayerDevicesWithAddressesErrorNoDevice(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	model := description.NewModel(description.ModelArgs{})

	model.AddLinkLayerDevice(description.LinkLayerDeviceArgs{
		Name:      "eth0",
		MachineID: "0",
	})
	model.AddIPAddress(description.IPAddressArgs{
		DeviceName: "eth1",
		MachineID:  "0",

		Value: "10.0.0.1",
	})

	// Act
	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorMatches, `address \"10.0.0.1\" for machine \"0\" on device \"eth1\" not found`)
}

func (s *importSuite) TestImportLinkLayerDevicesWithAddressesErrorInvalidConfigMethod(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	model := description.NewModel(description.ModelArgs{})

	model.AddLinkLayerDevice(description.LinkLayerDeviceArgs{
		Name:      "eth0",
		MachineID: "0",
	})
	model.AddIPAddress(description.IPAddressArgs{
		DeviceName: "eth0",
		MachineID:  "0",

		ConfigMethod: "not-valid",
		Value:        "10.0.0.1",
	})

	// Act
	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorMatches, `invalid address config type \"not-valid\" for address \"10.0.0.1\" of device \"eth0\" on machine \"0\"`)
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
	type key struct {
		machineID string
		name      string
	}
	noAddresses := func(in internal.ImportLinkLayerDevice) internal.ImportLinkLayerDevice {
		in.Addresses = nil
		return in
	}
	mapAddresses := func(in internal.ImportLinkLayerDevice) (key, []internal.ImportIPAddress) {
		return key{machineID: in.MachineID, name: in.Name}, in.Addresses
	}
	inputAddresses := transform.SliceToMap(input, mapAddresses)
	expectedAddresses := transform.SliceToMap(m.expected, mapAddresses)

	m.c.Check(slices.Collect(maps.Keys(inputAddresses)), tc.SameContents, slices.Collect(maps.Keys(expectedAddresses)))
	for k, in := range inputAddresses {
		// UUIDs are assigned in the code under test. Ensure they exist, then
		// remove it to enable SameContents checks over the other fields.
		in = transform.Slice(in, func(in internal.ImportIPAddress) internal.ImportIPAddress {
			m.c.Check(in.UUID, tc.Not(tc.Equals), "")
			in.UUID = ""
			return in
		})
		m.c.Check(in, tc.SameContents, expectedAddresses[k], tc.Commentf("for %+v", k))
	}

	// UUIDs are assigned in the code under test. Ensure they exist, then
	// remove it to enable SameContents checks over the other fields.
	input = transform.Slice(input, func(in internal.ImportLinkLayerDevice) internal.ImportLinkLayerDevice {
		m.c.Check(in.UUID, tc.Not(tc.Equals), "")
		in.UUID = ""
		return in
	})

	return m.c.Check(
		transform.Slice(input, noAddresses),
		tc.SameContents,
		transform.Slice(m.expected, noAddresses))
}

func (lldArgMatcher) String() string {
	return "matches args for ImportLinkLayerDevice"
}
