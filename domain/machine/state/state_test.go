// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	stdtesting "testing"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	domainmachine "github.com/juju/juju/domain/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ModelSuite

	state *State
}

func TestStateSuite(t *stdtesting.T) {
	tc.Run(t, &stateSuite{})
}

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

func (s *stateSuite) TestCreateMachine(c *tc.C) {
	statusState := statusstate.NewModelState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	machineName, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)
	var (
		obtainedMachineName string
		nonce               sql.Null[string]
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT name, nonce FROM machine").Scan(&obtainedMachineName, &nonce)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedMachineName, tc.Equals, machineName.String())
	c.Check(nonce.Valid, tc.IsFalse)

	machineStatusInfo, err := statusState.GetMachineStatus(c.Context(), machineName.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machineStatusInfo.Status, tc.Equals, status.MachineStatusPending)

	instanceStatusInfo, err := statusState.GetInstanceStatus(c.Context(), machineName.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(instanceStatusInfo.Status, tc.Equals, status.InstanceStatusPending)

	containerTypes, err := s.state.GetSupportedContainersTypes(c.Context(), "deadbeef")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(containerTypes, tc.DeepEquals, []string{"lxd"})
}

func (s *stateSuite) TestCreateMachineWithNonce(c *tc.C) {
	statusState := statusstate.NewModelState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	machineName, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		Nonce: ptr("nonce-123"),
	})
	c.Assert(err, tc.ErrorIsNil)

	var (
		obtainedMachineName string
		nonce               sql.Null[string]
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT name, nonce FROM machine").Scan(&obtainedMachineName, &nonce)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machineName, tc.Equals, machineName)
	c.Assert(nonce.Valid, tc.IsTrue)
	c.Check(nonce.V, tc.Equals, "nonce-123")

	machineStatusInfo, err := statusState.GetMachineStatus(c.Context(), machineName.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machineStatusInfo.Status, tc.Equals, status.MachineStatusPending)

	instanceStatusInfo, err := statusState.GetInstanceStatus(c.Context(), machineName.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(instanceStatusInfo.Status, tc.Equals, status.InstanceStatusPending)
}

// TestCreateMachineWithParentSuccess asserts the happy path of
// CreateMachineWithParent at the state layer.
func (s *stateSuite) TestCreateMachineWithParentSuccess(c *tc.C) {
	// Create the parent first.
	_, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "parent-uuid",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Create the machine with the created parent.
	mName, err := s.state.CreateMachineWithParent(c.Context(), domainmachine.CreateMachineArgs{}, "parent-uuid")
	c.Assert(err, tc.ErrorIsNil)

	// Make sure the newly created machine with parent has been created.
	var (
		obtainedMachineName string
	)
	parentStmt := `
SELECT  name
FROM    machine
        LEFT JOIN machine_parent AS parent
	ON        parent.machine_uuid = machine.uuid
WHERE   parent.parent_uuid = ?
	`
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, parentStmt, "parent-uuid").Scan(&obtainedMachineName)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedMachineName, tc.Equals, mName.String())
}

// TestCreateMachineWithParentNotFound asserts that a NotFound error is returned
// when the parent machine is not found.
func (s *stateSuite) TestCreateMachineWithParentNotFound(c *tc.C) {
	_, err := s.state.CreateMachineWithParent(c.Context(), domainmachine.CreateMachineArgs{}, "unknown-parent-uuid")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetMachineParentUUIDGrandParentNotAllowed asserts that a
// GrandParentNotAllowed error is returned when a grandparent is detected for a
// machine.
func (s *stateSuite) TestCreateMachineWithGrandParentNotAllowed(c *tc.C) {
	// Create the parent machine first.
	_, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "grand-parent-uuid",
	})
	c.Assert(err, tc.ErrorIsNil)
	// Create the machine with the created parent.
	_, err = s.state.CreateMachineWithParent(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "parent-uuid",
	}, "grand-parent-uuid")
	c.Assert(err, tc.ErrorIsNil)
	// This fails.
	_, err = s.state.CreateMachineWithParent(c.Context(), domainmachine.CreateMachineArgs{}, "parent-uuid")
	c.Assert(err, tc.ErrorIs, machineerrors.GrandParentNotSupported)
}

// TestDeleteMachine asserts the happy path of DeleteMachine at the state layer.
func (s *stateSuite) TestDeleteMachine(c *tc.C) {
	machineName, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{})
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
	s.insertBlockDevice(c, bd, bdUUID, string(machineName))

	err = s.state.DeleteMachine(c.Context(), machineName)
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
	machineName, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
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
	s.insertBlockDevice(c, bd, bdUUID, string(machineName))

	s.state.SetMachineStatus(c.Context(), "666", status.StatusInfo[status.MachineStatusType]{
		Status:  status.MachineStatusStarted,
		Message: "started",
		Data:    []byte(`{"key": "data"}`),
	})

	err = s.state.DeleteMachine(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)

	var status int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT count(*) FROM machine_status WHERE machine_uuid=?", "deadbeef").Scan(&status)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(status, tc.Equals, 0)
}

func (s *stateSuite) insertBlockDevice(c *tc.C, bd blockdevice.BlockDevice, blockDeviceUUID, machineName string) {
	db := s.DB()

	inUse := 0
	if bd.InUse {
		inUse = 1
	}
	_, err := db.ExecContext(c.Context(), `
INSERT INTO block_device (uuid, name, label, device_uuid, hardware_id, wwn, bus_address, serial_id, mount_point, filesystem_type_id, Size_mib, in_use, machine_uuid)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 2, ?, ?, (SELECT uuid FROM machine WHERE name=?))
`, blockDeviceUUID, bd.DeviceName, bd.Label, bd.UUID, bd.HardwareId, bd.WWN, bd.BusAddress, bd.SerialId, bd.MountPoint, bd.SizeMiB, inUse, machineName)
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
	machineName, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	obtainedLife, err := s.state.GetMachineLife(c.Context(), machineName)
	expectedLife := life.Alive
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedLife, tc.Equals, expectedLife)
}

// TestGetMachineLifeNotFound asserts that a NotFound error is returned when the
// machine is not found.
func (s *stateSuite) TestGetMachineLifeNotFound(c *tc.C) {
	_, err := s.state.GetMachineLife(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestListAllMachines(c *tc.C) {
	mn0, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "uuid0",
	})
	c.Assert(err, tc.ErrorIsNil)

	mn1, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "uuid1",
	})
	c.Assert(err, tc.ErrorIsNil)

	machines, err := s.state.AllMachineNames(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(machines, tc.SameContents, []machine.Name{
		mn0,
		mn1,
	})
}

// TestSetMachineLifeSuccess asserts the happy path of SetMachineLife at the
// state layer.
func (s *stateSuite) TestSetMachineLifeSuccess(c *tc.C) {
	machineName, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Assert the life status is initially Alive
	obtainedLife, err := s.state.GetMachineLife(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedLife, tc.Equals, life.Alive)

	// Set the machine's life to Dead
	err = s.state.SetMachineLife(c.Context(), machineName, life.Dead)
	c.Assert(err, tc.ErrorIsNil)

	// Assert we get the Dead as the machine's new life status.
	obtainedLife, err = s.state.GetMachineLife(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedLife, tc.Equals, life.Dead)
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
	mn0, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "uuid0",
	})
	c.Assert(err, tc.ErrorIsNil)
	mn1, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "uuid1",
	})
	c.Assert(err, tc.ErrorIsNil)

	machines, err := s.state.AllMachineNames(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(machines, tc.SameContents, []machine.Name{mn0, mn1})
}

func (s *stateSuite) TestIsMachineControllerApplicationController(c *tc.C) {
	machineName := s.createApplicationWithUnitAndMachine(c, true, false)

	isController, err := s.state.IsMachineController(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isController, tc.IsTrue)
}

func (s *stateSuite) TestIsMachineControllerApplicationNonController(c *tc.C) {
	machineName := s.createApplicationWithUnitAndMachine(c, false, false)

	isController, err := s.state.IsMachineController(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isController, tc.IsFalse)
}

func (s *stateSuite) TestIsMachineControllerFailure(c *tc.C) {
	machineName, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	isController, err := s.state.IsMachineController(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isController, tc.IsFalse)
}

// TestIsMachineControllerNotFound asserts that a NotFound error is returned when the
// machine is not found.
func (s *stateSuite) TestIsMachineControllerNotFound(c *tc.C) {
	_, err := s.state.IsMachineController(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestIsMachineManuallyProvisioned(c *tc.C) {
	machineName := s.createApplicationWithUnitAndMachine(c, false, false)

	isManual, err := s.state.IsMachineManuallyProvisioned(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isManual, tc.IsFalse)
}

func (s *stateSuite) TestIsMachineManuallyProvisionedManual(c *tc.C) {
	machineName := s.createApplicationWithUnitAndMachine(c, false, false)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO machine_manual (machine_uuid)
VALUES ((SELECT uuid FROM machine WHERE name=?))
`, machineName)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	isManual, err := s.state.IsMachineManuallyProvisioned(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isManual, tc.IsTrue)
}

func (s *stateSuite) TestIsMachineManuallyProvisionedNotFound(c *tc.C) {
	_, err := s.state.IsMachineManuallyProvisioned(c.Context(), machine.Name("666"))
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetMachineParentUUIDSuccess asserts the happy path of
// GetMachineParentUUID at the state layer.
func (s *stateSuite) TestGetMachineParentUUIDSuccess(c *tc.C) {
	// Create the parent machine first.
	_, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "parent-uuid",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Create the machine with the created parent.
	_, err = s.state.CreateMachineWithParent(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "child-uuid",
	}, "parent-uuid")
	c.Assert(err, tc.ErrorIsNil)

	// Get the parent UUID of the machine.
	obtainedParentUUID, err := s.state.GetMachineParentUUID(c.Context(), "child-uuid")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedParentUUID.String(), tc.Equals, "parent-uuid")
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
	_, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.state.GetMachineParentUUID(c.Context(), "deadbeef")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineHasNoParent)
}

// TestMarkMachineForRemovalSuccess asserts the happy path of
// MarkMachineForRemoval at the state layer.
func (s *stateSuite) TestMarkMachineForRemovalSuccess(c *tc.C) {
	machineName, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.MarkMachineForRemoval(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)

	var obtainedMachineUUID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT machine_uuid FROM machine_removals WHERE machine_uuid=?", "deadbeef").Scan(&obtainedMachineUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedMachineUUID, tc.Equals, "deadbeef")
}

// TestMarkMachineForRemovalSuccessIdempotent asserts that marking a machine for
// removal multiple times is idempotent.
func (s *stateSuite) TestMarkMachineForRemovalSuccessIdempotent(c *tc.C) {
	machineName, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.MarkMachineForRemoval(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.MarkMachineForRemoval(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)

	machines, err := s.state.GetAllMachineRemovals(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, 1)
	c.Assert(machines[0].String(), tc.Equals, "deadbeef")
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
	machineName, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.MarkMachineForRemoval(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)

	machines, err := s.state.GetAllMachineRemovals(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, 1)
	c.Assert(machines[0].String(), tc.Equals, "deadbeef")
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
	name0, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "uuid0",
	})
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{})
	c.Assert(err, tc.ErrorIsNil)

	name2, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "uuid2",
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.MarkMachineForRemoval(c.Context(), name0)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.MarkMachineForRemoval(c.Context(), name2)
	c.Assert(err, tc.ErrorIsNil)

	machines, err := s.state.GetAllMachineRemovals(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, 2)
	c.Assert(machines[0].String(), tc.Equals, "uuid0")
	c.Assert(machines[1].String(), tc.Equals, "uuid2")
}

// TestGetMachineUUIDNotFound asserts that a NotFound error is returned
// when the machine is not found.
func (s *stateSuite) TestGetMachineUUIDNotFound(c *tc.C) {
	_, err := s.state.GetMachineUUID(c.Context(), "none")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetMachineUUID asserts that the uuid is returned from a machine name
func (s *stateSuite) TestGetMachineUUID(c *tc.C) {
	machineName, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	uuid, err := s.state.GetMachineUUID(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(uuid.String(), tc.Equals, "deadbeef")
}

func (s *stateSuite) TestKeepInstance(c *tc.C) {
	machineName, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{})
	c.Assert(err, tc.ErrorIsNil)

	isController, err := s.state.ShouldKeepInstance(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isController, tc.IsFalse)

	db := s.DB()

	updateIsController := `
UPDATE machine
SET    keep_instance = TRUE
WHERE  name = $1`
	_, err = db.ExecContext(c.Context(), updateIsController, machineName)
	c.Assert(err, tc.ErrorIsNil)
	isController, err = s.state.ShouldKeepInstance(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isController, tc.IsTrue)
}

// TestIsControllerNotFound asserts that a NotFound error is returned when the
// machine is not found.
func (s *stateSuite) TestKeepInstanceNotFound(c *tc.C) {
	_, err := s.state.ShouldKeepInstance(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestSetKeepInstance(c *tc.C) {
	machineName, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{})
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetKeepInstance(c.Context(), machineName, true)
	c.Assert(err, tc.ErrorIsNil)

	db := s.DB()
	query := `
SELECT keep_instance
FROM   machine
WHERE  name = $1`
	row := db.QueryRowContext(c.Context(), query, machineName)
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
	_, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetMachineCloudInstance(c.Context(), "deadbeef", instance.Id("123"), "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetAppliedLXDProfileNames(c.Context(), "deadbeef", []string{"profile1", "profile2"})
	c.Assert(err, tc.ErrorIsNil)

	// Check that the profile names are in the machine_lxd_profile table.
	var profiles []string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT name FROM machine_lxd_profile WHERE machine_uuid = ?", "deadbeef")
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var profile string
			err = rows.Scan(&profile)
			if err != nil {
				return err
			}
			profiles = append(profiles, profile)
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(profiles, tc.SameContents, []string{"profile1", "profile2"})
}

func (s *stateSuite) TestSetLXDProfilesPartial(c *tc.C) {
	_, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetMachineCloudInstance(c.Context(), "deadbeef", instance.Id("123"), "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	// Insert a single lxd profile.
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO machine_lxd_profile VALUES
(?, "profile2", 0)`, "deadbeef")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetAppliedLXDProfileNames(c.Context(), "deadbeef", []string{"profile1", "profile2"})
	// This shouldn't fail, but add the missing profile to the table.
	c.Assert(err, tc.ErrorIsNil)

	// Check that the profile names are in the machine_lxd_profile table.
	var profiles []string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT name FROM machine_lxd_profile WHERE machine_uuid = ?", "deadbeef")
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var profile string
			err = rows.Scan(&profile)
			if err != nil {
				return err
			}
			profiles = append(profiles, profile)
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(profiles, tc.DeepEquals, []string{"profile1", "profile2"})
}

func (s *stateSuite) TestSetLXDProfilesOverwriteAll(c *tc.C) {
	_, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetMachineCloudInstance(c.Context(), "deadbeef", instance.Id("123"), "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	// Insert 3 lxd profiles.
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO machine_lxd_profile VALUES
(?, "profile1", 0)`, "deadbeef")
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO machine_lxd_profile VALUES
(?, "profile2", 1)`, "deadbeef")
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO machine_lxd_profile VALUES
(?, "profile3", 2)`, "deadbeef")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetAppliedLXDProfileNames(c.Context(), "deadbeef", []string{"profile1", "profile4"})
	c.Assert(err, tc.ErrorIsNil)

	// Check that the profile names are in the machine_lxd_profile table.
	var profiles []string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT name FROM machine_lxd_profile WHERE machine_uuid = ?", "deadbeef")
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var profile string
			err = rows.Scan(&profile)
			if err != nil {
				return err
			}
			profiles = append(profiles, profile)
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(profiles, tc.DeepEquals, []string{"profile1", "profile4"})
}

func (s *stateSuite) TestSetLXDProfilesSameOrder(c *tc.C) {
	_, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetMachineCloudInstance(c.Context(), "deadbeef", instance.Id("123"), "", "nonce", nil)
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
	_, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetAppliedLXDProfileNames(c.Context(), "deadbeef", []string{"profile3", "profile1", "profile2"})
	c.Assert(err, tc.ErrorIs, machineerrors.NotProvisioned)
}

func (s *stateSuite) TestSetLXDProfilesEmpty(c *tc.C) {
	_, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetMachineCloudInstance(c.Context(), "deadbeef", instance.Id("123"), "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetAppliedLXDProfileNames(c.Context(), "deadbeef", []string{})
	c.Assert(err, tc.ErrorIsNil)

	profiles, err := s.state.AppliedLXDProfileNames(c.Context(), "deadbeef")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(profiles, tc.HasLen, 0)
}

func (s *stateSuite) TestAppliedLXDProfileNames(c *tc.C) {
	_, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetMachineCloudInstance(c.Context(), "deadbeef", instance.Id("123"), "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	// Insert 2 lxd profiles.
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO machine_lxd_profile VALUES
(?, "profile1", 0)`, "deadbeef")
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO machine_lxd_profile VALUES
(?, "profile2", 1)`, "deadbeef")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	profiles, err := s.state.AppliedLXDProfileNames(c.Context(), "deadbeef")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(profiles, tc.DeepEquals, []string{"profile1", "profile2"})
}

func (s *stateSuite) TestAppliedLXDProfileNamesNotProvisioned(c *tc.C) {
	_, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)
	profiles, err := s.state.AppliedLXDProfileNames(c.Context(), "deadbeef")
	c.Assert(err, tc.ErrorIs, machineerrors.NotProvisioned)
	c.Check(profiles, tc.HasLen, 0)
}

func (s *stateSuite) TestAppliedLXDProfileNamesNoErrorEmpty(c *tc.C) {
	_, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetMachineCloudInstance(c.Context(), "deadbeef", instance.Id("123"), "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
	profiles, err := s.state.AppliedLXDProfileNames(c.Context(), "deadbeef")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(profiles, tc.HasLen, 0)
}

func (s *stateSuite) TestGetNamesForUUIDs(c *tc.C) {
	// Arrange
	mn0, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "uuid0",
	})
	c.Assert(err, tc.ErrorIsNil)
	mn1, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "uuid1",
	})
	c.Assert(err, tc.ErrorIsNil)
	mn2, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "uuid2",
	})
	c.Assert(err, tc.ErrorIsNil)
	expected := map[machine.UUID]machine.Name{
		machine.UUID("uuid0"): mn0,
		machine.UUID("uuid1"): mn1,
		machine.UUID("uuid2"): mn2,
	}

	// Act
	obtained, err := s.state.GetNamesForUUIDs(c.Context(), []string{"uuid0", "uuid1", "uuid2"})

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

func (s *stateSuite) TestGetAllProvisionedMachineInstanceID(c *tc.C) {
	machineInstances, err := s.state.GetAllProvisionedMachineInstanceID(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machineInstances, tc.HasLen, 0)

	machineName, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	machineInstances, err = s.state.GetAllProvisionedMachineInstanceID(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machineInstances, tc.HasLen, 0)

	err = s.state.SetMachineCloudInstance(c.Context(), "deadbeef", instance.Id("123"), "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	machineInstances, err = s.state.GetAllProvisionedMachineInstanceID(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machineInstances, tc.DeepEquals, map[machine.Name]string{
		machineName: "123",
	})
}

func (s *stateSuite) TestGetAllProvisionedMachineInstanceIDContainer(c *tc.C) {
	parentName, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "parent-uuid",
	})
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.state.CreateMachineWithParent(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "child-uuid",
	}, "parent-uuid")
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetMachineCloudInstance(c.Context(), "child-uuid", instance.Id("123"), "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetMachineCloudInstance(c.Context(), "deadbeef2", instance.Id("124"), "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().Txn(c.Context(), func(c context.Context, tx *sqlair.TX) error {
		return s.state.createParentMachineLink(c, tx, createMachineArgs{
			name:        "666/lxd/667",
			machineUUID: "deadbeef2",
			netNodeUUID: "abc1",
		})
	})
	err = s.state.SetMachineCloudInstance(c.Context(), "parent-uuid", instance.Id("124"), "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	machineInstances, err := s.state.GetAllProvisionedMachineInstanceID(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machineInstances, tc.DeepEquals, map[machine.Name]string{
		parentName: "124",
	})
}

func (s *stateSuite) TestSetMachineHostname(c *tc.C) {
	mName, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetMachineHostname(c.Context(), "deadbeef", "my-hostname")
	c.Assert(err, tc.ErrorIsNil)

	var hostname string
	err = s.TxnRunner().StdTxn(c.Context(), func(c context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(c, "SELECT hostname FROM machine WHERE name = ?", mName).Scan(&hostname)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(hostname, tc.Equals, "my-hostname")
}

func (s *stateSuite) TestSetMachineHostnameEmpty(c *tc.C) {
	mName, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetMachineHostname(c.Context(), "deadbeef", "")
	c.Assert(err, tc.ErrorIsNil)

	var hostname *string
	err = s.TxnRunner().StdTxn(c.Context(), func(c context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(c, "SELECT hostname FROM machine WHERE name = ?", mName).Scan(&hostname)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(hostname, tc.IsNil)
}

func (s *stateSuite) TestSetMachineHostnameNoMachine(c *tc.C) {
	err := s.state.SetMachineHostname(c.Context(), "666", "my-hostname")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestGetSupportedContainersTypes(c *tc.C) {
	_, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	containerTypes, err := s.state.GetSupportedContainersTypes(c.Context(), "deadbeef")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(containerTypes, tc.DeepEquals, []string{"lxd"})
}

func (s *stateSuite) TestGetSupportedContainersTypesNoMachine(c *tc.C) {
	_, err := s.state.GetSupportedContainersTypes(c.Context(), "deadbeef")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestGetMachinePrincipalApplications(c *tc.C) {
	s.createApplicationWithUnitAndMachine(c, false, false)

	principalUnits, err := s.state.GetMachinePrincipalApplications(c.Context(), "0")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(principalUnits, tc.DeepEquals, []string{"foo"})
}

func (s *stateSuite) TestGetMachinePrincipalApplicationsSubordinate(c *tc.C) {
	s.createApplicationWithUnitAndMachine(c, false, true)

	principalUnits, err := s.state.GetMachinePrincipalApplications(c.Context(), "0")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(principalUnits, tc.DeepEquals, []string{})
}

func (s *stateSuite) TestGetMachinePrincipalApplicationsNotFound(c *tc.C) {
	_, err := s.state.GetMachinePrincipalApplications(c.Context(), "1")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) createApplicationWithUnitAndMachine(c *tc.C, controller, subordinate bool) machine.Name {
	machineName := machine.Name("0")
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		_, err = tx.ExecContext(ctx, `
INSERT INTO charm (uuid, reference_name, source_id) 
VALUES (?, 'foo', 0)`, "charm-uuid")
		if err != nil {
			return errors.Capture(err)
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_metadata (charm_uuid, name, subordinate) 
VALUES (?, 'foo', ?)`, "charm-uuid", subordinate)
		if err != nil {
			return errors.Capture(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO application (uuid, charm_uuid, name, life_id, space_uuid)
VALUES (?,?,?,0,?)`, "app-uuid", "charm-uuid", "foo", network.AlphaSpaceId)
		if err != nil {
			return errors.Capture(err)
		}
		netNodeUUID := uuid.MustNewUUID().String()
		_, err = tx.ExecContext(ctx, `
INSERT INTO net_node (uuid)
VALUES (?)
`, netNodeUUID)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO unit (uuid, name, life_id, net_node_uuid, application_uuid, charm_uuid)
SELECT ?, ?, ?, ?, uuid, charm_uuid
FROM application
WHERE uuid = ?
`, "unit-uuid", "foo/0", "0", netNodeUUID, "app-uuid")
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO machine (uuid, name, life_id, net_node_uuid)
SELECT ?, ?, ?, net_node_uuid
FROM unit
WHERE uuid = ?
`, "machine-uuid", machineName, 0, "unit-uuid")
		if err != nil {
			return err
		}

		if controller {
			_, err = tx.ExecContext(ctx, `
INSERT INTO application_controller (application_uuid)
VALUES (?)`, "app-uuid")
			if err != nil {
				return errors.Capture(err)
			}
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	return machineName
}

func (s *stateSuite) TestGetMachinePlacementDirective(c *tc.C) {
	machineName, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return insertMachineProviderPlacement(c.Context(), tx, s.state, "deadbeef", "0/lxd/42")
	})
	c.Assert(err, tc.ErrorIsNil)

	placement, err := s.state.GetMachinePlacementDirective(c.Context(), machineName.String())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(*placement, tc.Equals, "0/lxd/42")
}

func (s *stateSuite) TestGetMachinePlacementDirectiveNotFound(c *tc.C) {
	_, err := s.state.GetMachinePlacementDirective(c.Context(), "1")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestConstraintFull(c *tc.C) {
	machineName, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		addConstraintStmt := `INSERT INTO "constraint" (uuid, arch, cpu_cores, cpu_power, mem, root_disk, root_disk_source, instance_role, instance_type, container_type_id, virt_type, allocate_public_ip, image_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
		_, err := tx.ExecContext(ctx, addConstraintStmt, "constraint-uuid", "amd64", 2, 42, 8, 256, "root-disk-source", "instance-role", "instance-type", 1, "virt-type", true, "image-id")
		if err != nil {
			return err
		}

		addTagConsStmt := `INSERT INTO constraint_tag (constraint_uuid, tag) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, addTagConsStmt, "constraint-uuid", "tag0")
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, addTagConsStmt, "constraint-uuid", "tag1")
		if err != nil {
			return err
		}
		addSpaceStmt := `INSERT INTO space (uuid, name) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, addSpaceStmt, "space0-uuid", "space0")
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, addSpaceStmt, "space1-uuid", "space1")
		if err != nil {
			return err
		}
		addSpaceConsStmt := `INSERT INTO constraint_space (constraint_uuid, space, exclude) VALUES (?, ?, ?)`
		_, err = tx.ExecContext(ctx, addSpaceConsStmt, "constraint-uuid", "space0", false)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, addSpaceConsStmt, "constraint-uuid", "space1", true)
		if err != nil {
			return err
		}
		addZoneConsStmt := `INSERT INTO constraint_zone (constraint_uuid, zone) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, addZoneConsStmt, "constraint-uuid", "zone0")
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, addZoneConsStmt, "constraint-uuid", "zone1")
		if err != nil {
			return err
		}

		addMachineConstraintStmt := `INSERT INTO machine_constraint (machine_uuid, constraint_uuid) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, addMachineConstraintStmt, "deadbeef", "constraint-uuid")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	cons, err := s.state.GetMachineConstraints(c.Context(), machineName.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*cons.Tags, tc.SameContents, []string{"tag0", "tag1"})
	c.Check(*cons.Spaces, tc.SameContents, []constraints.SpaceConstraint{
		{SpaceName: "space0", Exclude: false},
		{SpaceName: "space1", Exclude: true},
	})
	c.Check(*cons.Zones, tc.SameContents, []string{"zone0", "zone1"})
	c.Check(cons.Arch, tc.DeepEquals, ptr("amd64"))
	c.Check(cons.CpuCores, tc.DeepEquals, ptr(uint64(2)))
	c.Check(cons.CpuPower, tc.DeepEquals, ptr(uint64(42)))
	c.Check(cons.Mem, tc.DeepEquals, ptr(uint64(8)))
	c.Check(cons.RootDisk, tc.DeepEquals, ptr(uint64(256)))
	c.Check(cons.RootDiskSource, tc.DeepEquals, ptr("root-disk-source"))
	c.Check(cons.InstanceRole, tc.DeepEquals, ptr("instance-role"))
	c.Check(cons.InstanceType, tc.DeepEquals, ptr("instance-type"))
	c.Check(cons.Container, tc.DeepEquals, ptr(instance.LXD))
	c.Check(cons.VirtType, tc.DeepEquals, ptr("virt-type"))
	c.Check(cons.AllocatePublicIP, tc.DeepEquals, ptr(true))
	c.Check(cons.ImageID, tc.DeepEquals, ptr("image-id"))
}

func (s *stateSuite) TestConstraintPartial(c *tc.C) {
	machineName, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		addConstraintStmt := `INSERT INTO "constraint" (uuid, arch, cpu_cores, allocate_public_ip, image_id) VALUES (?, ?, ?, ?, ?)`
		_, err := tx.ExecContext(ctx, addConstraintStmt, "constraint-uuid", "amd64", 2, true, "image-id")
		if err != nil {
			return err
		}
		addMachineConstraintStmt := `INSERT INTO machine_constraint (machine_uuid, constraint_uuid) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, addMachineConstraintStmt, "deadbeef", "constraint-uuid")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	cons, err := s.state.GetMachineConstraints(c.Context(), machineName.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cons, tc.DeepEquals, constraints.Constraints{
		Arch:             ptr("amd64"),
		CpuCores:         ptr(uint64(2)),
		AllocatePublicIP: ptr(true),
		ImageID:          ptr("image-id"),
	})
}

func (s *stateSuite) TestConstraintSingleValue(c *tc.C) {
	machineName, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		addConstraintStmt := `INSERT INTO "constraint" (uuid, cpu_cores) VALUES (?, ?)`
		_, err := tx.ExecContext(ctx, addConstraintStmt, "constraint-uuid", 2)
		if err != nil {
			return err
		}
		addMachineConstraintStmt := `INSERT INTO machine_constraint (machine_uuid, constraint_uuid) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, addMachineConstraintStmt, "deadbeef", "constraint-uuid")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	cons, err := s.state.GetMachineConstraints(c.Context(), machineName.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cons, tc.DeepEquals, constraints.Constraints{
		CpuCores: ptr(uint64(2)),
	})
}

func (s *stateSuite) TestConstraintEmpty(c *tc.C) {
	machineName, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	cons, err := s.state.GetMachineConstraints(c.Context(), machineName.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cons, tc.DeepEquals, constraints.Constraints{})
}

func (s *stateSuite) TestConstraintsApplicationNotFound(c *tc.C) {
	_, err := s.state.GetMachineConstraints(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestGetMachineBase(c *tc.C) {
	machineName, err := s.state.CreateMachine(c.Context(), domainmachine.CreateMachineArgs{
		MachineUUID: "deadbeef",
		Platform: deployment.Platform{
			OSType:       deployment.Ubuntu,
			Channel:      "22.04/stable",
			Architecture: architecture.AMD64,
		},
	})

	s.DumpTable(c, "machine_platform", "machine")

	base, err := s.state.GetMachineBase(c.Context(), machineName.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(base, tc.DeepEquals, corebase.Base{
		OS: "ubuntu",
		Channel: corebase.Channel{
			Track: "22.04",
			Risk:  "stable",
		},
	})
}

func (s *stateSuite) TestGetMachineBaseNotFound(c *tc.C) {
	_, err := s.state.GetMachineBase(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}
