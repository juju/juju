// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	domainmachine "github.com/juju/juju/domain/machine"
	"github.com/juju/juju/internal/uuid"
)

// pollingInfoSuite composes baseSuite to test GetPollingInfos.
type pollingInfoSuite struct {
	baseSuite
}

func TestPollingInfoSuite(t *stdtesting.T) { tc.Run(t, &pollingInfoSuite{}) }

func (s *pollingInfoSuite) TestGetPollingInfosOneMachineNoDevicesNoInstanceID(c *tc.C) {
	// Arrange
	mName := "0"
	mUUID := s.addMachineWithInstance(c, mName, "")

	// Act
	infos, err := s.state.GetPollingInfos(c.Context(), []string{mName})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(infos, tc.SameContents, domainmachine.PollingInfos{{
		MachineUUID:         machine.UUID(mUUID),
		MachineName:         machine.Name(mName),
		InstanceID:          "",
		ExistingDeviceCount: 0,
	}})
}

func (s *pollingInfoSuite) TestGetPollingInfosOneMachineMultipleDevices(c *tc.C) {
	// Arrange
	mName := "1"
	mUUID := s.addMachineWithInstance(c, mName, "")
	s.addDeviceForMachine(c, mUUID, "eth0")
	s.addDeviceForMachine(c, mUUID, "eth1")

	// Act
	infos, err := s.state.GetPollingInfos(c.Context(), []string{mName})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(infos, tc.SameContents, domainmachine.PollingInfos{{
		MachineUUID:         machine.UUID(mUUID),
		MachineName:         machine.Name(mName),
		InstanceID:          "",
		ExistingDeviceCount: 2,
	}})
}

func (s *pollingInfoSuite) TestGetPollingInfosMultipleMachineMultipleDevice(c *tc.C) {
	// Arrange
	mA := "2" // 0 devices
	mB := "3" // 1 device
	mC := "4" // 3 devices
	idA := "instance-a"
	idB := "instance-b"
	uA := s.addMachineWithInstance(c, mA, idA)
	uB := s.addMachineWithInstance(c, mB, idB)
	uC := s.addMachineWithInstance(c, mC, "")
	s.addDeviceForMachine(c, uB, "eth0")
	s.addDeviceForMachine(c, uC, "eth0")
	s.addDeviceForMachine(c, uC, "eth1")
	s.addDeviceForMachine(c, uC, "eth2")

	// Act
	names := []string{mA, mB, mC}
	infos, err := s.state.GetPollingInfos(c.Context(), names)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(infos, tc.SameContents, domainmachine.PollingInfos{{
		MachineUUID:         machine.UUID(uA),
		MachineName:         machine.Name(mA),
		InstanceID:          instance.Id(idA),
		ExistingDeviceCount: 0,
	}, {
		MachineUUID:         machine.UUID(uB),
		MachineName:         machine.Name(mB),
		InstanceID:          instance.Id(idB),
		ExistingDeviceCount: 1,
	}, {
		MachineUUID:         machine.UUID(uC),
		MachineName:         machine.Name(mC),
		InstanceID:          "",
		ExistingDeviceCount: 3,
	}})
}

func (s *pollingInfoSuite) TestGetPollingInfosOneMachineWithInstanceID(c *tc.C) {
	// Arrange
	mName := "5"
	mInstanceID := "i-abc123"
	mUUID := s.addMachineWithInstance(c, mName, mInstanceID)

	// Act
	infos, err := s.state.GetPollingInfos(c.Context(), []string{mName})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(infos, tc.SameContents, domainmachine.PollingInfos{{
		MachineUUID:         machine.UUID(mUUID),
		MachineName:         machine.Name(mName),
		InstanceID:          instance.Id(mInstanceID),
		ExistingDeviceCount: 0,
	}})
}

func (s *pollingInfoSuite) TestGetPollingInfosMissingMachine(c *tc.C) {
	// Arrange
	mName := "5"
	_ = s.addMachineWithInstance(c, mName, "")

	// Act
	infos, err := s.state.GetPollingInfos(c.Context(), []string{mName, "unknown"})

	// Assert - the call work but the unknown machine is not returned.
	c.Assert(err, tc.ErrorIsNil)
	c.Check(infos, tc.HasLen, 1)
}

func (s *pollingInfoSuite) TestGetPollingInfosEmptyInput(c *tc.C) {
	// Arrange
	// none

	// Act
	infos, err := s.state.GetPollingInfos(c.Context(), []string{})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(infos, tc.HasLen, 0)
}

// addMachineWithInstance inserts minimal rows into net_node, machine, and
// machine_cloud_instance.
// instanceID can be empty to represent NULL.
func (s *pollingInfoSuite) addMachineWithInstance(c *tc.C, name string, instanceID string) string {
	machineUUID := s.addMachine(c, name, s.addNetNode(c))
	var instanceIDPtr *string
	if instanceID != "" {
		instanceIDPtr = &instanceID
	}
	s.runQuery(c, `INSERT INTO machine_cloud_instance (machine_uuid, instance_id, life_id) VALUES (?, ?, 0)`,
		machineUUID,
		instanceIDPtr)
	return machineUUID
}

// addDeviceForMachine creates a link_layer_device for the given machine and
// returns the device UUID.
func (s *pollingInfoSuite) addDeviceForMachine(c *tc.C, machineUUID string, devName string) string {
	// Insert a basic ethernet device with NonVirtualPort and defaults.
	devUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `
INSERT INTO link_layer_device (    uuid, net_node_uuid, name, device_type_id, virtual_port_type_id) 
VALUES (?, (SELECT net_node_uuid FROM machine WHERE uuid = ?), ?, 2, 0)`, devUUID, machineUUID, devName)

	return devUUID
}
