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

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network"
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

func (s *migrationSuite) service(c *tc.C) *Service {
	return NewService(s.st, loggertesting.WrapCheckLog(c))
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
	subnets := corenetwork.SubnetInfos{{
		ID:         corenetwork.Id(subnetUUID),
		ProviderId: corenetwork.Id(providerSubnetID),
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
	subnets := corenetwork.SubnetInfos{{
		ID:         corenetwork.Id(subnetUUID),
		ProviderId: corenetwork.Id(providerSubnetID),
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
	subnets := corenetwork.SubnetInfos{{
		ID:   corenetwork.Id(subnetUUID),
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

func (s *migrationSuite) TestImportLinkLayerDevicesSubnetWithoutProviderNoSubnetSlash32(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	netNodeUUID := uuid.MustNewUUID().String()
	nameMap := map[string]string{
		"88": netNodeUUID,
	}

	s.st.EXPECT().AllMachinesAndNetNodes(gomock.Any()).Return(nameMap, nil)
	s.st.EXPECT().GetAllSubnets(gomock.Any()).Return(nil, nil)

	// Expect a /32 subnet to be created.
	subnetInfo := corenetwork.SubnetInfo{
		CIDR: "192.0.2.0/32",
	}
	matcher := &spaceInfoAsArgMatcher{
		c:        c,
		expected: subnetInfo,
	}
	s.st.EXPECT().AddSubnet(gomock.Any(), matcher).Return(nil)

	args := []internal.ImportLinkLayerDevice{
		{
			MachineID: "88",
			Name:      "cillium_host",
			Addresses: []internal.ImportIPAddress{
				{
					AddressValue: "192.0.2.10",
					SubnetCIDR:   "192.0.2.0/32", // No matching subnet for this CIDR
				},
			},
		},
	}

	expectedArgs := make([]internal.ImportLinkLayerDevice, len(args))
	copy(expectedArgs, args)
	expectedArgs[0].NetNodeUUID = netNodeUUID
	matcherTwo := &importLinkLayerDeviceArgMatcher{
		c:        c,
		expected: expectedArgs,
	}
	s.st.EXPECT().ImportLinkLayerDevices(gomock.Any(), matcherTwo).Return(nil)

	// Act
	err := s.migrationService(c).ImportLinkLayerDevices(c.Context(), args)

	// Assert: no error
	c.Assert(err, tc.ErrorIsNil)
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
	subnetInfo1 := corenetwork.SubnetInfo{
		ID:   corenetwork.Id(subnetUUID1),
		CIDR: "192.0.2.0/24",
	}
	subnetInfo2 := corenetwork.SubnetInfo{
		ID:   corenetwork.Id(subnetUUID2),
		CIDR: "192.0.2.0/24", // Same CIDR as subnetInfo1
	}
	subnets := corenetwork.SubnetInfos{subnetInfo1, subnetInfo2}

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
	subnets := corenetwork.SubnetInfos{{
		ID:   corenetwork.Id(subnetUUID),
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
					ConfigType:   corenetwork.ConfigLoopback,
				},
				{
					AddressValue: "192.0.2.10",
					SubnetCIDR:   "192.0.2.0/24",
					ConfigType:   corenetwork.ConfigStatic,
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
			ConfigType:   corenetwork.ConfigLoopback,
			// SubnetUUID is empty for loopback.
		},
		{
			AddressValue: "192.0.2.10",
			SubnetCIDR:   "192.0.2.0/24",
			ConfigType:   corenetwork.ConfigStatic,
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
					ConfigType:   corenetwork.ConfigLoopback,
				},
				{
					AddressValue: "::1",
					SubnetCIDR:   "::1/128",
					ConfigType:   corenetwork.ConfigLoopback,
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
			ConfigType:   corenetwork.ConfigLoopback,
			// SubnetUUID is empty for loopback.
		},
		{
			AddressValue: "::1",
			SubnetCIDR:   "::1/128",
			ConfigType:   corenetwork.ConfigLoopback,
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

func (s *migrationSuite) TestImportCloudServicesGetAllSubnetsError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	s.st.EXPECT().GetAllSubnets(gomock.Any()).Return(nil, errors.New("subnets error"))
	// No calls to another state function.

	// Act
	err := s.migrationService(c).ImportCloudServices(c.Context(), []internal.ImportCloudService{})

	// Assert: the error from GetAllSubnets is passed through to the caller
	c.Assert(err, tc.ErrorMatches, ".*subnets error")
}

func (s *migrationSuite) TestImportCloudServicesCreateCloudServicesError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().GetAllSubnets(gomock.Any()).Return(s.fallbackSubnetInfo(), nil)
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

	s.st.EXPECT().GetAllSubnets(gomock.Any()).Return(s.fallbackSubnetInfo(), nil)
	s.st.EXPECT().CreateCloudServices(gomock.Any(), gomock.Any()).Return(nil)
	s.st.EXPECT().ImportLinkLayerDevices(gomock.Any(), gomock.Any()).Return(errors.New("import devices error"))

	// Act
	err := s.migrationService(c).ImportCloudServices(c.Context(), []internal.ImportCloudService{})

	// Assert: the error from ImportLinkLayerDevices is passed through to the caller
	c.Assert(err, tc.ErrorMatches, "importing link layer devices: import devices error")
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

	s.st.EXPECT().GetAllSubnets(gomock.Any()).Return(s.fallbackSubnetInfo(), nil)

	err := s.migrationService(c).ImportCloudServices(c.Context(), services)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *migrationSuite) TestImportCloudServicesIPv4AndIPv6Success(c *tc.C) {
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
					UUID:    "addr-uuid1",
					Value:   "192.0.2.1",
					Type:    "ipv4",
					Scope:   "local-cloud",
					Origin:  "provider",
					SpaceID: "space-id",
				},
				{
					UUID:    "addr-uuid2",
					Value:   "be7b:a111:58aa:fe61:a0b9:81c7:f136:697f",
					Type:    "ipv6",
					Scope:   "local-cloud",
					Origin:  "provider",
					SpaceID: "space-id",
				},
			},
		},
	}

	// Create subnet info for the test
	subnets := s.fallbackSubnetInfo()
	c.Assert(subnets, tc.HasLen, 2)

	s.st.EXPECT().GetAllSubnets(gomock.Any()).Return(subnets, nil)

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
			"addr-uuid1": subnets[0].ID.String(),
			"addr-uuid2": subnets[1].ID.String(),
		},
	}).Return(nil)

	// Act
	err := s.migrationService(c).ImportCloudServices(c.Context(), services)

	// Assert: no error is returned
	c.Assert(err, tc.IsNil)
}

func (s *migrationSuite) TestImportCloudServicesIPv4Success(c *tc.C) {
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
	subnets := s.fallbackSubnetInfo()
	c.Assert(subnets, tc.HasLen, 2)

	s.st.EXPECT().GetAllSubnets(gomock.Any()).Return(subnets, nil)

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
			"addr-uuid": subnets[0].ID.String(),
		},
	}).Return(nil)

	// Act
	err := s.migrationService(c).ImportCloudServices(c.Context(), services)

	// Assert: no error is returned
	c.Assert(err, tc.IsNil)
}

func (s *migrationSuite) TestImportCloudServicesIPv6Success(c *tc.C) {
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
					Value:   "be7b:a111:58aa:fe61:a0b9:81c7:f136:697f",
					Type:    "ipv6",
					Scope:   "local-cloud",
					Origin:  "provider",
					SpaceID: "space-id",
				},
			},
		},
	}

	// Create subnet info for the test
	subnets := s.fallbackSubnetInfo()
	c.Assert(subnets, tc.HasLen, 2)

	s.st.EXPECT().GetAllSubnets(gomock.Any()).Return(subnets, nil)

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
			"addr-uuid": subnets[1].ID.String(),
		},
	}).Return(nil)

	// Act
	err := s.migrationService(c).ImportCloudServices(c.Context(), services)

	// Assert: no error is returned
	c.Assert(err, tc.IsNil)
}

func (s *migrationSuite) fallbackSubnetInfo() corenetwork.SubnetInfos {
	return corenetwork.SubnetInfos{
		{
			ID:   corenetwork.Id(uuid.MustNewUUID().String()),
			CIDR: corenetwork.FallbackSubnetInfo[0].CIDR,
		},
		{
			ID:   corenetwork.Id(uuid.MustNewUUID().String()),
			CIDR: corenetwork.FallbackSubnetInfo[1].CIDR,
		},
	}
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
		tc.Commentf("mismatch between inserted cloud services and imported one")) {
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
			Type:            network.DeviceTypeUnknown,
			VirtualPortType: corenetwork.NonVirtualPort,
		})
		if !m.c.Check(slices.Collect(maps.Keys(inputAddrByUUID)), tc.SameContents,
			slices.Collect(maps.Keys(expectedAddrByUUID)),
			tc.Commentf("mismatch between inserted cloud services addresses and imported one")) {
			continue
		}
		for k, inAddr := range inputAddrByUUID {
			expectedAddr := expectedAddrByUUID[k]
			result = result && m.c.Check(inAddr, tc.DeepEquals, internal.ImportIPAddress{
				UUID:         expectedAddr.UUID,
				Type:         corenetwork.AddressType(expectedAddr.Type),
				Scope:        corenetwork.Scope(expectedAddr.Scope),
				AddressValue: expectedAddr.Value,
				ConfigType:   corenetwork.ConfigStatic,
				Origin:       corenetwork.Origin(expectedAddr.Origin),
				SubnetUUID:   m.subnetUUIDs[expectedAddr.UUID],
			})
		}
	}
	return result
}

func (cloudServiceLLDMatcher) String() string {
	return "matches args for ImportLinkLayerDevices"
}

func (s *migrationSuite) TestSetMachineNetConfigBadUUIDError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := machine.UUID("bad-machine-uuid")

	err := s.service(c).SetMachineNetConfig(c.Context(), mUUID, nil)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

type spaceInfoAsArgMatcher struct {
	c        *tc.C
	expected corenetwork.SubnetInfo
}

func (m spaceInfoAsArgMatcher) Matches(x interface{}) bool {
	obtained, ok := x.(corenetwork.SubnetInfo)
	m.c.Assert(ok, tc.IsTrue)
	if !ok {
		return false
	}
	mc := tc.NewMultiChecker()
	mc.AddExpr("_.ID", tc.IsNonZeroUUID)
	return m.c.Check(obtained, mc, m.expected)
}

func (m spaceInfoAsArgMatcher) String() string {
	return "match the corenetwork.SubnetInfo modulo subnet uuid"
}

type importLinkLayerDeviceArgMatcher struct {
	c        *tc.C
	expected []internal.ImportLinkLayerDevice
}

func (m importLinkLayerDeviceArgMatcher) Matches(x interface{}) bool {
	obtained, ok := x.([]internal.ImportLinkLayerDevice)
	m.c.Assert(ok, tc.IsTrue)
	if !ok {
		return false
	}
	mc := tc.NewMultiChecker()
	mc.AddExpr("_.Addresses", tc.Ignore)
	if m.c.Check(
		obtained,
		tc.UnorderedMatch[[]internal.ImportLinkLayerDevice](mc),
		m.expected,
		tc.Commentf("top level fail"),
	) == false {
		return false
	}

	mc = tc.NewMultiChecker()
	mc.AddExpr("_.SubnetUUID", tc.IsNonZeroUUID)
	for i := range m.expected {
		if m.c.Check(
			obtained[i].Addresses,
			tc.UnorderedMatch[[]internal.ImportIPAddress](mc),
			m.expected[i].Addresses,
			tc.Commentf("[%d].Addresses fail", i),
		) == false {
			return false
		}
	}
	return true
}

func (m importLinkLayerDeviceArgMatcher) String() string {
	return "match the internal.ImportLinkLayerDevice"
}
