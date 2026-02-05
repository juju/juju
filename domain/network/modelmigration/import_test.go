// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"maps"
	"slices"
	"testing"

	"github.com/juju/collections/transform"
	"github.com/juju/description/v11"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/network/internal"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	migrationService *MockLinkLayerDevicesMigrationService
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.migrationService = NewMockLinkLayerDevicesMigrationService(ctrl)

	c.Cleanup(func() {
		s.migrationService = nil
	})

	return ctrl
}

func (s *importSuite) newImportOperation(c *tc.C) *importOperation {
	return &importOperation{
		migrationService: s.migrationService,
		logger:           loggertesting.WrapCheckLog(c),
	}
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

	s.migrationService.EXPECT().GetModelCloudType(c.Context()).Return("ec2", nil)
	args := []internal.ImportLinkLayerDevice{
		{
			IsAutoStart: dArgs.IsAutoStart,
			IsEnabled:   dArgs.IsUp,
			MTU:         ptr(int64(dArgs.MTU)),
			MachineID:   dArgs.MachineID,
			MACAddress:  ptr(dArgs.MACAddress),
			Name:        dArgs.Name,
			ProviderID:  ptr(dArgs.ProviderID),
			Type:        network.DeviceTypeEthernet,
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
		ConfigMethod:     string(corenetwork.ConfigStatic),
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
		ConfigMethod:     string(corenetwork.ConfigManual),
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
		ConfigMethod:     string(corenetwork.ConfigDHCP),
		Value:            "198.0.0.1",
		ProviderSubnetID: "subnet-198.0.0.0/24",
		Origin:           "provider",
		IsShadow:         true,
		IsSecondary:      false,
	})
	model.AddIPAddress(description.IPAddressArgs{
		DeviceName: "eth0",
		MachineID:  "1",

		ConfigMethod: string(corenetwork.ConfigDHCP),
		Value:        "172.0.0.1",
		Origin:       "machine",
		IsShadow:     false,
		IsSecondary:  true,
	})

	model.AddIPAddress(description.IPAddressArgs{
		DeviceName: "eth0",
		MachineID:  "1",

		ConfigMethod: string(corenetwork.ConfigDHCP),
		Value:        "fd42:9102:88cb:dce3:216:3eff:dead:a9dc",
		Origin:       "machine",
		IsShadow:     false,
		IsSecondary:  true,
	})

	s.migrationService.EXPECT().GetModelCloudType(c.Context()).Return("ec2", nil)
	s.migrationService.EXPECT().ImportLinkLayerDevices(gomock.Any(), lldArgMatcher{c: c,
		expected: []internal.ImportLinkLayerDevice{{
			MachineID: "0",
			Name:      "eth0",
			Addresses: []internal.ImportIPAddress{{
				ProviderID:       ptr("address-10.0.0.1"),
				SubnetCIDR:       "10.0.0.0/24",
				ConfigType:       corenetwork.ConfigStatic,
				AddressValue:     "10.0.0.1/24",
				ProviderSubnetID: ptr("subnet-10.0.0.0/24"),
				Origin:           "provider",
				IsShadow:         false,
				IsSecondary:      false,
				// Resolved values
				Type:  corenetwork.IPv4Address,
				Scope: corenetwork.ScopeCloudLocal,
			}, {
				ProviderID:       ptr("address-fd42:9102:88cb:dce3:216:3eff:fe59:a9dc"),
				SubnetCIDR:       "fd42:9102:88cb:dce3::/64",
				ConfigType:       corenetwork.ConfigManual,
				AddressValue:     "fd42:9102:88cb:dce3:216:3eff:fe59:a9dc/64",
				ProviderSubnetID: ptr("subnet-fd42:9102:88cb:dce3::/64"),
				Origin:           "provider",
				IsShadow:         true,
				IsSecondary:      true,
				// Resolved values
				Type:  corenetwork.IPv6Address,
				Scope: corenetwork.ScopeCloudLocal,
			}},
		}, {
			MachineID: "0",
			Name:      "eth1",
			Addresses: []internal.ImportIPAddress{{
				ProviderID:       ptr("address-198.0.0.1"),
				SubnetCIDR:       "198.0.0.0/24",
				ConfigType:       corenetwork.ConfigDHCP,
				AddressValue:     "198.0.0.1/24",
				ProviderSubnetID: ptr("subnet-198.0.0.0/24"),
				Origin:           "provider",
				IsShadow:         true,
				IsSecondary:      false,
				// Resolved values
				Type:  corenetwork.IPv4Address,
				Scope: corenetwork.ScopePublic,
			}},
		}, {
			MachineID: "1",
			Name:      "eth0",
			Addresses: []internal.ImportIPAddress{{
				ConfigType:   corenetwork.ConfigDHCP,
				AddressValue: "172.0.0.1/32",
				Origin:       "machine",
				IsShadow:     false,
				IsSecondary:  true,
				// Resolved values
				Type:  corenetwork.IPv4Address,
				Scope: corenetwork.ScopePublic,
			}, {
				ConfigType:   corenetwork.ConfigDHCP,
				AddressValue: "fd42:9102:88cb:dce3:216:3eff:dead:a9dc/128",
				Origin:       "machine",
				IsShadow:     false,
				IsSecondary:  true,
				// Resolved values
				Type:  corenetwork.IPv6Address,
				Scope: corenetwork.ScopeCloudLocal,
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

	s.migrationService.EXPECT().GetModelCloudType(c.Context()).Return("lxd", nil)

	// Act
	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorMatches, `address \"10.0.0.1\" for machine \"0\" on device \"eth1\" not found`)
}

func (s *importSuite) TestImportLinkLayerDevicesSkipsFanAddresses(c *tc.C) {
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

		ConfigMethod: string(corenetwork.ConfigStatic),
		Value:        "240.0.0.1",
	})
	model.AddIPAddress(description.IPAddressArgs{
		DeviceName:       "eth0",
		MachineID:        "0",
		ProviderID:       "address-10.0.0.1",
		SubnetCIDR:       "10.0.0.0/24",
		ConfigMethod:     string(corenetwork.ConfigStatic),
		Value:            "10.0.0.1",
		ProviderSubnetID: "subnet--10.0.0.0/24",
		Origin:           "provider",
	})

	s.migrationService.EXPECT().GetModelCloudType(c.Context()).Return("lxd", nil)
	s.migrationService.EXPECT().ImportLinkLayerDevices(gomock.Any(), lldArgMatcher{c: c,
		expected: []internal.ImportLinkLayerDevice{{
			MachineID: "0",
			Name:      "eth0",
			Addresses: []internal.ImportIPAddress{{
				ProviderID:   ptr("address-10.0.0.1"),
				SubnetCIDR:   "10.0.0.0/24",
				ConfigType:   corenetwork.ConfigStatic,
				AddressValue: "10.0.0.1/24",
				Origin:       "provider",
				IsShadow:     false,
				IsSecondary:  false,
				// Resolved values
				Type:  corenetwork.IPv4Address,
				Scope: corenetwork.ScopeCloudLocal,
			}},
		}}}).Return(nil)

	// Act
	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
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

	s.migrationService.EXPECT().GetModelCloudType(c.Context()).Return("ec2", nil)

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

	s.migrationService.EXPECT().GetModelCloudType(c.Context()).Return("ec2", nil)
	args := []internal.ImportLinkLayerDevice{
		{
			IsAutoStart: dArgs.IsAutoStart,
			IsEnabled:   dArgs.IsUp,
			MachineID:   dArgs.MachineID,
			Name:        dArgs.Name,
			Type:        network.DeviceTypeEthernet,
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

// ptr returns a reference to a copied value of type T.
func ptr[T any](i T) *T {
	return &i
}
