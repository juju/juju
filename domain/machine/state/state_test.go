// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"sort"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/collections/transform"
	"github.com/juju/tc"

	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/domain/life"
	domainmachine "github.com/juju/juju/domain/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ModelSuite

	state *State
}

var _ = tc.Suite(&stateSuite{})

// runQuery executes the provided SQL query string using the current state's database connection.
//
// It is a convenient function to setup test with a specific database state
func (s *stateSuite) runQuery(c *tc.C, query string) error {
	db, err := s.state.DB()
	if err != nil {
		return err
	}
	stmt, err := sqlair.Prepare(query)
	if err != nil {
		return err
	}
	return db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt).Run()
	})
}

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

// TestCreateMachine asserts the happy path of CreateMachine at the state layer.
func (s *stateSuite) TestCreateMachine(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "")
	c.Assert(err, tc.ErrorIsNil)

	var (
		machineName string
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT name FROM machine").Scan(&machineName)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machineName, tc.Equals, "666")
}

// TestCreateMachineAlreadyExists asserts that a MachineAlreadyExists error is
// returned when the machine already exists.
func (s *stateSuite) TestCreateMachineAlreadyExists(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "")
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.CreateMachine(c.Context(), "666", "", "")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineAlreadyExists)
}

// TestCreateMachineWithParentSuccess asserts the happy path of
// CreateMachineWithParent at the state layer.
func (s *stateSuite) TestCreateMachineWithParentSuccess(c *tc.C) {
	// Create the parent first
	err := s.state.CreateMachine(c.Context(), "666", "3", "1")
	c.Assert(err, tc.ErrorIsNil)

	// Create the machine with the created parent
	err = s.state.CreateMachineWithParent(c.Context(), "667", "666", "4", "2")
	c.Assert(err, tc.ErrorIsNil)

	// Make sure the newly created machine with parent has been created.
	var (
		machineName string
	)
	parentStmt := `
SELECT  name
FROM    machine
        LEFT JOIN machine_parent AS parent
	ON        parent.machine_uuid = machine.uuid
WHERE   parent.parent_uuid = 1
	`
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, parentStmt).Scan(&machineName)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machineName, tc.Equals, "667")
}

// TestCreateMachineWithParentNotFound asserts that a NotFound error is returned
// when the parent machine is not found.
func (s *stateSuite) TestCreateMachineWithParentNotFound(c *tc.C) {
	err := s.state.CreateMachineWithParent(c.Context(), "667", "666", "4", "2")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestCreateMachineWithparentAlreadyExists asserts that a MachineAlreadyExists
// error is returned when the machine to be created already exists.
func (s *stateSuite) TestCreateMachineWithParentAlreadyExists(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "")
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.CreateMachineWithParent(c.Context(), "666", "357", "4", "2")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineAlreadyExists)
}

// TestGetMachineParentUUIDGrandParentNotAllowed asserts that a
// GrandParentNotAllowed error is returned when a grandparent is detected for a
// machine.
func (s *stateSuite) TestCreateMachineWithGrandParentNotAllowed(c *tc.C) {
	// Create the parent machine first.
	err := s.state.CreateMachine(c.Context(), "666", "1", "123")
	c.Assert(err, tc.ErrorIsNil)

	// Create the machine with the created parent.
	err = s.state.CreateMachineWithParent(c.Context(), "667", "666", "2", "456")
	c.Assert(err, tc.ErrorIsNil)

	// Create the machine with the created parent.
	err = s.state.CreateMachineWithParent(c.Context(), "668", "667", "3", "789")
	c.Assert(err, tc.ErrorIs, machineerrors.GrandParentNotSupported)
}

// TestDeleteMachine asserts the happy path of DeleteMachine at the state layer.
func (s *stateSuite) TestDeleteMachine(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "")
	c.Assert(err, tc.ErrorIsNil)

	bd := blockdevice.BlockDevice{
		DeviceName:     "name-666",
		Label:          "label-666",
		UUID:           "device-666",
		HardwareId:     "hardware-666",
		WWN:            "wwn-666",
		BusAddress:     "bus-666",
		SizeMiB:        666,
		FilesystemType: "btrfs",
		InUse:          true,
		MountPoint:     "mount-666",
		SerialId:       "serial-666",
	}
	bdUUID := uuid.MustNewUUID().String()
	s.insertBlockDevice(c, bd, bdUUID, "666")

	err = s.state.DeleteMachine(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)

	var machineCount int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT count(*) FROM machine WHERE name=?", "666").Scan(&machineCount)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machineCount, tc.Equals, 0)
}

// TestDeleteMachineStatus asserts that DeleteMachine at the state layer removes
// any machine status and status data when deleting a machine.
func (s *stateSuite) TestDeleteMachineStatus(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "123")
	c.Assert(err, tc.ErrorIsNil)

	bd := blockdevice.BlockDevice{
		DeviceName:     "name-666",
		Label:          "label-666",
		UUID:           "device-666",
		HardwareId:     "hardware-666",
		WWN:            "wwn-666",
		BusAddress:     "bus-666",
		SizeMiB:        666,
		FilesystemType: "btrfs",
		InUse:          true,
		MountPoint:     "mount-666",
		SerialId:       "serial-666",
	}
	bdUUID := uuid.MustNewUUID().String()
	s.insertBlockDevice(c, bd, bdUUID, "666")

	s.state.SetMachineStatus(c.Context(), "666", domainmachine.StatusInfo[domainmachine.MachineStatusType]{
		Status:  domainmachine.MachineStatusStarted,
		Message: "started",
		Data:    []byte(`{"key": "data"}`),
	})

	err = s.state.DeleteMachine(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)

	var status int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT count(*) FROM machine_status WHERE machine_uuid=?", "123").Scan(&status)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(status, tc.Equals, 0)
}

func (s *stateSuite) insertBlockDevice(c *tc.C, bd blockdevice.BlockDevice, blockDeviceUUID, machineId string) {
	db := s.DB()

	inUse := 0
	if bd.InUse {
		inUse = 1
	}
	_, err := db.ExecContext(c.Context(), `
INSERT INTO block_device (uuid, name, label, device_uuid, hardware_id, wwn, bus_address, serial_id, mount_point, filesystem_type_id, Size_mib, in_use, machine_uuid)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 2, ?, ?, (SELECT uuid FROM machine WHERE name=?))
`, blockDeviceUUID, bd.DeviceName, bd.Label, bd.UUID, bd.HardwareId, bd.WWN, bd.BusAddress, bd.SerialId, bd.MountPoint, bd.SizeMiB, inUse, machineId)
	c.Assert(err, tc.ErrorIsNil)

	for _, link := range bd.DeviceLinks {
		_, err = db.ExecContext(c.Context(), `
INSERT INTO block_device_link_device (block_device_uuid, name)
VALUES (?, ?)
`, blockDeviceUUID, link)
		c.Assert(err, tc.ErrorIsNil)
	}
	c.Assert(err, tc.ErrorIsNil)
}

// TestGetMachineLifeSuccess asserts the happy path of GetMachineLife at the
// state layer.
func (s *stateSuite) TestGetMachineLifeSuccess(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "")
	c.Assert(err, tc.ErrorIsNil)

	obtainedLife, err := s.state.GetMachineLife(c.Context(), "666")
	expectedLife := life.Alive
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*obtainedLife, tc.Equals, expectedLife)
}

// TestGetMachineLifeNotFound asserts that a NotFound error is returned when the
// machine is not found.
func (s *stateSuite) TestGetMachineLifeNotFound(c *tc.C) {
	_, err := s.state.GetMachineLife(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestListAllMachines(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "3", "1")
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.CreateMachine(c.Context(), "667", "4", "2")
	c.Assert(err, tc.ErrorIsNil)

	machines, err := s.state.AllMachineNames(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	expectedMachines := []string{"666", "667"}
	ms := transform.Slice[machine.Name, string](machines, func(m machine.Name) string { return m.String() })

	sort.Strings(ms)
	sort.Strings(expectedMachines)
	c.Assert(ms, tc.DeepEquals, expectedMachines)
}

// TestGetMachineStatusSuccess asserts the happy path of GetMachineStatus at the
// state layer.
func (s *stateSuite) TestGetMachineStatusSuccess(c *tc.C) {
	db := s.DB()

	err := s.state.CreateMachine(c.Context(), "666", "", "123")
	c.Assert(err, tc.ErrorIsNil)

	// Add a status value for this machine into the
	// machine_status table using the machineUUID and the status
	// value 2 for "running" (from machine_cloud_instance_status_value table).
	_, err = db.ExecContext(c.Context(), "INSERT INTO machine_status VALUES('123', '1', 'started', NULL, '2024-07-12 12:00:00')")
	c.Assert(err, tc.ErrorIsNil)

	obtainedStatus, err := s.state.GetMachineStatus(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedStatus, tc.DeepEquals, domainmachine.StatusInfo[domainmachine.MachineStatusType]{
		Status:  domainmachine.MachineStatusStarted,
		Message: "started",
		Since:   ptr(time.Date(2024, 7, 12, 12, 0, 0, 0, time.UTC)),
	})
}

// TestGetMachineStatusWithData asserts the happy path of GetMachineStatus at
// the state layer.
func (s *stateSuite) TestGetMachineStatusSuccessWithData(c *tc.C) {
	db := s.DB()

	err := s.state.CreateMachine(c.Context(), "666", "", "123")
	c.Assert(err, tc.ErrorIsNil)

	// Add a status value for this machine into the
	// machine_status table using the machineUUID and the status
	// value 2 for "running" (from machine_cloud_instance_status_value table).
	_, err = db.ExecContext(c.Context(), `INSERT INTO machine_status VALUES('123', '1', 'started', '{"key":"data"}',  '2024-07-12 12:00:00')`)
	c.Assert(err, tc.ErrorIsNil)

	obtainedStatus, err := s.state.GetMachineStatus(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedStatus, tc.DeepEquals, domainmachine.StatusInfo[domainmachine.MachineStatusType]{
		Status:  domainmachine.MachineStatusStarted,
		Message: "started",
		Data:    []byte(`{"key":"data"}`),
		Since:   ptr(time.Date(2024, 7, 12, 12, 0, 0, 0, time.UTC)),
	})
}

// TestGetMachineStatusNotFoundError asserts that a NotFound error is returned
// when the machine is not found.
func (s *stateSuite) TestGetMachineStatusNotFoundError(c *tc.C) {
	_, err := s.state.GetMachineStatus(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetMachineStatusNotSetError asserts that a StatusNotSet error is returned
// when the status is not set.
func (s *stateSuite) TestGetMachineStatusNotSetError(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "123")
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.state.GetMachineStatus(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, machineerrors.StatusNotSet)
}

// TestSetMachineStatusSuccess asserts the happy path of SetMachineStatus at the
// state layer.
func (s *stateSuite) TestSetMachineStatusSuccess(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "123")
	c.Assert(err, tc.ErrorIsNil)

	expectedStatus := domainmachine.StatusInfo[domainmachine.MachineStatusType]{
		Status:  domainmachine.MachineStatusStarted,
		Message: "started",
		Since:   ptr(time.Now().UTC()),
	}
	err = s.state.SetMachineStatus(c.Context(), "666", expectedStatus)
	c.Assert(err, tc.ErrorIsNil)

	obtainedStatus, err := s.state.GetMachineStatus(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedStatus, tc.DeepEquals, expectedStatus)
}

// TestSetMachineStatusSuccessWithData asserts the happy path of
// SetMachineStatus at the state layer.
func (s *stateSuite) TestSetMachineStatusSuccessWithData(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "123")
	c.Assert(err, tc.ErrorIsNil)

	expectedStatus := domainmachine.StatusInfo[domainmachine.MachineStatusType]{
		Status:  domainmachine.MachineStatusStarted,
		Message: "started",
		Data:    []byte(`{"key": "data"}`),
		Since:   ptr(time.Now().UTC()),
	}
	err = s.state.SetMachineStatus(c.Context(), "666", expectedStatus)
	c.Assert(err, tc.ErrorIsNil)

	obtainedStatus, err := s.state.GetMachineStatus(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedStatus, tc.DeepEquals, expectedStatus)
}

// TestSetMachineStatusNotFoundError asserts that a NotFound error is returned
// when the machine is not found.
func (s *stateSuite) TestSetMachineStatusNotFoundError(c *tc.C) {
	err := s.state.SetMachineStatus(c.Context(), "666", domainmachine.StatusInfo[domainmachine.MachineStatusType]{
		Status: domainmachine.MachineStatusStarted,
	})
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestMachineStatusValues asserts the keys and values in the
// machine_status_value table, because we convert between core.status values
// and machine_status_value based on these associations. This test will catch
// any discrepancies between the two sets of values, and error if/when any of
// them ever change.
func (s *stateSuite) TestMachineStatusValues(c *tc.C) {
	db := s.DB()

	// Check that the status values in the machine_status_value table match
	// the instance status values in core status.
	rows, err := db.QueryContext(c.Context(), "SELECT id, status FROM machine_status_value")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()
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
	c.Check(statusValues[0].Name, tc.Equals, "error")
	c.Check(statusValues[1].ID, tc.Equals, 1)
	c.Check(statusValues[1].Name, tc.Equals, "started")
	c.Check(statusValues[2].ID, tc.Equals, 2)
	c.Check(statusValues[2].Name, tc.Equals, "pending")
	c.Check(statusValues[3].ID, tc.Equals, 3)
	c.Check(statusValues[3].Name, tc.Equals, "stopped")
	c.Check(statusValues[4].ID, tc.Equals, 4)
	c.Check(statusValues[4].Name, tc.Equals, "down")
}

// TestMachineStatusValuesConversion asserts the conversions to and from the
// core status values and the internal status values for machine stay intact.
func (s *stateSuite) TestMachineStatusValuesConversion(c *tc.C) {
	tests := []struct {
		statusValue string
		expected    int
	}{
		{statusValue: "error", expected: 0},
		{statusValue: "started", expected: 1},
		{statusValue: "pending", expected: 2},
		{statusValue: "stopped", expected: 3},
		{statusValue: "down", expected: 4},
	}

	for _, test := range tests {
		a, err := decodeMachineStatus(test.statusValue)
		c.Assert(err, tc.ErrorIsNil)
		b, err := encodeMachineStatus(a)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(b, tc.Equals, test.expected)
	}
}

// TestInstanceStatusValuesConversion asserts the conversions to and from the
// core status values and the internal status values for instances stay intact.
func (s *stateSuite) TestInstanceStatusValuesConversion(c *tc.C) {
	tests := []struct {
		statusValue string
		expected    int
	}{
		{statusValue: "", expected: 0},
		{statusValue: "unknown", expected: 0},
		{statusValue: "allocating", expected: 1},
		{statusValue: "running", expected: 2},
		{statusValue: "provisioning error", expected: 3},
	}

	for _, test := range tests {
		a, err := decodeCloudInstanceStatus(test.statusValue)
		c.Assert(err, tc.ErrorIsNil)

		b, err := encodeCloudInstanceStatus(a)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(b, tc.Equals, test.expected)
	}
}

// TestSetMachineLifeSuccess asserts the happy path of SetMachineLife at the
// state layer.
func (s *stateSuite) TestSetMachineLifeSuccess(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "")
	c.Assert(err, tc.ErrorIsNil)

	// Assert the life status is initially Alive
	obtainedLife, err := s.state.GetMachineLife(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*obtainedLife, tc.Equals, life.Alive)

	// Set the machine's life to Dead
	err = s.state.SetMachineLife(c.Context(), "666", life.Dead)
	c.Assert(err, tc.ErrorIsNil)

	// Assert we get the Dead as the machine's new life status.
	obtainedLife, err = s.state.GetMachineLife(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*obtainedLife, tc.Equals, life.Dead)
}

// TestSetMachineLifeNotFoundError asserts that we get a NotFound if the
// provided machine doesn't exist.
func (s *stateSuite) TestSetMachineLifeNotFoundError(c *tc.C) {
	err := s.state.SetMachineLife(c.Context(), "666", life.Dead)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestListAllMachinesEmpty asserts that AllMachineNames returns an empty list
// if there are no machines.
func (s *stateSuite) TestListAllMachinesEmpty(c *tc.C) {
	machines, err := s.state.AllMachineNames(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, 0)
}

// TestListAllMachineNamesSuccess asserts the happy path of AllMachineNames at
// the state layer.
func (s *stateSuite) TestListAllMachineNamesSuccess(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "3", "1")
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.CreateMachine(c.Context(), "667", "4", "2")
	c.Assert(err, tc.ErrorIsNil)

	machines, err := s.state.AllMachineNames(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	expectedMachines := []string{"666", "667"}
	ms := transform.Slice[machine.Name, string](machines, func(m machine.Name) string { return m.String() })

	sort.Strings(ms)
	sort.Strings(expectedMachines)
	c.Assert(ms, tc.DeepEquals, expectedMachines)
}

// TestIsControllerSuccess asserts the happy path of IsController at the state
// layer.
func (s *stateSuite) TestIsControllerSuccess(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "")
	c.Assert(err, tc.ErrorIsNil)

	isController, err := s.state.IsMachineController(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isController, tc.Equals, false)

	db := s.DB()

	updateIsController := `
UPDATE machine
SET is_controller = TRUE
WHERE name = $1;
`
	_, err = db.ExecContext(c.Context(), updateIsController, "666")
	c.Assert(err, tc.ErrorIsNil)
	isController, err = s.state.IsMachineController(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isController, tc.Equals, true)
}

// TestIsControllerNotFound asserts that a NotFound error is returned when the
// machine is not found.
func (s *stateSuite) TestIsControllerNotFound(c *tc.C) {
	_, err := s.state.IsMachineController(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetMachineParentUUIDSuccess asserts the happy path of
// GetMachineParentUUID at the state layer.
func (s *stateSuite) TestGetMachineParentUUIDSuccess(c *tc.C) {
	// Create the parent machine first.
	err := s.state.CreateMachine(c.Context(), "666", "1", "123")
	c.Assert(err, tc.ErrorIsNil)

	// Create the machine with the created parent.
	err = s.state.CreateMachineWithParent(c.Context(), "667", "666", "2", "456")
	c.Assert(err, tc.ErrorIsNil)

	// Get the parent UUID of the machine.
	parentUUID, err := s.state.GetMachineParentUUID(c.Context(), "456")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(parentUUID, tc.Equals, machine.UUID("123"))
}

// TestGetMachineParentUUIDNotFound asserts that a NotFound error is returned
// when the machine is not found.
func (s *stateSuite) TestGetMachineParentUUIDNotFound(c *tc.C) {
	_, err := s.state.GetMachineParentUUID(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetMachineParentUUIDNoParent asserts that a NotFound error is returned
// when the machine has no parent.
func (s *stateSuite) TestGetMachineParentUUIDNoParent(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "123")
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.state.GetMachineParentUUID(c.Context(), "123")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineHasNoParent)
}

// TestMarkMachineForRemovalSuccess asserts the happy path of
// MarkMachineForRemoval at the state layer.
func (s *stateSuite) TestMarkMachineForRemovalSuccess(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "123")
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.MarkMachineForRemoval(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)

	var machineUUID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT machine_uuid FROM machine_removals WHERE machine_uuid=?", "123").Scan(&machineUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machineUUID, tc.Equals, "123")
}

// TestMarkMachineForRemovalSuccessIdempotent asserts that marking a machine for
// removal multiple times is idempotent.
func (s *stateSuite) TestMarkMachineForRemovalSuccessIdempotent(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "123")
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.MarkMachineForRemoval(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.MarkMachineForRemoval(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)

	machines, err := s.state.GetAllMachineRemovals(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, 1)
	c.Assert(machines[0], tc.Equals, machine.UUID("123"))
}

// TestMarkMachineForRemovalNotFound asserts that a NotFound error is returned
// when the machine is not found.
// TODO(cderici): use machineerrors.MachineNotFound on rebase after #17759
// lands.
func (s *stateSuite) TestMarkMachineForRemovalNotFound(c *tc.C) {
	err := s.state.MarkMachineForRemoval(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetAllMachineRemovalsSuccess asserts the happy path of
// GetAllMachineRemovals at the state layer.
func (s *stateSuite) TestGetAllMachineRemovalsSuccess(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "123")
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.MarkMachineForRemoval(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)

	machines, err := s.state.GetAllMachineRemovals(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, 1)
	c.Assert(machines[0], tc.Equals, machine.UUID("123"))
}

// TestGetAllMachineRemovalsEmpty asserts that GetAllMachineRemovals returns an
// empty list if there are no machines marked for removal.
func (s *stateSuite) TestGetAllMachineRemovalsEmpty(c *tc.C) {
	machines, err := s.state.GetAllMachineRemovals(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, 0)
}

// TestGetSomeMachineRemovals asserts the happy path of GetAllMachineRemovals at
// the state layer for a subset of machines.
func (s *stateSuite) TestGetSomeMachineRemovals(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "1", "123")
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.CreateMachine(c.Context(), "667", "2", "124")
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.CreateMachine(c.Context(), "668", "3", "125")
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.MarkMachineForRemoval(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.MarkMachineForRemoval(c.Context(), "668")
	c.Assert(err, tc.ErrorIsNil)

	machines, err := s.state.GetAllMachineRemovals(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, 2)
	c.Assert(machines[0], tc.Equals, machine.UUID("123"))
	c.Assert(machines[1], tc.Equals, machine.UUID("125"))
}

// TestGetMachineUUIDNotFound asserts that a NotFound error is returned
// when the machine is not found.
func (s *stateSuite) TestGetMachineUUIDNotFound(c *tc.C) {
	_, err := s.state.GetMachineUUID(c.Context(), "none")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetMachineUUID asserts that the uuid is returned from a machine name
func (s *stateSuite) TestGetMachineUUID(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "rage", "", "123")
	c.Assert(err, tc.ErrorIsNil)

	name, err := s.state.GetMachineUUID(c.Context(), "rage")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(name, tc.Equals, machine.UUID("123"))
}

func (s *stateSuite) TestKeepInstance(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "")
	c.Assert(err, tc.ErrorIsNil)

	isController, err := s.state.ShouldKeepInstance(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isController, tc.Equals, false)

	db := s.DB()

	updateIsController := `
UPDATE machine
SET    keep_instance = TRUE
WHERE  name = $1`
	_, err = db.ExecContext(c.Context(), updateIsController, "666")
	c.Assert(err, tc.ErrorIsNil)
	isController, err = s.state.ShouldKeepInstance(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isController, tc.Equals, true)
}

// TestIsControllerNotFound asserts that a NotFound error is returned when the
// machine is not found.
func (s *stateSuite) TestKeepInstanceNotFound(c *tc.C) {
	_, err := s.state.ShouldKeepInstance(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestSetKeepInstance(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "")
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetKeepInstance(c.Context(), "666", true)
	c.Assert(err, tc.ErrorIsNil)

	db := s.DB()
	query := `
SELECT keep_instance
FROM   machine
WHERE  name = $1`
	row := db.QueryRowContext(c.Context(), query, "666")
	c.Assert(row.Err(), tc.ErrorIsNil)

	var keep bool
	c.Assert(row.Scan(&keep), tc.ErrorIsNil)
	c.Check(keep, tc.IsTrue)

}

func (s *stateSuite) TestSetKeepInstanceNotFound(c *tc.C) {
	err := s.state.SetKeepInstance(c.Context(), "666", true)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestSetAppliedLXDProfileNames(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "deadbeef")
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetMachineCloudInstance(c.Context(), "deadbeef", instance.Id("123"), "", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetAppliedLXDProfileNames(c.Context(), "deadbeef", []string{"profile1", "profile2"})
	c.Assert(err, tc.ErrorIsNil)

	// Check that the profile names are in the machine_lxd_profile table.
	db := s.DB()
	rows, err := db.Query("SELECT name FROM machine_lxd_profile WHERE machine_uuid = 'deadbeef'")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()
	var profiles []string
	for rows.Next() {
		var profile string
		err := rows.Scan(&profile)
		c.Assert(err, tc.ErrorIsNil)
		profiles = append(profiles, profile)
	}
	c.Check(profiles, tc.DeepEquals, []string{"profile1", "profile2"})
}

func (s *stateSuite) TestSetLXDProfilesPartial(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "deadbeef")
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetMachineCloudInstance(c.Context(), "deadbeef", instance.Id("123"), "", nil)
	c.Assert(err, tc.ErrorIsNil)

	// Insert a single lxd profile.
	db := s.DB()
	_, err = db.Exec(`INSERT INTO machine_lxd_profile VALUES
("deadbeef", "profile1", 0)`)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetAppliedLXDProfileNames(c.Context(), "deadbeef", []string{"profile1", "profile2"})
	// This shouldn't fail, but add the missing profile to the table.
	c.Assert(err, tc.ErrorIsNil)

	// Check that the profile names are in the machine_lxd_profile table.
	rows, err := db.Query("SELECT name FROM machine_lxd_profile WHERE machine_uuid = 'deadbeef'")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()
	var profiles []string
	for rows.Next() {
		var profile string
		err := rows.Scan(&profile)
		c.Assert(err, tc.ErrorIsNil)
		profiles = append(profiles, profile)
	}
	c.Check(profiles, tc.DeepEquals, []string{"profile1", "profile2"})
}

func (s *stateSuite) TestSetLXDProfilesOverwriteAll(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "deadbeef")
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetMachineCloudInstance(c.Context(), "deadbeef", instance.Id("123"), "", nil)
	c.Assert(err, tc.ErrorIsNil)

	// Insert 3 lxd profiles.
	db := s.DB()
	_, err = db.Exec(`INSERT INTO machine_lxd_profile VALUES
("deadbeef", "profile1", 0), ("deadbeef", "profile2", 1), ("deadbeef", "profile3", 2)`)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetAppliedLXDProfileNames(c.Context(), "deadbeef", []string{"profile1", "profile4"})
	c.Assert(err, tc.ErrorIsNil)

	// Check that the profile names are in the machine_lxd_profile table.
	rows, err := db.Query("SELECT name FROM machine_lxd_profile WHERE machine_uuid = 'deadbeef'")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()
	var profiles []string
	for rows.Next() {
		var profile string
		err := rows.Scan(&profile)
		c.Assert(err, tc.ErrorIsNil)
		profiles = append(profiles, profile)
	}
	c.Check(profiles, tc.DeepEquals, []string{"profile1", "profile4"})
}

func (s *stateSuite) TestSetLXDProfilesSameOrder(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "deadbeef")
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetMachineCloudInstance(c.Context(), "deadbeef", instance.Id("123"), "", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetAppliedLXDProfileNames(c.Context(), "deadbeef", []string{"profile3", "profile1", "profile2"})
	c.Assert(err, tc.ErrorIsNil)

	profiles, err := s.state.AppliedLXDProfileNames(c.Context(), "deadbeef")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(profiles, tc.DeepEquals, []string{"profile3", "profile1", "profile2"})
}

func (s *stateSuite) TestSetLXDProfilesNotFound(c *tc.C) {
	err := s.state.SetAppliedLXDProfileNames(c.Context(), "666", []string{"profile1", "profile2"})
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestSetLXDProfilesNotProvisioned(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "deadbeef")
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetAppliedLXDProfileNames(c.Context(), "deadbeef", []string{"profile3", "profile1", "profile2"})
	c.Assert(err, tc.ErrorIs, machineerrors.NotProvisioned)
}

func (s *stateSuite) TestSetLXDProfilesEmpty(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "deadbeef")
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetMachineCloudInstance(c.Context(), "deadbeef", instance.Id("123"), "", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetAppliedLXDProfileNames(c.Context(), "deadbeef", []string{})
	c.Assert(err, tc.ErrorIsNil)

	profiles, err := s.state.AppliedLXDProfileNames(c.Context(), "deadbeef")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(profiles, tc.HasLen, 0)
}

func (s *stateSuite) TestAppliedLXDProfileNames(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "deadbeef")
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetMachineCloudInstance(c.Context(), "deadbeef", instance.Id("123"), "", nil)
	c.Assert(err, tc.ErrorIsNil)

	// Insert 2 lxd profiles.
	db := s.DB()
	_, err = db.Exec(`INSERT INTO machine_lxd_profile VALUES
("deadbeef", "profile1", 0), ("deadbeef", "profile2", 1)`)
	c.Assert(err, tc.ErrorIsNil)

	profiles, err := s.state.AppliedLXDProfileNames(c.Context(), "deadbeef")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(profiles, tc.DeepEquals, []string{"profile1", "profile2"})
}

func (s *stateSuite) TestAppliedLXDProfileNamesNotProvisioned(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "deadbeef")
	c.Assert(err, tc.ErrorIsNil)
	profiles, err := s.state.AppliedLXDProfileNames(c.Context(), "deadbeef")
	c.Assert(err, tc.ErrorIs, machineerrors.NotProvisioned)
	c.Check(profiles, tc.HasLen, 0)
}

func (s *stateSuite) TestAppliedLXDProfileNamesNoErrorEmpty(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "deadbeef")
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetMachineCloudInstance(c.Context(), "deadbeef", instance.Id("123"), "", nil)
	c.Assert(err, tc.ErrorIsNil)
	profiles, err := s.state.AppliedLXDProfileNames(c.Context(), "deadbeef")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(profiles, tc.HasLen, 0)
}

func (s *stateSuite) TestGetNamesForUUIDs(c *tc.C) {
	// Arrange
	uuid111 := testing.GenUUID(c)
	err := s.state.CreateMachine(c.Context(), "111", "1", uuid111)
	c.Assert(err, tc.ErrorIsNil)
	uuid222 := testing.GenUUID(c)
	err = s.state.CreateMachine(c.Context(), "222", "2", uuid222)
	c.Assert(err, tc.ErrorIsNil)
	uuid333 := testing.GenUUID(c)
	err = s.state.CreateMachine(c.Context(), "333", "3", uuid333)
	c.Assert(err, tc.ErrorIsNil)
	expected := map[string]machine.Name{
		uuid111.String(): "111",
		uuid333.String(): "333",
	}

	// Act
	obtained, err := s.state.GetNamesForUUIDs(c.Context(), []string{uuid333.String(), uuid111.String()})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained, tc.DeepEquals, expected)
}

func (s *stateSuite) TestGetNamesForUUIDsNotFound(c *tc.C) {
	// Act
	_, err := s.state.GetNamesForUUIDs(c.Context(), []string{"deadbeef"})

	// Assert
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}
