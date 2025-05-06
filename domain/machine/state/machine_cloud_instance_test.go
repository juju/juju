// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	coremachinetesting "github.com/juju/juju/core/machine/testing"
	domainmachine "github.com/juju/juju/domain/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
)

func (s *stateSuite) TestGetHardwareCharacteristicsWithNoData(c *gc.C) {
	machineUUID := coremachinetesting.GenUUID(c)

	_, err := s.state.HardwareCharacteristics(context.Background(), machineUUID)
	c.Assert(err, jc.ErrorIs, machineerrors.NotProvisioned)
}

func (s *stateSuite) TestGetHardwareCharacteristics(c *gc.C) {
	machineUUID := s.ensureInstance(c, "42")

	hc, err := s.state.HardwareCharacteristics(context.Background(), machineUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(*hc.Arch, gc.Equals, "arm64")
	c.Check(*hc.Mem, gc.Equals, uint64(1024))
	c.Check(*hc.RootDisk, gc.Equals, uint64(256))
	c.Check(*hc.RootDiskSource, gc.Equals, "/test")
	c.Check(*hc.CpuCores, gc.Equals, uint64(4))
	c.Check(*hc.CpuPower, gc.Equals, uint64(75))
	c.Check(*hc.AvailabilityZone, gc.Equals, "az-1")
	c.Check(*hc.VirtType, gc.Equals, "virtual-machine")
}

func (s *stateSuite) TestGetHardwareCharacteristicsWithoutAvailabilityZone(c *gc.C) {
	db := s.DB()
	// Create a reference machine.
	err := s.state.CreateMachine(context.Background(), "42", "", "")
	c.Assert(err, jc.ErrorIsNil)
	var machineUUID machine.UUID
	err = db.QueryRowContext(context.Background(), "SELECT uuid FROM machine WHERE name='42'").Scan(&machineUUID)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetMachineCloudInstance(
		context.Background(),
		machineUUID,
		instance.Id("123"),
		"",
		&instance.HardwareCharacteristics{
			Arch:           ptr("arm64"),
			Mem:            ptr[uint64](1024),
			RootDisk:       ptr[uint64](256),
			RootDiskSource: ptr("/test"),
			CpuCores:       ptr[uint64](4),
			CpuPower:       ptr[uint64](75),
			Tags:           ptr([]string{"tag1", "tag2"}),
			VirtType:       ptr("virtual-machine"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	hc, err := s.state.HardwareCharacteristics(context.Background(), machineUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(*hc.Arch, gc.Equals, "arm64")
	c.Check(*hc.Mem, gc.Equals, uint64(1024))
	c.Check(*hc.RootDisk, gc.Equals, uint64(256))
	c.Check(*hc.RootDiskSource, gc.Equals, "/test")
	c.Check(*hc.CpuCores, gc.Equals, uint64(4))
	c.Check(*hc.CpuPower, gc.Equals, uint64(75))
	c.Check(hc.AvailabilityZone, gc.IsNil)
	c.Check(*hc.VirtType, gc.Equals, "virtual-machine")
}

func (s *stateSuite) TestAvailabilityZoneWithNoMachine(c *gc.C) {
	machineUUID := coremachinetesting.GenUUID(c)

	_, err := s.state.AvailabilityZone(context.Background(), machineUUID)
	c.Assert(err, jc.ErrorIs, machineerrors.AvailabilityZoneNotFound)
}

func (s *stateSuite) TestAvailabilityZone(c *gc.C) {
	machineUUID := s.ensureInstance(c, "42")

	az, err := s.state.AvailabilityZone(context.Background(), machineUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(az, gc.Equals, "az-1")
}

func (s *stateSuite) TestSetInstanceData(c *gc.C) {
	db := s.DB()

	// Create a reference machine.
	err := s.state.CreateMachine(context.Background(), "42", "", "")
	c.Assert(err, jc.ErrorIsNil)
	var machineUUID machine.UUID
	row := db.QueryRowContext(context.Background(), "SELECT uuid FROM machine WHERE name='42'")
	c.Assert(row.Err(), jc.ErrorIsNil)
	err = row.Scan(&machineUUID)
	c.Assert(err, jc.ErrorIsNil)
	// Add a reference AZ.
	_, err = db.ExecContext(context.Background(), "INSERT INTO availability_zone VALUES('deadbeef-0bad-400d-8000-4b1d0d06f00d', 'az-1')")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetMachineCloudInstance(
		context.Background(),
		machineUUID,
		instance.Id("1"),
		"one",
		&instance.HardwareCharacteristics{
			Arch:             ptr("arm64"),
			Mem:              ptr[uint64](1024),
			RootDisk:         ptr[uint64](256),
			CpuCores:         ptr[uint64](4),
			CpuPower:         ptr[uint64](75),
			Tags:             ptr([]string{"tag1", "tag2"}),
			AvailabilityZone: ptr("az-1"),
			VirtType:         ptr("virtual-machine"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	var instanceData instanceData
	row = db.QueryRowContext(context.Background(), "SELECT * FROM machine_cloud_instance WHERE instance_id='1'")
	c.Assert(row.Err(), jc.ErrorIsNil)
	err = row.Scan(
		&instanceData.MachineUUID,
		&instanceData.InstanceID,
		&instanceData.DisplayName,
		&instanceData.Arch,
		&instanceData.Mem,
		&instanceData.RootDisk,
		&instanceData.RootDiskSource,
		&instanceData.CPUCores,
		&instanceData.CPUPower,
		&instanceData.AvailabilityZoneUUID,
		&instanceData.VirtType,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(instanceData.MachineUUID, gc.Equals, machineUUID)
	c.Check(instanceData.InstanceID, gc.Equals, "1")
	c.Check(instanceData.DisplayName, gc.Equals, "one")
	c.Check(*instanceData.Arch, gc.Equals, "arm64")
	c.Check(*instanceData.Mem, gc.Equals, uint64(1024))
	c.Check(*instanceData.RootDisk, gc.Equals, uint64(256))
	// Make sure we also handle correctly NULL values.
	c.Check(instanceData.RootDiskSource, gc.IsNil)
	c.Check(*instanceData.CPUCores, gc.Equals, uint64(4))
	c.Check(*instanceData.CPUPower, gc.Equals, uint64(75))
	c.Check(*instanceData.AvailabilityZoneUUID, gc.Equals, "deadbeef-0bad-400d-8000-4b1d0d06f00d")
	c.Check(*instanceData.VirtType, gc.Equals, "virtual-machine")

	rows, err := db.QueryContext(context.Background(), "SELECT tag FROM instance_tag WHERE machine_uuid='"+machineUUID.String()+"'")
	defer func() { _ = rows.Close() }()
	c.Assert(err, jc.ErrorIsNil)
	var instanceTags []string
	for rows.Next() {
		var tag string
		err = rows.Scan(&tag)
		c.Assert(err, jc.ErrorIsNil)
		instanceTags = append(instanceTags, tag)
	}
	c.Check(instanceTags, gc.HasLen, 2)
	c.Check(instanceTags[0], gc.Equals, "tag1")
	c.Check(instanceTags[1], gc.Equals, "tag2")
}

func (s *stateSuite) TestSetInstanceDataAlreadyExists(c *gc.C) {
	db := s.DB()

	// Create a reference machine.
	err := s.state.CreateMachine(context.Background(), "42", "", "")
	c.Assert(err, jc.ErrorIsNil)
	var machineUUID machine.UUID
	row := db.QueryRowContext(context.Background(), "SELECT uuid FROM machine WHERE name='42'")
	c.Assert(row.Err(), jc.ErrorIsNil)
	err = row.Scan(&machineUUID)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetMachineCloudInstance(
		context.Background(),
		machineUUID,
		instance.Id("1"),
		"one",
		&instance.HardwareCharacteristics{
			Arch: ptr("arm64"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	// Must fail when we try to add again.
	err = s.state.SetMachineCloudInstance(
		context.Background(),
		machineUUID,
		instance.Id("1"),
		"one",
		&instance.HardwareCharacteristics{
			Arch: ptr("amd64"),
		},
	)
	c.Assert(err, gc.ErrorMatches, "machine cloud instance already exists.*")
	c.Assert(err, jc.ErrorIs, machineerrors.MachineCloudInstanceAlreadyExists)
}

// TestDeleteInstanceData asserts the happy path of DeleteMachineCloudInstance
// at the state layer.
func (s *stateSuite) TestDeleteInstanceData(c *gc.C) {
	db := s.DB()

	machineUUID := s.ensureInstance(c, "42")

	err := s.state.DeleteMachineCloudInstance(context.Background(), machineUUID)
	c.Assert(err, jc.ErrorIsNil)

	// Check that all rows've been deleted.
	rows, err := db.QueryContext(context.Background(), "SELECT * FROM machine_cloud_instance WHERE instance_id='1'")
	defer func() { _ = rows.Close() }()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rows.Err(), jc.ErrorIsNil)
	c.Check(rows.Next(), jc.IsFalse)
	rows, err = db.QueryContext(context.Background(), "SELECT * FROM instance_tag WHERE machine_uuid='"+machineUUID.String()+"'")
	defer func() { _ = rows.Close() }()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rows.Err(), jc.ErrorIsNil)
	c.Check(rows.Next(), jc.IsFalse)
}

// TestDeleteInstanceDataWithStatus asserts that DeleteMachineCloudInstance at
// the state layer removes any instance status and status data when deleting an
// instance.
func (s *stateSuite) TestDeleteInstanceDataWithStatus(c *gc.C) {
	db := s.DB()

	machineUUID := s.ensureInstance(c, "42")

	// Add a status with data for this instance
	s.state.SetInstanceStatus(context.Background(), "42", domainmachine.StatusInfo[domainmachine.InstanceStatusType]{
		Status:  domainmachine.InstanceStatusAllocating,
		Message: "running",
		Data:    []byte(`{"key":"data"}`),
		Since:   ptr(time.Now().UTC()),
	})

	err := s.state.DeleteMachineCloudInstance(context.Background(), machineUUID)
	c.Assert(err, jc.ErrorIsNil)

	// Check that all rows've been deleted from the status tables.
	var status int
	err = db.QueryRowContext(context.Background(), "SELECT count(*) FROM machine_cloud_instance_status WHERE machine_uuid=?", "123").Scan(&status)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(status, gc.Equals, 0)
}

func (s *stateSuite) TestInstanceIdSuccess(c *gc.C) {
	machineUUID := s.ensureInstance(c, "666")

	instanceId, err := s.state.InstanceID(context.Background(), machineUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(instanceId, gc.Equals, "123")
}

func (s *stateSuite) TestInstanceIdError(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "")
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.state.InstanceID(context.Background(), "666")
	c.Assert(err, jc.ErrorIs, machineerrors.NotProvisioned)
}

func (s *stateSuite) TestInstanceNameSuccess(c *gc.C) {
	machineUUID := s.ensureInstance(c, "666")

	instanceID, displayName, err := s.state.InstanceIDAndName(context.Background(), machineUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instanceID, gc.Equals, "123")
	c.Assert(displayName, gc.Equals, "one-two-three")
}

func (s *stateSuite) TestInstanceNameError(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "")
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = s.state.InstanceIDAndName(context.Background(), "666")
	c.Assert(err, jc.ErrorIs, machineerrors.NotProvisioned)
}

// TestGetInstanceStatusSuccess asserts the happy path of InstanceStatus at the
// state layer.
func (s *stateSuite) TestGetInstanceStatusSuccess(c *gc.C) {
	db := s.DB()

	machineUUID := s.ensureInstance(c, "666")

	// Add a status value for this machine into the
	// machine_cloud_instance_status table using the machineUUID and the status
	// value 2 for "running" (from machine_cloud_instance_status_value table).
	_, err := db.ExecContext(context.Background(), `INSERT INTO machine_cloud_instance_status VALUES(?, '2', 'running', NULL, '2024-07-12 12:00:00')`, machineUUID)
	c.Assert(err, jc.ErrorIsNil)

	obtainedStatus, err := s.state.GetInstanceStatus(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	expectedStatus := domainmachine.StatusInfo[domainmachine.InstanceStatusType]{
		Status:  domainmachine.InstanceStatusRunning,
		Message: "running",
		Since:   ptr(time.Date(2024, 7, 12, 12, 0, 0, 0, time.UTC)),
	}
	c.Check(obtainedStatus, gc.DeepEquals, expectedStatus)
}

// TestGetInstanceStatusSuccessWithData asserts the happy path of InstanceStatus
// at the state layer.
func (s *stateSuite) TestGetInstanceStatusSuccessWithData(c *gc.C) {
	db := s.DB()
	machineUUID := s.ensureInstance(c, "666")

	// Add a status value for this machine into the
	// machine_cloud_instance_status table using the machineUUID and the status
	// value 2 for "running" (from machine_cloud_instance_status_value table).
	_, err := db.ExecContext(context.Background(), `INSERT INTO machine_cloud_instance_status VALUES(?, '2', 'running', '{"key": "data"}', '2024-07-12 12:00:00')`, machineUUID)
	c.Assert(err, jc.ErrorIsNil)

	obtainedStatus, err := s.state.GetInstanceStatus(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	expectedStatus := domainmachine.StatusInfo[domainmachine.InstanceStatusType]{
		Status:  domainmachine.InstanceStatusRunning,
		Message: "running",
		Data:    []byte(`{"key": "data"}`),
		Since:   ptr(time.Date(2024, 7, 12, 12, 0, 0, 0, time.UTC)),
	}
	c.Check(obtainedStatus, gc.DeepEquals, expectedStatus)
}

// TestGetInstanceStatusNotFoundError asserts that GetInstanceStatus returns a
// NotFound error when the given machine cannot be found.
func (s *stateSuite) TestGetInstanceStatusNotFoundError(c *gc.C) {
	_, err := s.state.GetInstanceStatus(context.Background(), "666")
	c.Assert(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetInstanceStatusStatusNotSetError asserts that GetInstanceStatus returns
// a StatusNotSet error when a status value cannot be found for the given
// machine.
func (s *stateSuite) TestGetInstanceStatusStatusNotSetError(c *gc.C) {
	s.ensureInstance(c, "666")

	// Don't add a status value for this instance into the
	// machine_cloud_instance_status table.
	_, err := s.state.GetInstanceStatus(context.Background(), "666")
	c.Assert(err, jc.ErrorIs, machineerrors.StatusNotSet)
}

// TestSetInstanceStatusSuccess asserts the happy path of SetInstanceStatus at
// the state layer.
func (s *stateSuite) TestSetInstanceStatusSuccess(c *gc.C) {
	s.ensureInstance(c, "666")

	expectedStatus := domainmachine.StatusInfo[domainmachine.InstanceStatusType]{
		Status:  domainmachine.InstanceStatusRunning,
		Message: "running",
	}
	err := s.state.SetInstanceStatus(context.Background(), "666", expectedStatus)
	c.Assert(err, jc.ErrorIsNil)

	obtainedStatus, err := s.state.GetInstanceStatus(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedStatus.Status, gc.Equals, expectedStatus.Status)
	c.Assert(obtainedStatus.Message, gc.Equals, expectedStatus.Message)
}

// TestSetInstanceStatusSuccessWithData asserts the happy path of
// SetInstanceStatus at the state layer.
func (s *stateSuite) TestSetInstanceStatusSuccessWithData(c *gc.C) {
	s.ensureInstance(c, "666")

	expectedStatus := domainmachine.StatusInfo[domainmachine.InstanceStatusType]{
		Status:  domainmachine.InstanceStatusRunning,
		Message: "running",
		Data:    []byte(`{"key": "data"}`),
		Since:   ptr(time.Date(2024, 7, 12, 12, 0, 0, 0, time.UTC)),
	}
	err := s.state.SetInstanceStatus(context.Background(), "666", expectedStatus)
	c.Assert(err, jc.ErrorIsNil)

	obtainedStatus, err := s.state.GetInstanceStatus(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtainedStatus, gc.DeepEquals, expectedStatus)
}

// TestSetInstanceStatusError asserts that SetInstanceStatus returns a NotFound
// error when the given machine cannot be found.
func (s *stateSuite) TestSetInstanceStatusError(c *gc.C) {
	err := s.state.SetInstanceStatus(context.Background(), "666", domainmachine.StatusInfo[domainmachine.InstanceStatusType]{
		Status:  domainmachine.InstanceStatusRunning,
		Message: "running",
	})
	c.Assert(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestInstanceStatusValues asserts the keys and values in the
// machine_cloud_instance_status_value table, because we convert between core.status values
// and machine_cloud_instance_status_value based on these associations. This test will catch
// any discrepancies between the two sets of values, and error if/when any of
// them ever change.
func (s *stateSuite) TestInstanceStatusValues(c *gc.C) {
	db := s.DB()

	// Check that the status values in the machine_cloud_instance_status_value table match
	// the instance status values in core status.
	rows, err := db.QueryContext(context.Background(), "SELECT id, status FROM machine_cloud_instance_status_value")
	defer func() { _ = rows.Close() }()
	c.Assert(err, jc.ErrorIsNil)
	var statusValues []struct {
		ID   int
		Name string
	}
	for rows.Next() {
		var statusValue struct {
			ID   int
			Name string
		}
		err = rows.Scan(&statusValue.ID, &statusValue.Name)
		c.Assert(err, jc.ErrorIsNil)
		statusValues = append(statusValues, statusValue)
	}
	c.Assert(statusValues, gc.HasLen, 4)
	c.Check(statusValues[0].ID, gc.Equals, 0)
	c.Check(statusValues[0].Name, gc.Equals, "unknown")
	c.Check(statusValues[1].ID, gc.Equals, 1)
	c.Check(statusValues[1].Name, gc.Equals, "allocating")
	c.Check(statusValues[2].ID, gc.Equals, 2)
	c.Check(statusValues[2].Name, gc.Equals, "running")
	c.Check(statusValues[3].ID, gc.Equals, 3)
	c.Check(statusValues[3].Name, gc.Equals, "provisioning error")
}

func (s *stateSuite) ensureInstance(c *gc.C, mName machine.Name) machine.UUID {
	db := s.DB()

	// Create a reference machine.
	machineUUID := coremachinetesting.GenUUID(c)
	err := s.state.CreateMachine(context.Background(), mName, "", machineUUID)
	c.Assert(err, jc.ErrorIsNil)
	// Add a reference AZ.
	_, err = db.ExecContext(context.Background(), "INSERT INTO availability_zone VALUES('deadbeef-0bad-400d-8000-4b1d0d06f00d', 'az-1')")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetMachineCloudInstance(
		context.Background(),
		machineUUID,
		instance.Id("123"),
		"one-two-three",
		&instance.HardwareCharacteristics{
			Arch:             ptr("arm64"),
			Mem:              ptr[uint64](1024),
			RootDisk:         ptr[uint64](256),
			RootDiskSource:   ptr("/test"),
			CpuCores:         ptr[uint64](4),
			CpuPower:         ptr[uint64](75),
			Tags:             ptr([]string{"tag1", "tag2"}),
			AvailabilityZone: ptr("az-1"),
			VirtType:         ptr("virtual-machine"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	return machineUUID
}
