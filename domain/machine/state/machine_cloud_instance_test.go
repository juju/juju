// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	coremachinetesting "github.com/juju/juju/core/machine/testing"
	domainmachine "github.com/juju/juju/domain/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
)

func (s *stateSuite) TestGetHardwareCharacteristicsWithNoData(c *tc.C) {
	machineUUID := coremachinetesting.GenUUID(c)

	_, err := s.state.HardwareCharacteristics(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIs, machineerrors.NotProvisioned)
}

func (s *stateSuite) TestGetHardwareCharacteristics(c *tc.C) {
	machineUUID := s.ensureInstance(c, "42")

	hc, err := s.state.HardwareCharacteristics(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*hc.Arch, tc.Equals, "arm64")
	c.Check(*hc.Mem, tc.Equals, uint64(1024))
	c.Check(*hc.RootDisk, tc.Equals, uint64(256))
	c.Check(*hc.RootDiskSource, tc.Equals, "/test")
	c.Check(*hc.CpuCores, tc.Equals, uint64(4))
	c.Check(*hc.CpuPower, tc.Equals, uint64(75))
	c.Check(*hc.AvailabilityZone, tc.Equals, "az-1")
	c.Check(*hc.VirtType, tc.Equals, "virtual-machine")
}

func (s *stateSuite) TestGetHardwareCharacteristicsWithoutAvailabilityZone(c *tc.C) {
	db := s.DB()
	// Create a reference machine.
	err := s.state.CreateMachine(c.Context(), "42", "", "")
	c.Assert(err, tc.ErrorIsNil)
	var machineUUID machine.UUID
	err = db.QueryRowContext(c.Context(), "SELECT uuid FROM machine WHERE name='42'").Scan(&machineUUID)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetMachineCloudInstance(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)

	hc, err := s.state.HardwareCharacteristics(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*hc.Arch, tc.Equals, "arm64")
	c.Check(*hc.Mem, tc.Equals, uint64(1024))
	c.Check(*hc.RootDisk, tc.Equals, uint64(256))
	c.Check(*hc.RootDiskSource, tc.Equals, "/test")
	c.Check(*hc.CpuCores, tc.Equals, uint64(4))
	c.Check(*hc.CpuPower, tc.Equals, uint64(75))
	c.Check(hc.AvailabilityZone, tc.IsNil)
	c.Check(*hc.VirtType, tc.Equals, "virtual-machine")
}

func (s *stateSuite) TestAvailabilityZoneWithNoMachine(c *tc.C) {
	machineUUID := coremachinetesting.GenUUID(c)

	_, err := s.state.AvailabilityZone(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIs, machineerrors.AvailabilityZoneNotFound)
}

func (s *stateSuite) TestAvailabilityZone(c *tc.C) {
	machineUUID := s.ensureInstance(c, "42")

	az, err := s.state.AvailabilityZone(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(az, tc.Equals, "az-1")
}

func (s *stateSuite) TestSetInstanceData(c *tc.C) {
	db := s.DB()

	// Create a reference machine.
	err := s.state.CreateMachine(c.Context(), "42", "", "")
	c.Assert(err, tc.ErrorIsNil)
	var machineUUID machine.UUID
	row := db.QueryRowContext(c.Context(), "SELECT uuid FROM machine WHERE name='42'")
	c.Assert(row.Err(), tc.ErrorIsNil)
	err = row.Scan(&machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	// Add a reference AZ.
	_, err = db.ExecContext(c.Context(), "INSERT INTO availability_zone VALUES('deadbeef-0bad-400d-8000-4b1d0d06f00d', 'az-1')")
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetMachineCloudInstance(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)

	var instanceData instanceData
	row = db.QueryRowContext(c.Context(), "SELECT * FROM machine_cloud_instance WHERE instance_id='1'")
	c.Assert(row.Err(), tc.ErrorIsNil)
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
	c.Assert(err, tc.ErrorIsNil)
	c.Check(instanceData.MachineUUID, tc.Equals, machineUUID)
	c.Check(instanceData.InstanceID, tc.Equals, "1")
	c.Check(instanceData.DisplayName, tc.Equals, "one")
	c.Check(*instanceData.Arch, tc.Equals, "arm64")
	c.Check(*instanceData.Mem, tc.Equals, uint64(1024))
	c.Check(*instanceData.RootDisk, tc.Equals, uint64(256))
	// Make sure we also handle correctly NULL values.
	c.Check(instanceData.RootDiskSource, tc.IsNil)
	c.Check(*instanceData.CPUCores, tc.Equals, uint64(4))
	c.Check(*instanceData.CPUPower, tc.Equals, uint64(75))
	c.Check(*instanceData.AvailabilityZoneUUID, tc.Equals, "deadbeef-0bad-400d-8000-4b1d0d06f00d")
	c.Check(*instanceData.VirtType, tc.Equals, "virtual-machine")

	rows, err := db.QueryContext(c.Context(), "SELECT tag FROM instance_tag WHERE machine_uuid='"+machineUUID.String()+"'")
	defer func() { _ = rows.Close() }()
	c.Assert(err, tc.ErrorIsNil)
	var instanceTags []string
	for rows.Next() {
		var tag string
		err = rows.Scan(&tag)
		c.Assert(err, tc.ErrorIsNil)
		instanceTags = append(instanceTags, tag)
	}
	c.Check(instanceTags, tc.HasLen, 2)
	c.Check(instanceTags[0], tc.Equals, "tag1")
	c.Check(instanceTags[1], tc.Equals, "tag2")
}

func (s *stateSuite) TestSetInstanceDataAlreadyExists(c *tc.C) {
	db := s.DB()

	// Create a reference machine.
	err := s.state.CreateMachine(c.Context(), "42", "", "")
	c.Assert(err, tc.ErrorIsNil)
	var machineUUID machine.UUID
	row := db.QueryRowContext(c.Context(), "SELECT uuid FROM machine WHERE name='42'")
	c.Assert(row.Err(), tc.ErrorIsNil)
	err = row.Scan(&machineUUID)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetMachineCloudInstance(
		c.Context(),
		machineUUID,
		instance.Id("1"),
		"one",
		&instance.HardwareCharacteristics{
			Arch: ptr("arm64"),
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	// Must fail when we try to add again.
	err = s.state.SetMachineCloudInstance(
		c.Context(),
		machineUUID,
		instance.Id("1"),
		"one",
		&instance.HardwareCharacteristics{
			Arch: ptr("amd64"),
		},
	)
	c.Assert(err, tc.ErrorMatches, "machine cloud instance already exists.*")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineCloudInstanceAlreadyExists)
}

// TestDeleteInstanceData asserts the happy path of DeleteMachineCloudInstance
// at the state layer.
func (s *stateSuite) TestDeleteInstanceData(c *tc.C) {
	db := s.DB()

	machineUUID := s.ensureInstance(c, "42")

	err := s.state.DeleteMachineCloudInstance(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Check that all rows've been deleted.
	rows, err := db.QueryContext(c.Context(), "SELECT * FROM machine_cloud_instance WHERE instance_id='1'")
	defer func() { _ = rows.Close() }()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rows.Err(), tc.ErrorIsNil)
	c.Check(rows.Next(), tc.IsFalse)
	rows, err = db.QueryContext(c.Context(), "SELECT * FROM instance_tag WHERE machine_uuid='"+machineUUID.String()+"'")
	defer func() { _ = rows.Close() }()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rows.Err(), tc.ErrorIsNil)
	c.Check(rows.Next(), tc.IsFalse)
}

// TestDeleteInstanceDataWithStatus asserts that DeleteMachineCloudInstance at
// the state layer removes any instance status and status data when deleting an
// instance.
func (s *stateSuite) TestDeleteInstanceDataWithStatus(c *tc.C) {
	db := s.DB()

	machineUUID := s.ensureInstance(c, "42")

	// Add a status with data for this instance
	s.state.SetInstanceStatus(c.Context(), "42", domainmachine.StatusInfo[domainmachine.InstanceStatusType]{
		Status:  domainmachine.InstanceStatusAllocating,
		Message: "running",
		Data:    []byte(`{"key":"data"}`),
		Since:   ptr(time.Now().UTC()),
	})

	err := s.state.DeleteMachineCloudInstance(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Check that all rows've been deleted from the status tables.
	var status int
	err = db.QueryRowContext(c.Context(), "SELECT count(*) FROM machine_cloud_instance_status WHERE machine_uuid=?", "123").Scan(&status)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(status, tc.Equals, 0)
}

func (s *stateSuite) TestInstanceIdSuccess(c *tc.C) {
	machineUUID := s.ensureInstance(c, "666")

	instanceId, err := s.state.InstanceID(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(instanceId, tc.Equals, "123")
}

func (s *stateSuite) TestInstanceIdError(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "")
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.state.InstanceID(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, machineerrors.NotProvisioned)
}

func (s *stateSuite) TestInstanceNameSuccess(c *tc.C) {
	machineUUID := s.ensureInstance(c, "666")

	instanceID, displayName, err := s.state.InstanceIDAndName(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(instanceID, tc.Equals, "123")
	c.Assert(displayName, tc.Equals, "one-two-three")
}

func (s *stateSuite) TestInstanceNameError(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "")
	c.Assert(err, tc.ErrorIsNil)

	_, _, err = s.state.InstanceIDAndName(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, machineerrors.NotProvisioned)
}

// TestGetInstanceStatusSuccess asserts the happy path of InstanceStatus at the
// state layer.
func (s *stateSuite) TestGetInstanceStatusSuccess(c *tc.C) {
	machineUUID := s.ensureInstance(c, "666")

	// Add a status value for this machine into the
	// machine_cloud_instance_status table using the machineUUID and the status
	// value 3 for "running" (from machine_cloud_instance_status_value table).
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(c.Context(), `
UPDATE machine_cloud_instance_status
SET status_id='3', 
	message='running', 
	updated_at='2024-07-12 12:00:00'
WHERE machine_uuid=?`, machineUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	obtainedStatus, err := s.state.GetInstanceStatus(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)
	expectedStatus := domainmachine.StatusInfo[domainmachine.InstanceStatusType]{
		Status:  domainmachine.InstanceStatusRunning,
		Message: "running",
		Since:   ptr(time.Date(2024, 7, 12, 12, 0, 0, 0, time.UTC)),
	}
	c.Check(obtainedStatus, tc.DeepEquals, expectedStatus)
}

// TestGetInstanceStatusSuccessWithData asserts the happy path of InstanceStatus
// at the state layer.
func (s *stateSuite) TestGetInstanceStatusSuccessWithData(c *tc.C) {
	machineUUID := s.ensureInstance(c, "666")

	// Add a status value for this machine into the
	// machine_cloud_instance_status table using the machineUUID and the status
	// value 2 for "running" (from machine_cloud_instance_status_value table).
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(c.Context(), `
UPDATE machine_cloud_instance_status
SET status_id='3', 
	message='running', 
	data='{"key": "data"}',
	updated_at='2024-07-12 12:00:00'
WHERE machine_uuid=?`, machineUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	obtainedStatus, err := s.state.GetInstanceStatus(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)
	expectedStatus := domainmachine.StatusInfo[domainmachine.InstanceStatusType]{
		Status:  domainmachine.InstanceStatusRunning,
		Message: "running",
		Data:    []byte(`{"key": "data"}`),
		Since:   ptr(time.Date(2024, 7, 12, 12, 0, 0, 0, time.UTC)),
	}
	c.Check(obtainedStatus, tc.DeepEquals, expectedStatus)
}

// TestGetInstanceStatusNotFoundError asserts that GetInstanceStatus returns a
// NotFound error when the given machine cannot be found.
func (s *stateSuite) TestGetInstanceStatusNotFoundError(c *tc.C) {
	_, err := s.state.GetInstanceStatus(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetInstanceStatusStatusNotSetError asserts that GetInstanceStatus returns
// a StatusNotSet error when a status value cannot be found for the given
// machine.
func (s *stateSuite) TestGetInstanceStatusStatusNotSetError(c *tc.C) {
	machineUUID := s.ensureInstance(c, "666")

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(c.Context(), `
DELETE FROM machine_cloud_instance_status
WHERE machine_uuid=?`, machineUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	// Don't add a status value for this instance into the
	// machine_cloud_instance_status table.
	_, err = s.state.GetInstanceStatus(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, machineerrors.StatusNotSet)
}

// TestSetInstanceStatusSuccess asserts the happy path of SetInstanceStatus at
// the state layer.
func (s *stateSuite) TestSetInstanceStatusSuccess(c *tc.C) {
	s.ensureInstance(c, "666")

	expectedStatus := domainmachine.StatusInfo[domainmachine.InstanceStatusType]{
		Status:  domainmachine.InstanceStatusRunning,
		Message: "running",
	}
	err := s.state.SetInstanceStatus(c.Context(), "666", expectedStatus)
	c.Assert(err, tc.ErrorIsNil)

	obtainedStatus, err := s.state.GetInstanceStatus(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedStatus.Status, tc.Equals, expectedStatus.Status)
	c.Assert(obtainedStatus.Message, tc.Equals, expectedStatus.Message)
}

// TestSetInstanceStatusSuccessWithData asserts the happy path of
// SetInstanceStatus at the state layer.
func (s *stateSuite) TestSetInstanceStatusSuccessWithData(c *tc.C) {
	s.ensureInstance(c, "666")

	expectedStatus := domainmachine.StatusInfo[domainmachine.InstanceStatusType]{
		Status:  domainmachine.InstanceStatusRunning,
		Message: "running",
		Data:    []byte(`{"key": "data"}`),
		Since:   ptr(time.Date(2024, 7, 12, 12, 0, 0, 0, time.UTC)),
	}
	err := s.state.SetInstanceStatus(c.Context(), "666", expectedStatus)
	c.Assert(err, tc.ErrorIsNil)

	obtainedStatus, err := s.state.GetInstanceStatus(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedStatus, tc.DeepEquals, expectedStatus)
}

// TestSetInstanceStatusError asserts that SetInstanceStatus returns a NotFound
// error when the given machine cannot be found.
func (s *stateSuite) TestSetInstanceStatusError(c *tc.C) {
	err := s.state.SetInstanceStatus(c.Context(), "666", domainmachine.StatusInfo[domainmachine.InstanceStatusType]{
		Status:  domainmachine.InstanceStatusRunning,
		Message: "running",
	})
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestInstanceStatusValues asserts the keys and values in the
// machine_cloud_instance_status_value table, because we convert between core.status values
// and machine_cloud_instance_status_value based on these associations. This test will catch
// any discrepancies between the two sets of values, and error if/when any of
// them ever change.
func (s *stateSuite) TestInstanceStatusValues(c *tc.C) {
	db := s.DB()

	// Check that the status values in the machine_cloud_instance_status_value table match
	// the instance status values in core status.
	rows, err := db.QueryContext(c.Context(), "SELECT id, status FROM machine_cloud_instance_status_value ORDER BY id")
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()
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
		c.Assert(err, tc.ErrorIsNil)
		statusValues = append(statusValues, statusValue)
	}
	c.Assert(statusValues, tc.HasLen, 5)
	c.Check(statusValues[0].ID, tc.Equals, 0)
	c.Check(statusValues[0].Name, tc.Equals, "unknown")
	c.Check(statusValues[1].ID, tc.Equals, 1)
	c.Check(statusValues[1].Name, tc.Equals, "pending")
	c.Check(statusValues[2].ID, tc.Equals, 2)
	c.Check(statusValues[2].Name, tc.Equals, "allocating")
	c.Check(statusValues[3].ID, tc.Equals, 3)
	c.Check(statusValues[3].Name, tc.Equals, "running")
	c.Check(statusValues[4].ID, tc.Equals, 4)
	c.Check(statusValues[4].Name, tc.Equals, "provisioning error")
}

func (s *stateSuite) ensureInstance(c *tc.C, mName machine.Name) machine.UUID {
	db := s.DB()

	// Create a reference machine.
	machineUUID := coremachinetesting.GenUUID(c)
	err := s.state.CreateMachine(c.Context(), mName, "", machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	// Add a reference AZ.
	_, err = db.ExecContext(c.Context(), "INSERT INTO availability_zone VALUES('deadbeef-0bad-400d-8000-4b1d0d06f00d', 'az-1')")
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetMachineCloudInstance(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)
	return machineUUID
}
