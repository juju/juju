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
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	corearch "github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/blockdevice"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	jujuversion "github.com/juju/juju/core/version"
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

var _ = gc.Suite(&stateSuite{})

// runQuery executes the provided SQL query string using the current state's database connection.
//
// It is a convenient function to setup test with a specific database state
func (s *stateSuite) runQuery(query string) error {
	db, err := s.state.DB()
	if err != nil {
		return err
	}
	stmt, err := sqlair.Prepare(query)
	if err != nil {
		return err
	}
	return db.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt).Run()
	})
}

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

// TestCreateMachine asserts the happy path of CreateMachine at the state layer.
func (s *stateSuite) TestCreateMachine(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "")
	c.Assert(err, jc.ErrorIsNil)

	var (
		machineName string
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT name FROM machine").Scan(&machineName)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineName, gc.Equals, "666")
}

// TestCreateMachineAlreadyExists asserts that a MachineAlreadyExists error is
// returned when the machine already exists.
func (s *stateSuite) TestCreateMachineAlreadyExists(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.CreateMachine(context.Background(), "666", "", "")
	c.Assert(err, jc.ErrorIs, machineerrors.MachineAlreadyExists)
}

// TestCreateMachineWithParentSuccess asserts the happy path of
// CreateMachineWithParent at the state layer.
func (s *stateSuite) TestCreateMachineWithParentSuccess(c *gc.C) {
	// Create the parent first
	err := s.state.CreateMachine(context.Background(), "666", "3", "1")
	c.Assert(err, jc.ErrorIsNil)

	// Create the machine with the created parent
	err = s.state.CreateMachineWithParent(context.Background(), "667", "666", "4", "2")
	c.Assert(err, jc.ErrorIsNil)

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
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, parentStmt).Scan(&machineName)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineName, gc.Equals, "667")
}

// TestCreateMachineWithParentNotFound asserts that a NotFound error is returned
// when the parent machine is not found.
func (s *stateSuite) TestCreateMachineWithParentNotFound(c *gc.C) {
	err := s.state.CreateMachineWithParent(context.Background(), "667", "666", "4", "2")
	c.Assert(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestCreateMachineWithparentAlreadyExists asserts that a MachineAlreadyExists
// error is returned when the machine to be created already exists.
func (s *stateSuite) TestCreateMachineWithParentAlreadyExists(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.CreateMachineWithParent(context.Background(), "666", "357", "4", "2")
	c.Assert(err, jc.ErrorIs, machineerrors.MachineAlreadyExists)
}

// TestGetMachineParentUUIDGrandParentNotAllowed asserts that a
// GrandParentNotAllowed error is returned when a grandparent is detected for a
// machine.
func (s *stateSuite) TestCreateMachineWithGrandParentNotAllowed(c *gc.C) {
	// Create the parent machine first.
	err := s.state.CreateMachine(context.Background(), "666", "1", "123")
	c.Assert(err, jc.ErrorIsNil)

	// Create the machine with the created parent.
	err = s.state.CreateMachineWithParent(context.Background(), "667", "666", "2", "456")
	c.Assert(err, jc.ErrorIsNil)

	// Create the machine with the created parent.
	err = s.state.CreateMachineWithParent(context.Background(), "668", "667", "3", "789")
	c.Assert(err, jc.ErrorIs, machineerrors.GrandParentNotSupported)
}

// TestDeleteMachine asserts the happy path of DeleteMachine at the state layer.
func (s *stateSuite) TestDeleteMachine(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "")
	c.Assert(err, jc.ErrorIsNil)

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

	err = s.state.DeleteMachine(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)

	var machineCount int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT count(*) FROM machine WHERE name=?", "666").Scan(&machineCount)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineCount, gc.Equals, 0)
}

// TestDeleteMachineStatus asserts that DeleteMachine at the state layer removes
// any machine status and status data when deleting a machine.
func (s *stateSuite) TestDeleteMachineStatus(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "123")
	c.Assert(err, jc.ErrorIsNil)

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

	s.state.SetMachineStatus(context.Background(), "666", domainmachine.StatusInfo[domainmachine.MachineStatusType]{
		Status:  domainmachine.MachineStatusStarted,
		Message: "started",
		Data:    []byte(`{"key": "data"}`),
	})

	err = s.state.DeleteMachine(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)

	var status int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT count(*) FROM machine_status WHERE machine_uuid=?", "123").Scan(&status)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, 0)
}

func (s *stateSuite) insertBlockDevice(c *gc.C, bd blockdevice.BlockDevice, blockDeviceUUID, machineId string) {
	db := s.DB()

	inUse := 0
	if bd.InUse {
		inUse = 1
	}
	_, err := db.ExecContext(context.Background(), `
INSERT INTO block_device (uuid, name, label, device_uuid, hardware_id, wwn, bus_address, serial_id, mount_point, filesystem_type_id, Size_mib, in_use, machine_uuid)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 2, ?, ?, (SELECT uuid FROM machine WHERE name=?))
`, blockDeviceUUID, bd.DeviceName, bd.Label, bd.UUID, bd.HardwareId, bd.WWN, bd.BusAddress, bd.SerialId, bd.MountPoint, bd.SizeMiB, inUse, machineId)
	c.Assert(err, jc.ErrorIsNil)

	for _, link := range bd.DeviceLinks {
		_, err = db.ExecContext(context.Background(), `
INSERT INTO block_device_link_device (block_device_uuid, name)
VALUES (?, ?)
`, blockDeviceUUID, link)
		c.Assert(err, jc.ErrorIsNil)
	}
	c.Assert(err, jc.ErrorIsNil)
}

// TestGetMachineLifeSuccess asserts the happy path of GetMachineLife at the
// state layer.
func (s *stateSuite) TestGetMachineLifeSuccess(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "")
	c.Assert(err, jc.ErrorIsNil)

	obtainedLife, err := s.state.GetMachineLife(context.Background(), "666")
	expectedLife := life.Alive
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*obtainedLife, gc.Equals, expectedLife)
}

// TestGetMachineLifeNotFound asserts that a NotFound error is returned when the
// machine is not found.
func (s *stateSuite) TestGetMachineLifeNotFound(c *gc.C) {
	_, err := s.state.GetMachineLife(context.Background(), "666")
	c.Assert(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestListAllMachines(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "3", "1")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.CreateMachine(context.Background(), "667", "4", "2")
	c.Assert(err, jc.ErrorIsNil)

	machines, err := s.state.AllMachineNames(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	expectedMachines := []string{"666", "667"}
	ms := transform.Slice[machine.Name, string](machines, func(m machine.Name) string { return m.String() })

	sort.Strings(ms)
	sort.Strings(expectedMachines)
	c.Assert(ms, gc.DeepEquals, expectedMachines)
}

// TestGetMachineStatusSuccess asserts the happy path of GetMachineStatus at the
// state layer.
func (s *stateSuite) TestGetMachineStatusSuccess(c *gc.C) {
	db := s.DB()

	err := s.state.CreateMachine(context.Background(), "666", "", "123")
	c.Assert(err, jc.ErrorIsNil)

	// Add a status value for this machine into the
	// machine_status table using the machineUUID and the status
	// value 2 for "running" (from machine_cloud_instance_status_value table).
	_, err = db.ExecContext(context.Background(), "INSERT INTO machine_status VALUES('123', '1', 'started', NULL, '2024-07-12 12:00:00')")
	c.Assert(err, jc.ErrorIsNil)

	obtainedStatus, err := s.state.GetMachineStatus(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtainedStatus, gc.DeepEquals, domainmachine.StatusInfo[domainmachine.MachineStatusType]{
		Status:  domainmachine.MachineStatusStarted,
		Message: "started",
		Since:   ptr(time.Date(2024, 7, 12, 12, 0, 0, 0, time.UTC)),
	})
}

// TestGetMachineStatusWithData asserts the happy path of GetMachineStatus at
// the state layer.
func (s *stateSuite) TestGetMachineStatusSuccessWithData(c *gc.C) {
	db := s.DB()

	err := s.state.CreateMachine(context.Background(), "666", "", "123")
	c.Assert(err, jc.ErrorIsNil)

	// Add a status value for this machine into the
	// machine_status table using the machineUUID and the status
	// value 2 for "running" (from machine_cloud_instance_status_value table).
	_, err = db.ExecContext(context.Background(), `INSERT INTO machine_status VALUES('123', '1', 'started', '{"key":"data"}',  '2024-07-12 12:00:00')`)
	c.Assert(err, jc.ErrorIsNil)

	obtainedStatus, err := s.state.GetMachineStatus(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtainedStatus, gc.DeepEquals, domainmachine.StatusInfo[domainmachine.MachineStatusType]{
		Status:  domainmachine.MachineStatusStarted,
		Message: "started",
		Data:    []byte(`{"key":"data"}`),
		Since:   ptr(time.Date(2024, 7, 12, 12, 0, 0, 0, time.UTC)),
	})
}

// TestGetMachineStatusNotFoundError asserts that a NotFound error is returned
// when the machine is not found.
func (s *stateSuite) TestGetMachineStatusNotFoundError(c *gc.C) {
	_, err := s.state.GetMachineStatus(context.Background(), "666")
	c.Assert(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetMachineStatusNotSetError asserts that a StatusNotSet error is returned
// when the status is not set.
func (s *stateSuite) TestGetMachineStatusNotSetError(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "123")
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.state.GetMachineStatus(context.Background(), "666")
	c.Assert(err, jc.ErrorIs, machineerrors.StatusNotSet)
}

// TestSetMachineStatusSuccess asserts the happy path of SetMachineStatus at the
// state layer.
func (s *stateSuite) TestSetMachineStatusSuccess(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "123")
	c.Assert(err, jc.ErrorIsNil)

	expectedStatus := domainmachine.StatusInfo[domainmachine.MachineStatusType]{
		Status:  domainmachine.MachineStatusStarted,
		Message: "started",
		Since:   ptr(time.Now().UTC()),
	}
	err = s.state.SetMachineStatus(context.Background(), "666", expectedStatus)
	c.Assert(err, jc.ErrorIsNil)

	obtainedStatus, err := s.state.GetMachineStatus(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtainedStatus, gc.DeepEquals, expectedStatus)
}

// TestSetMachineStatusSuccessWithData asserts the happy path of
// SetMachineStatus at the state layer.
func (s *stateSuite) TestSetMachineStatusSuccessWithData(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "123")
	c.Assert(err, jc.ErrorIsNil)

	expectedStatus := domainmachine.StatusInfo[domainmachine.MachineStatusType]{
		Status:  domainmachine.MachineStatusStarted,
		Message: "started",
		Data:    []byte(`{"key": "data"}`),
		Since:   ptr(time.Now().UTC()),
	}
	err = s.state.SetMachineStatus(context.Background(), "666", expectedStatus)
	c.Assert(err, jc.ErrorIsNil)

	obtainedStatus, err := s.state.GetMachineStatus(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtainedStatus, gc.DeepEquals, expectedStatus)
}

// TestSetMachineStatusNotFoundError asserts that a NotFound error is returned
// when the machine is not found.
func (s *stateSuite) TestSetMachineStatusNotFoundError(c *gc.C) {
	err := s.state.SetMachineStatus(context.Background(), "666", domainmachine.StatusInfo[domainmachine.MachineStatusType]{
		Status: domainmachine.MachineStatusStarted,
	})
	c.Assert(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestMachineStatusValues asserts the keys and values in the
// machine_status_value table, because we convert between core.status values
// and machine_status_value based on these associations. This test will catch
// any discrepancies between the two sets of values, and error if/when any of
// them ever change.
func (s *stateSuite) TestMachineStatusValues(c *gc.C) {
	db := s.DB()

	// Check that the status values in the machine_status_value table match
	// the instance status values in core status.
	rows, err := db.QueryContext(context.Background(), "SELECT id, status FROM machine_status_value")
	defer rows.Close()
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
	c.Assert(statusValues, gc.HasLen, 5)
	c.Check(statusValues[0].ID, gc.Equals, 0)
	c.Check(statusValues[0].Name, gc.Equals, "error")
	c.Check(statusValues[1].ID, gc.Equals, 1)
	c.Check(statusValues[1].Name, gc.Equals, "started")
	c.Check(statusValues[2].ID, gc.Equals, 2)
	c.Check(statusValues[2].Name, gc.Equals, "pending")
	c.Check(statusValues[3].ID, gc.Equals, 3)
	c.Check(statusValues[3].Name, gc.Equals, "stopped")
	c.Check(statusValues[4].ID, gc.Equals, 4)
	c.Check(statusValues[4].Name, gc.Equals, "down")
}

// TestMachineStatusValuesConversion asserts the conversions to and from the
// core status values and the internal status values for machine stay intact.
func (s *stateSuite) TestMachineStatusValuesConversion(c *gc.C) {
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
		c.Assert(err, jc.ErrorIsNil)
		b, err := encodeMachineStatus(a)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(b, gc.Equals, test.expected)
	}
}

// TestInstanceStatusValuesConversion asserts the conversions to and from the
// core status values and the internal status values for instances stay intact.
func (s *stateSuite) TestInstanceStatusValuesConversion(c *gc.C) {
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
		c.Assert(err, jc.ErrorIsNil)

		b, err := encodeCloudInstanceStatus(a)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(b, gc.Equals, test.expected)
	}
}

// TestSetMachineLifeSuccess asserts the happy path of SetMachineLife at the
// state layer.
func (s *stateSuite) TestSetMachineLifeSuccess(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "")
	c.Assert(err, jc.ErrorIsNil)

	// Assert the life status is initially Alive
	obtainedLife, err := s.state.GetMachineLife(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*obtainedLife, gc.Equals, life.Alive)

	// Set the machine's life to Dead
	err = s.state.SetMachineLife(context.Background(), "666", life.Dead)
	c.Assert(err, jc.ErrorIsNil)

	// Assert we get the Dead as the machine's new life status.
	obtainedLife, err = s.state.GetMachineLife(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*obtainedLife, gc.Equals, life.Dead)
}

// TestSetMachineLifeNotFoundError asserts that we get a NotFound if the
// provided machine doesn't exist.
func (s *stateSuite) TestSetMachineLifeNotFoundError(c *gc.C) {
	err := s.state.SetMachineLife(context.Background(), "666", life.Dead)
	c.Assert(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestListAllMachinesEmpty asserts that AllMachineNames returns an empty list
// if there are no machines.
func (s *stateSuite) TestListAllMachinesEmpty(c *gc.C) {
	machines, err := s.state.AllMachineNames(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 0)
}

// TestListAllMachineNamesSuccess asserts the happy path of AllMachineNames at
// the state layer.
func (s *stateSuite) TestListAllMachineNamesSuccess(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "3", "1")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.CreateMachine(context.Background(), "667", "4", "2")
	c.Assert(err, jc.ErrorIsNil)

	machines, err := s.state.AllMachineNames(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	expectedMachines := []string{"666", "667"}
	ms := transform.Slice[machine.Name, string](machines, func(m machine.Name) string { return m.String() })

	sort.Strings(ms)
	sort.Strings(expectedMachines)
	c.Assert(ms, gc.DeepEquals, expectedMachines)
}

// TestIsControllerSuccess asserts the happy path of IsController at the state
// layer.
func (s *stateSuite) TestIsControllerSuccess(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "")
	c.Assert(err, jc.ErrorIsNil)

	isController, err := s.state.IsMachineController(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isController, gc.Equals, false)

	db := s.DB()

	updateIsController := `
UPDATE machine
SET is_controller = TRUE
WHERE name = $1;
`
	_, err = db.ExecContext(context.Background(), updateIsController, "666")
	c.Assert(err, jc.ErrorIsNil)
	isController, err = s.state.IsMachineController(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isController, gc.Equals, true)
}

// TestIsControllerNotFound asserts that a NotFound error is returned when the
// machine is not found.
func (s *stateSuite) TestIsControllerNotFound(c *gc.C) {
	_, err := s.state.IsMachineController(context.Background(), "666")
	c.Assert(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetMachineParentUUIDSuccess asserts the happy path of
// GetMachineParentUUID at the state layer.
func (s *stateSuite) TestGetMachineParentUUIDSuccess(c *gc.C) {
	// Create the parent machine first.
	err := s.state.CreateMachine(context.Background(), "666", "1", "123")
	c.Assert(err, jc.ErrorIsNil)

	// Create the machine with the created parent.
	err = s.state.CreateMachineWithParent(context.Background(), "667", "666", "2", "456")
	c.Assert(err, jc.ErrorIsNil)

	// Get the parent UUID of the machine.
	parentUUID, err := s.state.GetMachineParentUUID(context.Background(), "456")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(parentUUID, gc.Equals, "123")
}

// TestGetMachineParentUUIDNotFound asserts that a NotFound error is returned
// when the machine is not found.
func (s *stateSuite) TestGetMachineParentUUIDNotFound(c *gc.C) {
	_, err := s.state.GetMachineParentUUID(context.Background(), "666")
	c.Assert(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetMachineParentUUIDNoParent asserts that a NotFound error is returned
// when the machine has no parent.
func (s *stateSuite) TestGetMachineParentUUIDNoParent(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "123")
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.state.GetMachineParentUUID(context.Background(), "123")
	c.Assert(err, jc.ErrorIs, machineerrors.MachineHasNoParent)
}

// TestMarkMachineForRemovalSuccess asserts the happy path of
// MarkMachineForRemoval at the state layer.
func (s *stateSuite) TestMarkMachineForRemovalSuccess(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "123")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.MarkMachineForRemoval(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)

	var machineUUID string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT machine_uuid FROM machine_removals WHERE machine_uuid=?", "123").Scan(&machineUUID)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineUUID, gc.Equals, "123")
}

// TestMarkMachineForRemovalSuccessIdempotent asserts that marking a machine for
// removal multiple times is idempotent.
func (s *stateSuite) TestMarkMachineForRemovalSuccessIdempotent(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "123")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.MarkMachineForRemoval(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.MarkMachineForRemoval(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)

	machines, err := s.state.GetAllMachineRemovals(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)
	c.Assert(machines[0], gc.Equals, "123")
}

// TestMarkMachineForRemovalNotFound asserts that a NotFound error is returned
// when the machine is not found.
// TODO(cderici): use machineerrors.MachineNotFound on rebase after #17759
// lands.
func (s *stateSuite) TestMarkMachineForRemovalNotFound(c *gc.C) {
	err := s.state.MarkMachineForRemoval(context.Background(), "666")
	c.Assert(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetAllMachineRemovalsSuccess asserts the happy path of
// GetAllMachineRemovals at the state layer.
func (s *stateSuite) TestGetAllMachineRemovalsSuccess(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "123")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.MarkMachineForRemoval(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)

	machines, err := s.state.GetAllMachineRemovals(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)
	c.Assert(machines[0], gc.Equals, "123")
}

// TestGetAllMachineRemovalsEmpty asserts that GetAllMachineRemovals returns an
// empty list if there are no machines marked for removal.
func (s *stateSuite) TestGetAllMachineRemovalsEmpty(c *gc.C) {
	machines, err := s.state.GetAllMachineRemovals(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 0)
}

// TestGetSomeMachineRemovals asserts the happy path of GetAllMachineRemovals at
// the state layer for a subset of machines.
func (s *stateSuite) TestGetSomeMachineRemovals(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "1", "123")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.CreateMachine(context.Background(), "667", "2", "124")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.CreateMachine(context.Background(), "668", "3", "125")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.MarkMachineForRemoval(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.MarkMachineForRemoval(context.Background(), "668")
	c.Assert(err, jc.ErrorIsNil)

	machines, err := s.state.GetAllMachineRemovals(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 2)
	c.Assert(machines[0], gc.Equals, "123")
	c.Assert(machines[1], gc.Equals, "125")
}

// TestGetMachineUUIDNotFound asserts that a NotFound error is returned
// when the machine is not found.
func (s *stateSuite) TestGetMachineUUIDNotFound(c *gc.C) {
	_, err := s.state.GetMachineUUID(context.Background(), "none")
	c.Assert(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetMachineUUID asserts that the uuid is returned from a machine name
func (s *stateSuite) TestGetMachineUUID(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "rage", "", "123")
	c.Assert(err, jc.ErrorIsNil)

	name, err := s.state.GetMachineUUID(context.Background(), "rage")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "123")
}

func (s *stateSuite) TestKeepInstance(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "")
	c.Assert(err, jc.ErrorIsNil)

	isController, err := s.state.ShouldKeepInstance(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isController, gc.Equals, false)

	db := s.DB()

	updateIsController := `
UPDATE machine
SET    keep_instance = TRUE
WHERE  name = $1`
	_, err = db.ExecContext(context.Background(), updateIsController, "666")
	c.Assert(err, jc.ErrorIsNil)
	isController, err = s.state.ShouldKeepInstance(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isController, gc.Equals, true)
}

// TestIsControllerNotFound asserts that a NotFound error is returned when the
// machine is not found.
func (s *stateSuite) TestKeepInstanceNotFound(c *gc.C) {
	_, err := s.state.ShouldKeepInstance(context.Background(), "666")
	c.Assert(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestSetKeepInstance(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "")
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.SetKeepInstance(context.Background(), "666", true)
	c.Assert(err, jc.ErrorIsNil)

	db := s.DB()
	query := `
SELECT keep_instance
FROM   machine
WHERE  name = $1`
	row := db.QueryRowContext(context.Background(), query, "666")
	c.Assert(row.Err(), jc.ErrorIsNil)

	var keep bool
	c.Assert(row.Scan(&keep), jc.ErrorIsNil)
	c.Check(keep, jc.IsTrue)

}

func (s *stateSuite) TestSetKeepInstanceNotFound(c *gc.C) {
	err := s.state.SetKeepInstance(context.Background(), "666", true)
	c.Assert(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestSetAppliedLXDProfileNames(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "deadbeef")
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.SetMachineCloudInstance(context.Background(), "deadbeef", instance.Id("123"), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.SetAppliedLXDProfileNames(context.Background(), "deadbeef", []string{"profile1", "profile2"})
	c.Assert(err, jc.ErrorIsNil)

	// Check that the profile names are in the machine_lxd_profile table.
	db := s.DB()
	rows, err := db.Query("SELECT name FROM machine_lxd_profile WHERE machine_uuid = 'deadbeef'")
	defer rows.Close()
	c.Assert(err, jc.ErrorIsNil)
	var profiles []string
	for rows.Next() {
		var profile string
		err := rows.Scan(&profile)
		c.Assert(err, jc.ErrorIsNil)
		profiles = append(profiles, profile)
	}
	c.Check(profiles, gc.DeepEquals, []string{"profile1", "profile2"})
}

func (s *stateSuite) TestSetLXDProfilesPartial(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "deadbeef")
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.SetMachineCloudInstance(context.Background(), "deadbeef", instance.Id("123"), "", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Insert a single lxd profile.
	db := s.DB()
	_, err = db.Exec(`INSERT INTO machine_lxd_profile VALUES
("deadbeef", "profile1", 0)`)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetAppliedLXDProfileNames(context.Background(), "deadbeef", []string{"profile1", "profile2"})
	// This shouldn't fail, but add the missing profile to the table.
	c.Assert(err, jc.ErrorIsNil)

	// Check that the profile names are in the machine_lxd_profile table.
	rows, err := db.Query("SELECT name FROM machine_lxd_profile WHERE machine_uuid = 'deadbeef'")
	defer rows.Close()
	c.Assert(err, jc.ErrorIsNil)
	var profiles []string
	for rows.Next() {
		var profile string
		err := rows.Scan(&profile)
		c.Assert(err, jc.ErrorIsNil)
		profiles = append(profiles, profile)
	}
	c.Check(profiles, gc.DeepEquals, []string{"profile1", "profile2"})
}

func (s *stateSuite) TestSetLXDProfilesOverwriteAll(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "deadbeef")
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.SetMachineCloudInstance(context.Background(), "deadbeef", instance.Id("123"), "", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Insert 3 lxd profiles.
	db := s.DB()
	_, err = db.Exec(`INSERT INTO machine_lxd_profile VALUES
("deadbeef", "profile1", 0), ("deadbeef", "profile2", 1), ("deadbeef", "profile3", 2)`)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetAppliedLXDProfileNames(context.Background(), "deadbeef", []string{"profile1", "profile4"})
	c.Assert(err, jc.ErrorIsNil)

	// Check that the profile names are in the machine_lxd_profile table.
	rows, err := db.Query("SELECT name FROM machine_lxd_profile WHERE machine_uuid = 'deadbeef'")
	defer rows.Close()
	c.Assert(err, jc.ErrorIsNil)
	var profiles []string
	for rows.Next() {
		var profile string
		err := rows.Scan(&profile)
		c.Assert(err, jc.ErrorIsNil)
		profiles = append(profiles, profile)
	}
	c.Check(profiles, gc.DeepEquals, []string{"profile1", "profile4"})
}

func (s *stateSuite) TestSetLXDProfilesSameOrder(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "deadbeef")
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.SetMachineCloudInstance(context.Background(), "deadbeef", instance.Id("123"), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.SetAppliedLXDProfileNames(context.Background(), "deadbeef", []string{"profile3", "profile1", "profile2"})
	c.Assert(err, jc.ErrorIsNil)

	profiles, err := s.state.AppliedLXDProfileNames(context.Background(), "deadbeef")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(profiles, gc.DeepEquals, []string{"profile3", "profile1", "profile2"})
}

func (s *stateSuite) TestSetLXDProfilesNotFound(c *gc.C) {
	err := s.state.SetAppliedLXDProfileNames(context.Background(), "666", []string{"profile1", "profile2"})
	c.Assert(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestSetLXDProfilesNotProvisioned(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "deadbeef")
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.SetAppliedLXDProfileNames(context.Background(), "deadbeef", []string{"profile3", "profile1", "profile2"})
	c.Assert(err, jc.ErrorIs, machineerrors.NotProvisioned)
}

func (s *stateSuite) TestSetLXDProfilesEmpty(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "deadbeef")
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.SetMachineCloudInstance(context.Background(), "deadbeef", instance.Id("123"), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.SetAppliedLXDProfileNames(context.Background(), "deadbeef", []string{})
	c.Assert(err, jc.ErrorIsNil)

	profiles, err := s.state.AppliedLXDProfileNames(context.Background(), "deadbeef")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(profiles, gc.HasLen, 0)
}

func (s *stateSuite) TestAppliedLXDProfileNames(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "deadbeef")
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.SetMachineCloudInstance(context.Background(), "deadbeef", instance.Id("123"), "", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Insert 2 lxd profiles.
	db := s.DB()
	_, err = db.Exec(`INSERT INTO machine_lxd_profile VALUES
("deadbeef", "profile1", 0), ("deadbeef", "profile2", 1)`)
	c.Assert(err, jc.ErrorIsNil)

	profiles, err := s.state.AppliedLXDProfileNames(context.Background(), "deadbeef")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(profiles, gc.DeepEquals, []string{"profile1", "profile2"})
}

func (s *stateSuite) TestAppliedLXDProfileNamesNotProvisioned(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "deadbeef")
	c.Assert(err, jc.ErrorIsNil)
	profiles, err := s.state.AppliedLXDProfileNames(context.Background(), "deadbeef")
	c.Assert(err, jc.ErrorIs, machineerrors.NotProvisioned)
	c.Check(profiles, gc.HasLen, 0)
}

func (s *stateSuite) TestAppliedLXDProfileNamesNoErrorEmpty(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "deadbeef")
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.SetMachineCloudInstance(context.Background(), "deadbeef", instance.Id("123"), "", nil)
	c.Assert(err, jc.ErrorIsNil)
	profiles, err := s.state.AppliedLXDProfileNames(context.Background(), "deadbeef")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(profiles, gc.HasLen, 0)
}

// TestSetRunningAgentBinaryVersionSuccess asserts that if we attempt to set the
// running agent binary version for a machine that doesn't exist we get back
// an error that satisfies [machineerrors.MachineNotFound].
func (s *stateSuite) TestSetRunningAgentBinaryVersionMachineNotFound(c *gc.C) {
	machineUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetRunningAgentBinaryVersion(
		context.Background(),
		machineUUID.String(),
		coreagentbinary.Version{
			Number: jujuversion.Current,
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestSetRunningAgentBinaryVersionNotSupportedArch tests that if we provide an
// architecture that isn't supported by the database we get back an error
// that satisfies [coreerrors.NotValid].
func (s *stateSuite) TestSetRunningAgentBinaryVersionNotSupportedArch(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "deadbeef")
	c.Assert(err, jc.ErrorIsNil)

	machineUUID, err := s.state.GetMachineUUID(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetRunningAgentBinaryVersion(
		context.Background(),
		machineUUID,
		coreagentbinary.Version{
			Number: jujuversion.Current,
			Arch:   corearch.Arch("noexist"),
		},
	)
	c.Check(err, jc.ErrorIs, coreerrors.NotSupported)
}

// TestSetRunningAgentBinaryVersion asserts setting the initial agent binary
// version (happy path).
func (s *stateSuite) TestSetRunningAgentBinaryVersion(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "deadbeef")
	c.Assert(err, jc.ErrorIsNil)

	machineUUID, err := s.state.GetMachineUUID(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetRunningAgentBinaryVersion(
		context.Background(),
		machineUUID,
		coreagentbinary.Version{
			Number: jujuversion.Current,
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIsNil)

	var (
		obtainedMachineUUID string
		obtainedVersion     string
		obtainedArch        string
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		stmt := `
SELECT machine_uuid,
       version,
       name
FROM machine_agent_version
INNER JOIN architecture ON machine_agent_version.architecture_id = architecture.id
WHERE machine_uuid = ?
	`

		return tx.QueryRowContext(ctx, stmt, machineUUID).Scan(
			&obtainedMachineUUID,
			&obtainedVersion,
			&obtainedArch,
		)
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(obtainedMachineUUID, gc.Equals, machineUUID)
	c.Check(obtainedVersion, gc.Equals, jujuversion.Current.String())
	c.Check(obtainedArch, gc.Equals, corearch.ARM64)
}

// TestSetRunningAgentBinaryVersion asserts setting the initial agent binary
// version (happy path) and then updating the value.
func (s *stateSuite) TestSetRunningAgentBinaryVersionUpdate(c *gc.C) {
	err := s.state.CreateMachine(context.Background(), "666", "", "deadbeef")
	c.Assert(err, jc.ErrorIsNil)

	machineUUID, err := s.state.GetMachineUUID(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetRunningAgentBinaryVersion(
		context.Background(),
		machineUUID,
		coreagentbinary.Version{
			Number: jujuversion.Current,
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIsNil)

	var (
		obtainedMachineUUID string
		obtainedVersion     string
		obtainedArch        string
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		stmt := `
SELECT machine_uuid,
       version,
       name
FROM machine_agent_version
INNER JOIN architecture ON machine_agent_version.architecture_id = architecture.id
WHERE machine_uuid = ?
	`

		return tx.QueryRowContext(ctx, stmt, machineUUID).Scan(
			&obtainedMachineUUID,
			&obtainedVersion,
			&obtainedArch,
		)
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(obtainedMachineUUID, gc.Equals, machineUUID)
	c.Check(obtainedVersion, gc.Equals, jujuversion.Current.String())

	// Update
	err = s.state.SetRunningAgentBinaryVersion(
		context.Background(),
		machineUUID,
		coreagentbinary.Version{
			Number: jujuversion.Current,
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIsNil)
	c.Check(obtainedArch, gc.Equals, corearch.ARM64)
}
