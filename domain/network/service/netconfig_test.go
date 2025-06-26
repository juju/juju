// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	corenetwork "github.com/juju/juju/core/network"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type netConfigSuite struct {
	testhelpers.IsolationSuite

	st *MockState
}

func (s *netConfigSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.st = NewMockState(ctrl)
	c.Cleanup(func() { s.st = nil })
	return ctrl
}

func (s *netConfigSuite) service(c *tc.C) *Service {
	return NewService(s.st, loggertesting.WrapCheckLog(c))
}

func TestNetConfigSuite(t *testing.T) {
	tc.Run(t, &netConfigSuite{})
}

func (s *netConfigSuite) TestImportLinkLayerDevices(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	netNodeUUID := uuid.MustNewUUID().String()
	nameMap := map[string]string{
		"88": netNodeUUID,
	}
	args := []internal.ImportLinkLayerDevice{
		{MachineID: "88"},
	}
	expectedArgs := args
	expectedArgs[0].NetNodeUUID = netNodeUUID
	s.st.EXPECT().AllMachinesAndNetNodes(gomock.Any()).Return(nameMap, nil)
	s.st.EXPECT().ImportLinkLayerDevices(gomock.Any(), args).Return(nil)

	// Act
	err := s.migrationService(c).ImportLinkLayerDevices(c.Context(), args)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
}

func (s *netConfigSuite) TestImportLinkLayerDevicesMachines(c *tc.C) {
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

func (s *netConfigSuite) TestImportLinkLayerDevicesNoContent(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	// Act
	err := s.migrationService(c).ImportLinkLayerDevices(c.Context(), []internal.ImportLinkLayerDevice{})

	// Assert: no failure if no data provided.
	c.Assert(err, tc.ErrorIsNil)
}

func (s *netConfigSuite) TestDeleteImportedLinkLayerDevices(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	s.st.EXPECT().DeleteImportedLinkLayerDevices(gomock.Any()).Return(errors.New("boom"))

	// Act
	err := s.migrationService(c).DeleteImportedLinkLayerDevices(c.Context())

	// Assert: the error from DeleteImportedLinkLayerDevices is passed
	// through to the caller.
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *netConfigSuite) migrationService(c *tc.C) *MigrationService {
	return NewMigrationService(s.st, loggertesting.WrapCheckLog(c))
}

func (s *netConfigSuite) TestSetMachineNetConfigBadUUIDError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := machine.UUID("bad-machine-uuid")

	err := s.service(c).SetMachineNetConfig(c.Context(), mUUID, nil)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *netConfigSuite) TestSetMachineNetConfigNodeNotFoundError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID, err := machine.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), mUUID.String()).Return("", machineerrors.MachineNotFound)

	nics := []network.NetInterface{{Name: "eth0"}}

	err = s.service(c).SetMachineNetConfig(c.Context(), mUUID, nics)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *netConfigSuite) TestSetMachineNetConfigSetCallError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	nUUID := "set-node-uuid"
	mUUID, err := machine.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	nics := []network.NetInterface{{Name: "eth0"}}

	exp := s.st.EXPECT()
	exp.GetMachineNetNodeUUID(gomock.Any(), mUUID.String()).Return(nUUID, nil)
	exp.SetMachineNetConfig(gomock.Any(), nUUID, nics).Return(errors.New("boom"))

	err = s.service(c).SetMachineNetConfig(c.Context(), mUUID, nics)
	c.Assert(err, tc.ErrorMatches, "setting net config for machine .* boom")
}

func (s *netConfigSuite) TestSetMachineNetConfigEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID, err := machine.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	err = s.service(c).SetMachineNetConfig(c.Context(), mUUID, nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *netConfigSuite) TestSetMachineNetConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ctx := c.Context()

	nUUID := "set-node-uuid"
	mUUID, err := machine.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	nics := []network.NetInterface{
		{
			Name: "eth0",
			Addrs: []network.NetAddr{
				{
					InterfaceName: "eth0",
					AddressValue:  "10.0.0.5/16",
					AddressType:   corenetwork.IPv4Address,
					ConfigType:    corenetwork.ConfigDHCP,
					Origin:        corenetwork.OriginMachine,
					Scope:         corenetwork.ScopeCloudLocal,
				},
			},
		},
	}

	exp := s.st.EXPECT()
	exp.GetMachineNetNodeUUID(gomock.Any(), mUUID.String()).Return(nUUID, nil)
	exp.SetMachineNetConfig(gomock.Any(), nUUID, nics).Return(nil)

	err = s.service(c).SetMachineNetConfig(ctx, mUUID, nics)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *netConfigSuite) TestSetProviderNetConfigInvalidMachineUUID(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	invalidUUID := machine.UUID("invalid-uuid")

	// Act
	err := s.service(c).SetProviderNetConfig(c.Context(), invalidUUID, nil)

	// Assert
	c.Assert(err, tc.ErrorMatches, `invalid machine UUID: id "invalid-uuid" not valid`)
}

func (s *netConfigSuite) TestSetProviderNetConfigGetNetNodeError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	machineUUID := machine.UUID(uuid.MustNewUUID().String())
	stateErr := errors.New("boom")

	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).Return("", stateErr)

	// Act
	err := s.service(c).SetProviderNetConfig(c.Context(), machineUUID, nil)

	// Assert
	c.Assert(err, tc.ErrorIs, stateErr)
}

func (s *netConfigSuite) TestSetProviderNetConfigError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	machineUUID := machine.UUID(uuid.MustNewUUID().String())
	nodeUUID := "node-uuid"
	incoming := []network.NetInterface{{}, {}}
	stateErr := errors.New("boom")

	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).Return(nodeUUID, nil)
	s.st.EXPECT().MergeLinkLayerDevice(gomock.Any(), nodeUUID, incoming).Return(stateErr)

	// Act
	err := s.service(c).SetProviderNetConfig(c.Context(), machineUUID, incoming)

	// Assert
	c.Assert(err, tc.ErrorIs, stateErr)
}

func (s *netConfigSuite) TestSetProviderNetConfig(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	machineUUID := machine.UUID(uuid.MustNewUUID().String())
	nodeUUID := "node-uuid"
	incoming := []network.NetInterface{
		{},
		{},
	}
	s.st.EXPECT().GetMachineNetNodeUUID(gomock.Any(), machineUUID.String()).Return(nodeUUID, nil)
	s.st.EXPECT().MergeLinkLayerDevice(gomock.Any(), nodeUUID, incoming).Return(nil)

	// Act
	err := s.service(c).SetProviderNetConfig(c.Context(), machineUUID, incoming)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestGetAllDevicesByMachineNamesMultipleMachinesWithDevices validates fetching
// devices for multiple machines with linked devices.
// It ensures devices are correctly mapped to machine names using mocked
// storage layer behavior.
func (s *netConfigSuite) TestGetAllDevicesByMachineNamesMultipleMachinesWithDevices(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	eth01 := network.NetInterface{Name: "eth0", MACAddress: ptr("00:11:22:33:44:55")}
	eth02 := network.NetInterface{Name: "eth0", MACAddress: ptr("aa:bb:cc:dd:ee:ff")}
	eth1 := network.NetInterface{Name: "eth1", MACAddress: ptr("00:11:22:33:44:66")}

	// Mock AllMachinesAndNetNodes to return a map of machine names to node UUIDs
	s.st.EXPECT().AllMachinesAndNetNodes(gomock.Any()).Return(map[string]string{
		"machine-0": "node-uuid-1",
		"machine-1": "node-uuid-2",
	}, nil)

	// Mock GetAllLinkLayerDevicesByNetNodeUUIDs to return devices for each node UUID
	s.st.EXPECT().GetAllLinkLayerDevicesByNetNodeUUIDs(gomock.Any()).Return(map[string][]network.NetInterface{
		"node-uuid-1": {eth01, eth02},
		"node-uuid-2": {eth1},
	}, nil)

	// Act
	result, err := s.service(c).GetAllDevicesByMachineNames(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 2)
	c.Assert(result["machine-0"], tc.SameContents, []network.NetInterface{eth01, eth02})
	c.Assert(result["machine-1"], tc.SameContents, []network.NetInterface{eth1})
}

// TestGetAllDevicesByMachineNamesEmptyResult verifies that the method handles
// an empty result scenario correctly.
// It tests the case where AllMachinesAndNetNodes and
// GetAllLinkLayerDevicesByNetNodeUUIDs return empty results.
// Ensures no errors occur and the output is an empty map.
func (s *netConfigSuite) TestGetAllDevicesByMachineNamesEmptyResult(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	// Mock AllMachinesAndNetNodes to return an empty map
	s.st.EXPECT().AllMachinesAndNetNodes(gomock.Any()).Return(map[string]string{}, nil)

	// Mock GetAllLinkLayerDevicesByNetNodeUUIDs to return an empty map
	s.st.EXPECT().GetAllLinkLayerDevicesByNetNodeUUIDs(gomock.Any()).Return(map[string][]network.NetInterface{}, nil)

	// Act
	result, err := s.service(c).GetAllDevicesByMachineNames(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 0)
}

// TestGetAllDevicesByMachineNamesMachinesWithNoDevices tests retrieving devices
// for machines when one machine has no associated devices.
// It validates behavior when machines are mapped but one has an empty device
// list, ensuring correctness of the returned data structure.
func (s *netConfigSuite) TestGetAllDevicesByMachineNamesMachinesWithNoDevices(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	eth0 := network.NetInterface{Name: "eth0", MACAddress: ptr("00:11:22:33:44:55")}
	// Mock AllMachinesAndNetNodes to return a map with machines
	s.st.EXPECT().AllMachinesAndNetNodes(gomock.Any()).Return(map[string]string{
		"machine-0": "node-uuid-1",
		"machine-1": "node-uuid-2",
	}, nil)

	// Mock GetAllLinkLayerDevicesByNetNodeUUIDs to return devices for only one node
	s.st.EXPECT().GetAllLinkLayerDevicesByNetNodeUUIDs(gomock.Any()).Return(map[string][]network.NetInterface{
		"node-uuid-1": {eth0},
		// No devices for node-uuid-2
	}, nil)

	// Act
	result, err := s.service(c).GetAllDevicesByMachineNames(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 2)
	c.Assert(result["machine-0"], tc.SameContents, []network.NetInterface{eth0})
	c.Assert(result["machine-1"], tc.HasLen, 0) // Empty slice for machine-1
}

// TestGetAllDevicesByMachineNamesGetDevicesError validates error handling when
// GetAllLinkLayerDevicesByNetNodeUUIDs fails.
func (s *netConfigSuite) TestGetAllDevicesByMachineNamesGetDevicesError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	// Mock GetAllLinkLayerDevicesByNetNodeUUIDs to return an error
	s.st.EXPECT().GetAllLinkLayerDevicesByNetNodeUUIDs(gomock.Any()).Return(nil, errors.New("database connection failed"))

	// Act
	result, err := s.service(c).GetAllDevicesByMachineNames(c.Context())

	// Assert
	c.Assert(err, tc.ErrorMatches, "retrieving devices by node UUIDs: database connection failed")
	c.Assert(result, tc.IsNil)
}

// TestGetAllDevicesByMachineNamesGetMachinesError verifies the behavior when
// retrieving machine names to UUIDs fails.
func (s *netConfigSuite) TestGetAllDevicesByMachineNamesGetMachinesError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	// Mock GetAllLinkLayerDevicesByNetNodeUUIDs to succeed
	s.st.EXPECT().GetAllLinkLayerDevicesByNetNodeUUIDs(gomock.Any()).Return(map[string][]network.NetInterface{}, nil)

	// Mock AllMachinesAndNetNodes to return an error
	s.st.EXPECT().AllMachinesAndNetNodes(gomock.Any()).Return(nil, errors.New("database query failed"))

	// Act
	result, err := s.service(c).GetAllDevicesByMachineNames(c.Context())

	// Assert
	c.Assert(err, tc.ErrorMatches, "retrieving machine names to UUIDs: database query failed")
	c.Assert(result, tc.IsNil)
}

// TestGetAllDevicesByMachineNamesNodeUUIDNotFound validates the behavior when
// node UUIDs are not found for the given machines.
func (s *netConfigSuite) TestGetAllDevicesByMachineNamesNodeUUIDNotFound(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	// Mock AllMachinesAndNetNodes to return a map with machines
	s.st.EXPECT().AllMachinesAndNetNodes(gomock.Any()).Return(map[string]string{
		"machine-0": "node-uuid-1",
		"machine-1": "node-uuid-2",
	}, nil)

	// Mock GetAllLinkLayerDevicesByNetNodeUUIDs to return devices for a different node UUID
	s.st.EXPECT().GetAllLinkLayerDevicesByNetNodeUUIDs(gomock.Any()).Return(map[string][]network.NetInterface{
		"node-uuid-3": {
			{Name: "eth0", MACAddress: ptr("00:11:22:33:44:55")},
		},
	}, nil)

	// Act
	result, err := s.service(c).GetAllDevicesByMachineNames(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 2)
	c.Assert(result["machine-0"], tc.HasLen, 0) // Empty slice for machine-0
	c.Assert(result["machine-1"], tc.HasLen, 0) // Empty slice for machine-1
}

func ptr[T any](v T) *T {
	return &v
}
