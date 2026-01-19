// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/juju/collections/transform"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/network"
	networktesting "github.com/juju/juju/core/network/testing"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type migrationSuite struct {
	testhelpers.IsolationSuite

	st *MockState
}

func (s *migrationSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.st = NewMockState(ctrl)
	c.Cleanup(func() { s.st = nil })
	return ctrl
}

func TestMigrationSuite(t *testing.T) {
	tc.Run(t, &migrationSuite{})
}

func (s *migrationSuite) TestImportLinkLayerDevices(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	netNodeUUID := uuid.MustNewUUID().String()
	nameMap := map[string]string{
		"88": netNodeUUID,
	}
	args := []internal.ImportLinkLayerDevice{
		{MachineID: "88"},
	}
	expectedArgs := make([]internal.ImportLinkLayerDevice, len(args))
	copy(expectedArgs, args)
	expectedArgs[0].NetNodeUUID = netNodeUUID
	s.st.EXPECT().AllMachinesAndNetNodes(gomock.Any()).Return(nameMap, nil)
	s.st.EXPECT().GetAllSubnets(gomock.Any()).Return(nil, nil)
	s.st.EXPECT().ImportLinkLayerDevices(gomock.Any(), expectedArgs).Return(nil)

	// Act
	err := s.migrationService(c).ImportLinkLayerDevices(c.Context(), args)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationSuite) TestImportLinkLayerDevicesSubnetError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	nameMap := map[string]string{}
	args := []internal.ImportLinkLayerDevice{{}}

	expectedError := errors.New("subnet error")
	s.st.EXPECT().AllMachinesAndNetNodes(gomock.Any()).Return(nameMap, nil)
	s.st.EXPECT().GetAllSubnets(gomock.Any()).Return(nil, expectedError)

	// Act
	err := s.migrationService(c).ImportLinkLayerDevices(c.Context(), args)

	// Assert: error about getting all subnets
	c.Assert(err, tc.ErrorMatches, `getting all subnets: subnet error`)
}

func (s *migrationSuite) TestImportLinkLayerDevicesSubnetWithProvider(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	netNodeUUID := uuid.MustNewUUID().String()
	nameMap := map[string]string{
		"88": netNodeUUID,
	}

	providerSubnetID := "provider-subnet-1"
	subnetUUID := uuid.MustNewUUID().String()
	subnets := network.SubnetInfos{{
		ID:         network.Id(subnetUUID),
		ProviderId: network.Id(providerSubnetID),
	}}

	args := []internal.ImportLinkLayerDevice{
		{
			MachineID: "88",
			Name:      "eth0",
			Addresses: []internal.ImportIPAddress{
				{
					ProviderSubnetID: &providerSubnetID,
				},
			},
		},
	}

	expectedArgs := make([]internal.ImportLinkLayerDevice, len(args))
	copy(expectedArgs, args)
	expectedArgs[0].NetNodeUUID = netNodeUUID
	expectedArgs[0].Addresses[0].SubnetUUID = subnetUUID

	s.st.EXPECT().AllMachinesAndNetNodes(gomock.Any()).Return(nameMap, nil)
	s.st.EXPECT().GetAllSubnets(gomock.Any()).Return(subnets, nil)
	s.st.EXPECT().ImportLinkLayerDevices(gomock.Any(), expectedArgs).Return(nil)

	// Act
	err := s.migrationService(c).ImportLinkLayerDevices(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationSuite) TestImportLinkLayerDevicesSubnetWithProviderError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	netNodeUUID := uuid.MustNewUUID().String()
	nameMap := map[string]string{
		"88": netNodeUUID,
	}

	// Create a subnet with a different provider ID than the one in the address
	providerSubnetID := "provider-subnet-1"
	unknownProviderSubnetID := "unknown-provider-subnet"
	subnetUUID := uuid.MustNewUUID().String()
	subnets := network.SubnetInfos{{
		ID:         network.Id(subnetUUID),
		ProviderId: network.Id(providerSubnetID),
	}}

	args := []internal.ImportLinkLayerDevice{
		{
			MachineID: "88",
			Name:      "eth0",
			Addresses: []internal.ImportIPAddress{
				{
					ProviderSubnetID: &unknownProviderSubnetID, // Provider subnet ID that doesn't match any subnet
				},
			},
		},
	}

	s.st.EXPECT().AllMachinesAndNetNodes(gomock.Any()).Return(nameMap, nil)
	s.st.EXPECT().GetAllSubnets(gomock.Any()).Return(subnets, nil)

	// Act
	err := s.migrationService(c).ImportLinkLayerDevices(c.Context(), args)

	// Assert: error about no subnet found for provider subnet ID
	c.Assert(err, tc.ErrorMatches, `converting device "eth0" on machine "88":.*converting address.*:.*no subnet found for provider subnet ID "unknown-provider-subnet"`)
}

func (s *migrationSuite) TestImportLinkLayerDevicesSubnetWithoutProvider(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	netNodeUUID := uuid.MustNewUUID().String()
	nameMap := map[string]string{
		"88": netNodeUUID,
	}
	subnetUUID := uuid.MustNewUUID().String()
	subnets := network.SubnetInfos{{
		ID:   network.Id(subnetUUID),
		CIDR: "192.0.2.0/24",
	}}

	args := []internal.ImportLinkLayerDevice{
		{
			MachineID: "88",
			Name:      "eth0",
			Addresses: []internal.ImportIPAddress{
				{
					SubnetCIDR: "192.0.2.0/31",
				},
			},
		},
	}

	expectedArgs := make([]internal.ImportLinkLayerDevice, len(args))
	copy(expectedArgs, args)
	expectedArgs[0].NetNodeUUID = netNodeUUID
	expectedArgs[0].Addresses[0].SubnetUUID = subnetUUID

	s.st.EXPECT().AllMachinesAndNetNodes(gomock.Any()).Return(nameMap, nil)
	s.st.EXPECT().GetAllSubnets(gomock.Any()).Return(subnets, nil)
	s.st.EXPECT().ImportLinkLayerDevices(gomock.Any(), expectedArgs).Return(nil)

	// Act
	err := s.migrationService(c).ImportLinkLayerDevices(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationSuite) TestImportLinkLayerDevicesSubnetWithoutProviderNoSubnet(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	netNodeUUID := uuid.MustNewUUID().String()
	nameMap := map[string]string{
		"88": netNodeUUID,
	}

	// Create a subnet with a different CIDR than the one in the address
	subnetUUID := uuid.MustNewUUID().String()
	subnetInfo := network.SubnetInfo{
		ID:   network.Id(subnetUUID),
		CIDR: "198.51.100.0/24", // Different CIDR than the one in the address
	}
	subnets := network.SubnetInfos{subnetInfo}

	args := []internal.ImportLinkLayerDevice{
		{
			MachineID: "88",
			Name:      "eth0",
			Addresses: []internal.ImportIPAddress{
				{
					AddressValue: "192.0.2.10",
					SubnetCIDR:   "192.0.2.0/24", // No matching subnet for this CIDR
				},
			},
		},
	}

	s.st.EXPECT().AllMachinesAndNetNodes(gomock.Any()).Return(nameMap, nil)
	s.st.EXPECT().GetAllSubnets(gomock.Any()).Return(subnets, nil)

	// Act
	err := s.migrationService(c).ImportLinkLayerDevices(c.Context(), args)

	// Assert: error about no subnet found for CIDR
	c.Assert(err, tc.ErrorMatches,
		`converting device "eth0" on machine "88":.*converting address "192.0.2.10":.*no subnet found`)
}

func (s *migrationSuite) TestImportLinkLayerDevicesSubnetWithoutProviderTooMuchSubnet(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	netNodeUUID := uuid.MustNewUUID().String()
	nameMap := map[string]string{
		"88": netNodeUUID,
	}

	// Create multiple subnets with the same CIDR
	subnetUUID1 := uuid.MustNewUUID().String()
	subnetUUID2 := uuid.MustNewUUID().String()
	subnetInfo1 := network.SubnetInfo{
		ID:   network.Id(subnetUUID1),
		CIDR: "192.0.2.0/24",
	}
	subnetInfo2 := network.SubnetInfo{
		ID:   network.Id(subnetUUID2),
		CIDR: "192.0.2.0/24", // Same CIDR as subnetInfo1
	}
	subnets := network.SubnetInfos{subnetInfo1, subnetInfo2}

	args := []internal.ImportLinkLayerDevice{
		{
			MachineID: "88",
			Name:      "eth0",
			Addresses: []internal.ImportIPAddress{
				{
					AddressValue: "192.0.2.10",
					SubnetCIDR:   "192.0.2.0/24", // Matches multiple subnets
				},
			},
		},
	}

	s.st.EXPECT().AllMachinesAndNetNodes(gomock.Any()).Return(nameMap, nil)
	s.st.EXPECT().GetAllSubnets(gomock.Any()).Return(subnets, nil)

	// Act
	err := s.migrationService(c).ImportLinkLayerDevices(c.Context(), args)

	// Assert: error about multiple subnets found for CIDR
	c.Assert(err, tc.ErrorMatches,
		`converting device "eth0" on machine "88":.*converting address "192.0.2.10":.*multiple subnets found:.*`)
}

func (s *migrationSuite) TestImportLinkLayerDevicesMachines(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	s.st.EXPECT().AllMachinesAndNetNodes(gomock.Any()).Return(nil, errors.New("boom"))
	args := []internal.ImportLinkLayerDevice{
		{
			MachineID: "88",
		},
	}

	// Act
	err := s.migrationService(c).ImportLinkLayerDevices(c.Context(), args)

	// Assert: error from AllMachinesAndNetNodes returned.
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *migrationSuite) TestImportLinkLayerDevicesLoopbackAddressesNoSubnet(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	netNodeUUID := uuid.MustNewUUID().String()
	nameMap := map[string]string{
		"88": netNodeUUID,
	}

	subnetUUID := uuid.MustNewUUID().String()
	subnets := network.SubnetInfos{{
		ID:   network.Id(subnetUUID),
		CIDR: "192.0.2.0/24",
	}}

	args := []internal.ImportLinkLayerDevice{
		{
			MachineID: "88",
			Name:      "eth0",
			Addresses: []internal.ImportIPAddress{
				{
					AddressValue: "127.0.0.1",
					SubnetCIDR:   "127.0.0.0/8",
					ConfigType:   network.ConfigLoopback,
				},
				{
					AddressValue: "192.0.2.10",
					SubnetCIDR:   "192.0.2.0/24",
					ConfigType:   network.ConfigStatic,
				},
			},
		},
	}

	expectedArgs := make([]internal.ImportLinkLayerDevice, len(args))
	copy(expectedArgs, args)
	expectedArgs[0].NetNodeUUID = netNodeUUID
	// Loopback address is included but not transformed (no SubnetUUID set)
	// Non-loopback address is transformed with SubnetUUID.
	expectedArgs[0].Addresses = []internal.ImportIPAddress{
		{
			AddressValue: "127.0.0.1",
			SubnetCIDR:   "127.0.0.0/8",
			ConfigType:   network.ConfigLoopback,
			// SubnetUUID is empty for loopback.
		},
		{
			AddressValue: "192.0.2.10",
			SubnetCIDR:   "192.0.2.0/24",
			ConfigType:   network.ConfigStatic,
			SubnetUUID:   subnetUUID,
		},
	}

	s.st.EXPECT().AllMachinesAndNetNodes(gomock.Any()).Return(nameMap, nil)
	s.st.EXPECT().GetAllSubnets(gomock.Any()).Return(subnets, nil)
	s.st.EXPECT().ImportLinkLayerDevices(gomock.Any(), expectedArgs).Return(nil)

	// Act
	err := s.migrationService(c).ImportLinkLayerDevices(c.Context(), args)

	// Assert: loopback addresses are included but have no subnet UUID.
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationSuite) TestImportLinkLayerDevicesOnlyLoopbackAddresses(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	netNodeUUID := uuid.MustNewUUID().String()
	nameMap := map[string]string{
		"88": netNodeUUID,
	}

	args := []internal.ImportLinkLayerDevice{
		{
			MachineID: "88",
			Name:      "lo",
			Addresses: []internal.ImportIPAddress{
				{
					AddressValue: "127.0.0.1",
					SubnetCIDR:   "127.0.0.0/8",
					ConfigType:   network.ConfigLoopback,
				},
				{
					AddressValue: "::1",
					SubnetCIDR:   "::1/128",
					ConfigType:   network.ConfigLoopback,
				},
			},
		},
	}

	expectedArgs := make([]internal.ImportLinkLayerDevice, len(args))
	copy(expectedArgs, args)
	expectedArgs[0].NetNodeUUID = netNodeUUID
	// All addresses are loopback, they're included but not transformed.
	expectedArgs[0].Addresses = []internal.ImportIPAddress{
		{
			AddressValue: "127.0.0.1",
			SubnetCIDR:   "127.0.0.0/8",
			ConfigType:   network.ConfigLoopback,
			// SubnetUUID is empty for loopback.
		},
		{
			AddressValue: "::1",
			SubnetCIDR:   "::1/128",
			ConfigType:   network.ConfigLoopback,
			// SubnetUUID is empty for loopback.
		},
	}

	s.st.EXPECT().AllMachinesAndNetNodes(gomock.Any()).Return(nameMap, nil)
	s.st.EXPECT().GetAllSubnets(gomock.Any()).Return(nil, nil)
	s.st.EXPECT().ImportLinkLayerDevices(gomock.Any(), expectedArgs).Return(nil)

	// Act
	err := s.migrationService(c).ImportLinkLayerDevices(c.Context(), args)

	// Assert: device is imported with loopback addresses but no subnet UUIDs.
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationSuite) TestImportLinkLayerDevicesNoContent(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	// Act
	err := s.migrationService(c).ImportLinkLayerDevices(c.Context(), []internal.ImportLinkLayerDevice{})

	// Assert: no failure if no data provided.
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationSuite) migrationService(c *tc.C) *MigrationService {
	return NewMigrationService(s.st, loggertesting.WrapCheckLog(c))
}

func (s *migrationSuite) TestImportCloudServicesGetAllSpacesError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	s.st.EXPECT().GetAllSpaces(gomock.Any()).Return(nil, errors.New("spaces error"))
	// No calls to another state function.

	// Act
	err := s.migrationService(c).ImportCloudServices(c.Context(), []internal.ImportCloudService{})

	// Assert: the error from GetAllSubnets is passed through to the caller
	c.Assert(err, tc.ErrorMatches, "converting services: getting all spaces: spaces error")
}

func (s *migrationSuite) TestImportCloudServicesCreateCloudServicesError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	s.st.EXPECT().GetAllSpaces(gomock.Any()).Return(network.SpaceInfos{}, nil)
	s.st.EXPECT().CreateCloudServices(gomock.Any(), gomock.Any()).Return(errors.New("create services error"))
	// No try to create LLD if creating cloud services fails

	// Act
	err := s.migrationService(c).ImportCloudServices(c.Context(), []internal.ImportCloudService{})

	// Assert: the error from CreateCloudServices is passed through to the caller
	c.Assert(err, tc.ErrorMatches, "creating cloud services: create services error")
}

func (s *migrationSuite) TestImportCloudServicesImportLinkLayerDevicesError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	s.st.EXPECT().GetAllSpaces(gomock.Any()).Return(network.SpaceInfos{}, nil)
	s.st.EXPECT().CreateCloudServices(gomock.Any(), gomock.Any()).Return(nil)
	s.st.EXPECT().ImportLinkLayerDevices(gomock.Any(), gomock.Any()).Return(errors.New("import devices error"))

	// Act
	err := s.migrationService(c).ImportCloudServices(c.Context(), []internal.ImportCloudService{})

	// Assert: the error from ImportLinkLayerDevices is passed through to the caller
	c.Assert(err, tc.ErrorMatches, "importing link layer devices: import devices error")
}

func (s *migrationSuite) TestImportCloudServicesErrorNoSpaceForAddress(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	// Create test data
	services := []internal.ImportCloudService{
		{
			Addresses: []internal.ImportCloudServiceAddress{
				{
					UUID:    "addr-uuid",
					SpaceID: "unknown-space-id",
				},
			},
		},
	}

	s.st.EXPECT().GetAllSpaces(gomock.Any()).Return(network.SpaceInfos{}, nil)

	// Act
	err := s.migrationService(c).ImportCloudServices(c.Context(), services)

	// Assert: no error is returned
	c.Assert(err, tc.ErrorMatches,
		`converting services:.*converting address.*:.*unknown space ID "unknown-space-id"`)
}

func (s *migrationSuite) TestImportCloudServicesErrorGetSubnet(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	// Create test data
	services := []internal.ImportCloudService{
		{
			Addresses: []internal.ImportCloudServiceAddress{
				{
					UUID:    "addr-uuid",
					SpaceID: "space-id",
					Value:   "boom",
				},
			},
		},
	}

	s.st.EXPECT().GetAllSpaces(gomock.Any()).Return(network.SpaceInfos{{
		ID:   "space-id",
		Name: "my-space",
	}}, nil)

	// Act
	err := s.migrationService(c).ImportCloudServices(c.Context(), services)

	// Assert: no error is returned
	c.Assert(err, tc.ErrorMatches,
		`converting services:.*converting address "boom".*:.*getting subnets: "boom" as IP address not valid`)
}

func (s *migrationSuite) TestImportCloudServicesErrorGetSubnetNoSubnet(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	// Create test data
	services := []internal.ImportCloudService{
		{
			Addresses: []internal.ImportCloudServiceAddress{
				{
					UUID:    "addr-uuid",
					SpaceID: "space-id",
					Value:   "192.0.2.1",
				},
			},
		},
	}

	s.st.EXPECT().GetAllSpaces(gomock.Any()).Return(network.SpaceInfos{{
		ID:      "space-id",
		Name:    "my-space",
		Subnets: nil,
	}}, nil)

	// Act
	err := s.migrationService(c).ImportCloudServices(c.Context(), services)

	// Assert: no error is returned
	c.Assert(err, tc.ErrorMatches,
		`converting services:.*converting address "192.0.2.1".*:.*no subnet found`)
}

func (s *migrationSuite) TestImportCloudServicesErrorGetSubnetSeveralSubnets(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	// Create test data
	services := []internal.ImportCloudService{
		{
			Addresses: []internal.ImportCloudServiceAddress{
				{
					UUID:    "addr-uuid",
					SpaceID: "space-id",
					Value:   "192.0.2.1",
				},
			},
		},
	}

	s.st.EXPECT().GetAllSpaces(gomock.Any()).Return(network.SpaceInfos{{
		ID:   "space-id",
		Name: "my-space",
		Subnets: network.SubnetInfos{
			{
				ID:   "subnet-id-1",
				CIDR: "192.0.2.0/24",
			},
			{
				ID:   "subnet-id-2",
				CIDR: "192.0.2.1/31",
			}},
	}}, nil)

	// Act
	err := s.migrationService(c).ImportCloudServices(c.Context(), services)

	// Assert: no error is returned
	c.Assert(err, tc.ErrorMatches,
		`converting services:.*converting address "192.0.2.1".*:.*multiple subnets found.*`)
}

func (s *migrationSuite) TestImportCloudServicesSuccess(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	// Create test data
	services := []internal.ImportCloudService{
		{
			UUID:            "service-uuid",
			DeviceUUID:      "device-uuid",
			NetNodeUUID:     "node-uuid",
			ApplicationName: "test-app",
			ProviderID:      "provider-id",
			Addresses: []internal.ImportCloudServiceAddress{
				{
					UUID:    "addr-uuid",
					Value:   "192.0.2.1",
					Type:    "ipv4",
					Scope:   "local-cloud",
					Origin:  "provider",
					SpaceID: "space-id",
				},
			},
		},
	}

	// Create subnet info for the test
	subnetUUID := uuid.MustNewUUID().String()
	subnets := network.SubnetInfos{
		{
			ID:   network.Id(subnetUUID),
			CIDR: "192.0.2.0/24",
		},
	}

	s.st.EXPECT().GetAllSpaces(gomock.Any()).Return(network.SpaceInfos{{
		ID:      "space-id",
		Subnets: subnets,
	}}, nil)

	// Capture the arguments passed to CreateCloudServices
	s.st.EXPECT().CreateCloudServices(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, svcs []internal.ImportCloudService) error {
			c.Assert(svcs, tc.DeepEquals, services)
			return nil
		})

	// Capture the arguments passed to ImportLinkLayerDevices
	s.st.EXPECT().ImportLinkLayerDevices(gomock.Any(), cloudServiceLLDMatcher{
		c:    c,
		from: services,
		subnetUUIDs: map[string]string{
			"addr-uuid": subnetUUID,
		},
	}).Return(nil)

	// Act
	err := s.migrationService(c).ImportCloudServices(c.Context(), services)

	// Assert: no error is returned
	c.Assert(err, tc.IsNil)
}

type cloudServiceLLDMatcher struct {
	c           *tc.C
	from        []internal.ImportCloudService
	subnetUUIDs map[string]string
}

func (m cloudServiceLLDMatcher) Matches(x interface{}) bool {
	input, ok := x.([]internal.ImportLinkLayerDevice)
	if !ok {
		return false
	}
	inputByNodeUUID := transform.SliceToMap(input,
		func(in internal.ImportLinkLayerDevice) (string, internal.ImportLinkLayerDevice) {
			return in.NetNodeUUID, in
		})
	expectedByNodeUUID := transform.SliceToMap(m.from,
		func(f internal.ImportCloudService) (string, internal.ImportCloudService) {
			return f.NetNodeUUID, f
		})

	if !m.c.Check(slices.Collect(maps.Keys(inputByNodeUUID)), tc.SameContents,
		slices.Collect(maps.Keys(expectedByNodeUUID)),
		tc.Commentf("mistmatch between inserted cloud services and imported one")) {
		return false
	}

	result := true
	for k, in := range inputByNodeUUID {
		expected := expectedByNodeUUID[k]

		inputAddrByUUID := transform.SliceToMap(in.Addresses, func(f internal.ImportIPAddress) (string, internal.ImportIPAddress) {
			return f.UUID, f
		})
		expectedAddrByUUID := transform.SliceToMap(expected.Addresses, func(f internal.ImportCloudServiceAddress) (string,
			internal.ImportCloudServiceAddress) {
			return f.UUID, f
		})

		in.Addresses = nil
		result = result && m.c.Check(in, tc.DeepEquals, internal.ImportLinkLayerDevice{
			UUID:            expected.DeviceUUID,
			IsAutoStart:     true,
			IsEnabled:       true,
			NetNodeUUID:     expected.NetNodeUUID,
			Name:            fmt.Sprintf("placeholder for %q cloud service", expected.ApplicationName),
			Type:            network.UnknownDevice,
			VirtualPortType: network.NonVirtualPort,
		})
		if !m.c.Check(slices.Collect(maps.Keys(inputAddrByUUID)), tc.SameContents,
			slices.Collect(maps.Keys(expectedAddrByUUID)),
			tc.Commentf("mistmatch between inserted cloud services addresses and imported one")) {
			continue
		}
		for k, inAddr := range inputAddrByUUID {
			expectedAddr := expectedAddrByUUID[k]
			result = result && m.c.Check(inAddr, tc.DeepEquals, internal.ImportIPAddress{
				UUID:         expectedAddr.UUID,
				Type:         network.AddressType(expectedAddr.Type),
				Scope:        network.Scope(expectedAddr.Scope),
				AddressValue: expectedAddr.Value,
				ConfigType:   network.ConfigStatic,
				Origin:       network.Origin(expectedAddr.Origin),
				SubnetUUID:   m.subnetUUIDs[expectedAddr.UUID],
			})
		}
	}
	return result
}

func (cloudServiceLLDMatcher) String() string {
	return "matches args for ImportLinkLayerDevices"
}

func (s *migrationSuite) TestEnsureAlphaSpaceAndSubnets(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().EnsureAlphaSpaceAndSubnets(gomock.Any()).Return(nil)

	err := s.migrationService(c).EnsureAlphaSpaceAndSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationSuite) TestAddSpaceInvalidNameEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Make sure no calls to state are done
	s.st.EXPECT().AddSpace(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	_, err := s.migrationService(c).AddSpace(
		c.Context(),
		network.SpaceInfo{})
	c.Assert(err, tc.ErrorIs, networkerrors.SpaceNameNotValid)
}

func (s *migrationSuite) TestAddSpaceInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Make sure no calls to state are done
	s.st.EXPECT().AddSpace(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	_, err := s.migrationService(c).AddSpace(
		c.Context(),
		network.SpaceInfo{
			Name:       "-bad name-",
			ProviderId: "provider-id",
		})
	c.Assert(err, tc.ErrorIs, networkerrors.SpaceNameNotValid)
}

func (s *migrationSuite) TestAddSpaceErrorAdding(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().AddSpace(gomock.Any(), gomock.Any(), network.SpaceName("0"), network.Id("provider-id"), []string{"0"}).
		Return(errors.Errorf("updating subnet %q using space uuid \"space0\"", "0"))

	_, err := s.migrationService(c).AddSpace(
		c.Context(),
		network.SpaceInfo{
			Name:       "0",
			ProviderId: "provider-id",
			Subnets: network.SubnetInfos{
				{
					ID: network.Id("0"),
				},
			},
		})
	c.Assert(err, tc.ErrorMatches, "updating subnet \"0\" using space uuid \"space0\"")
}

func (s *migrationSuite) TestAddSpace(c *tc.C) {
	defer s.setupMocks(c).Finish()

	var expectedUUID network.SpaceUUID
	// Verify that the passed UUID is also returned.
	s.st.EXPECT().AddSpace(gomock.Any(), gomock.Any(), network.SpaceName("space0"), network.Id("provider-id"), []string{}).
		Do(
			func(
				ctx context.Context,
				uuid network.SpaceUUID,
				name network.SpaceName,
				providerID network.Id,
				subnetIDs []string,
			) error {
				expectedUUID = uuid
				return nil
			})

	returnedUUID, err := s.migrationService(c).AddSpace(
		c.Context(),
		network.SpaceInfo{
			Name:       "space0",
			ProviderId: "provider-id",
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(returnedUUID, tc.Not(tc.Equals), "")
	c.Check(returnedUUID, tc.Equals, expectedUUID)
}

func (s *migrationSuite) TestGetSpaceByID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetSpace(gomock.Any(), network.AlphaSpaceId)
	_, err := s.migrationService(c).GetSpace(c.Context(), network.AlphaSpaceId)
	c.Assert(err, tc.ErrorIsNil)
}

// TestGetSpaceByIDNotFound checks that if we try to call Service.Space on
// a space that doesn't exist, an error is returned matching
// networkerrors.SpaceNotFound.
func (s *migrationSuite) TestGetSpaceByIDNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	spUUID := networktesting.GenSpaceUUID(c)
	s.st.EXPECT().GetSpace(gomock.Any(), spUUID).
		Return(nil, errors.Errorf("space %q: %w", spUUID, networkerrors.SpaceNotFound))
	_, err := s.migrationService(c).GetSpace(c.Context(), spUUID)
	c.Assert(err, tc.ErrorIs, networkerrors.SpaceNotFound)
}

func (s *migrationSuite) TestFailAddSubnet(c *tc.C) {
	defer s.setupMocks(c).Finish()

	subnetInfo := network.SubnetInfo{
		CIDR:              "192.168.0.0/20",
		ProviderId:        "provider-id-0",
		ProviderNetworkId: "provider-network-id-0",
		AvailabilityZones: []string{"az0"},
	}

	// Verify that the passed subnetInfo matches and return an error.
	s.st.EXPECT().AddSubnet(gomock.Any(), gomock.Any()).
		DoAndReturn(
			func(
				ctx context.Context,
				subnet network.SubnetInfo,
			) error {
				c.Assert(subnet.CIDR, tc.Equals, subnetInfo.CIDR)
				c.Assert(subnet.ProviderId, tc.Equals, subnetInfo.ProviderId)
				c.Assert(subnet.ProviderNetworkId, tc.Equals, subnetInfo.ProviderNetworkId)
				c.Assert(subnet.AvailabilityZones, tc.SameContents, subnetInfo.AvailabilityZones)
				return errors.New("boom")
			})

	_, err := s.migrationService(c).AddSubnet(c.Context(), subnetInfo)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *migrationSuite) TestAddSubnet(c *tc.C) {
	defer s.setupMocks(c).Finish()

	subnetInfo := network.SubnetInfo{
		CIDR:              "192.168.0.0/20",
		ProviderId:        "provider-id-0",
		ProviderNetworkId: "provider-network-id-0",
		AvailabilityZones: []string{"az0"},
	}

	var expectedUUID network.Id
	// Verify that the passed subnetInfo matches and don't return an error.
	s.st.EXPECT().AddSubnet(gomock.Any(), gomock.Any()).
		Do(
			func(
				ctx context.Context,
				subnet network.SubnetInfo,
			) error {
				c.Assert(subnet.CIDR, tc.Equals, subnetInfo.CIDR)
				c.Assert(subnet.ProviderId, tc.Equals, subnetInfo.ProviderId)
				c.Assert(subnet.ProviderNetworkId, tc.Equals, subnetInfo.ProviderNetworkId)
				c.Assert(subnet.AvailabilityZones, tc.SameContents, subnetInfo.AvailabilityZones)
				expectedUUID = subnet.ID
				return nil
			})

	returnedUUID, err := s.migrationService(c).AddSubnet(c.Context(), subnetInfo)
	c.Assert(err, tc.ErrorIsNil)
	// Verify that the passed UUID is also returned.
	c.Assert(returnedUUID, tc.Equals, expectedUUID)
}
