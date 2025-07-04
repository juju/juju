// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	stdtesting "testing"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/network"
	usertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	domainmachine "github.com/juju/juju/domain/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modelstate "github.com/juju/juju/domain/model/state"
	"github.com/juju/juju/domain/modelagent"
	domainnetwork "github.com/juju/juju/domain/network"
	networkstate "github.com/juju/juju/domain/network/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
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

// TestDeleteMachine asserts the happy path of DeleteMachine at the state layer.
func (s *stateSuite) TestDeleteMachine(c *tc.C) {
	_, machineName := s.addMachine(c)

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

	err := s.state.DeleteMachine(c.Context(), machineName)
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
	_, machineName := s.addMachine(c)

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
	_, machineName0 := s.addMachine(c)
	_, machineName1 := s.addMachine(c)

	machines, err := s.state.AllMachineNames(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(machines, tc.SameContents, []machine.Name{
		machineName0,
		machineName1,
	})
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
	_, mn0 := s.addMachine(c)
	_, mn1 := s.addMachine(c)

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

func (s *stateSuite) TestIsMachineControllerApplicationControllerMultiple(c *tc.C) {
	machineName0 := s.createApplicationWithUnitAndMachine(c, true, false)
	machineName1 := s.addUnit(c)

	isController, err := s.state.IsMachineController(c.Context(), machineName0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(isController, tc.IsTrue)

	isController, err = s.state.IsMachineController(c.Context(), machineName1)
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
	_, machineName := s.addMachine(c)

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

// TestGetMachineParentUUIDNotFound asserts that a NotFound error is returned
// when the machine is not found.
func (s *stateSuite) TestGetMachineParentUUIDNotFound(c *tc.C) {
	_, err := s.state.GetMachineParentUUID(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetMachineParentUUIDNoParent asserts that a NotFound error is returned
// when the machine has no parent.
func (s *stateSuite) TestGetMachineParentUUIDNoParent(c *tc.C) {
	machineUUID, _ := s.addMachine(c)

	_, err := s.state.GetMachineParentUUID(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIs, machineerrors.MachineHasNoParent)
}

// TestGetMachineUUIDNotFound asserts that a NotFound error is returned
// when the machine is not found.
func (s *stateSuite) TestGetMachineUUIDNotFound(c *tc.C) {
	_, err := s.state.GetMachineUUID(c.Context(), "none")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetMachineUUID asserts that the uuid is returned from a machine name
func (s *stateSuite) TestGetMachineUUID(c *tc.C) {
	machineUUID, machineName := s.addMachine(c)

	uuid, err := s.state.GetMachineUUID(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(uuid, tc.Equals, machineUUID)
}

func (s *stateSuite) TestKeepInstance(c *tc.C) {
	_, machineName := s.addMachine(c)

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
	_, machineName := s.addMachine(c)
	err := s.state.SetKeepInstance(c.Context(), machineName, true)
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
	machineUUID, _ := s.addMachine(c)
	err := s.state.SetMachineCloudInstance(c.Context(), machineUUID.String(), instance.Id("123"), "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetAppliedLXDProfileNames(c.Context(), machineUUID.String(), []string{"profile1", "profile2"})
	c.Assert(err, tc.ErrorIsNil)

	// Check that the profile names are in the machine_lxd_profile table.
	var profiles []string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT name FROM machine_lxd_profile WHERE machine_uuid = ?", machineUUID.String())
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
	machineUUID, _ := s.addMachine(c)
	err := s.state.SetMachineCloudInstance(c.Context(), machineUUID.String(), instance.Id("123"), "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	// Insert a single lxd profile.
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO machine_lxd_profile VALUES
(?, "profile2", 0)`, machineUUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetAppliedLXDProfileNames(c.Context(), machineUUID.String(), []string{"profile1", "profile2"})
	// This shouldn't fail, but add the missing profile to the table.
	c.Assert(err, tc.ErrorIsNil)

	// Check that the profile names are in the machine_lxd_profile table.
	var profiles []string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT name FROM machine_lxd_profile WHERE machine_uuid = ?", machineUUID.String())
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
	machineUUID, _ := s.addMachine(c)
	err := s.state.SetMachineCloudInstance(c.Context(), machineUUID.String(), instance.Id("123"), "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	// Insert 3 lxd profiles.
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO machine_lxd_profile VALUES
(?, "profile1", 0)`, machineUUID.String())
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO machine_lxd_profile VALUES
(?, "profile2", 1)`, machineUUID.String())
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO machine_lxd_profile VALUES
(?, "profile3", 2)`, machineUUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetAppliedLXDProfileNames(c.Context(), machineUUID.String(), []string{"profile1", "profile4"})
	c.Assert(err, tc.ErrorIsNil)

	// Check that the profile names are in the machine_lxd_profile table.
	var profiles []string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT name FROM machine_lxd_profile WHERE machine_uuid = ?", machineUUID.String())
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
	machineUUID, _ := s.addMachine(c)
	err := s.state.SetMachineCloudInstance(c.Context(), machineUUID.String(), instance.Id("123"), "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetAppliedLXDProfileNames(c.Context(), machineUUID.String(), []string{"profile3", "profile1", "profile2"})
	c.Assert(err, tc.ErrorIsNil)

	profiles, err := s.state.AppliedLXDProfileNames(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(profiles, tc.DeepEquals, []string{"profile3", "profile1", "profile2"})
}

func (s *stateSuite) TestSetLXDProfilesNotFound(c *tc.C) {
	err := s.state.SetAppliedLXDProfileNames(c.Context(), "666", []string{"profile1", "profile2"})
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestSetLXDProfilesNotProvisioned(c *tc.C) {
	machineUUID, _ := s.addMachine(c)
	err := s.state.SetAppliedLXDProfileNames(c.Context(), machineUUID.String(), []string{"profile3", "profile1", "profile2"})
	c.Assert(err, tc.ErrorIs, machineerrors.NotProvisioned)
}

func (s *stateSuite) TestSetLXDProfilesEmpty(c *tc.C) {
	machineUUID, _ := s.addMachine(c)
	err := s.state.SetMachineCloudInstance(c.Context(), machineUUID.String(), instance.Id("123"), "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetAppliedLXDProfileNames(c.Context(), machineUUID.String(), []string{})
	c.Assert(err, tc.ErrorIsNil)

	profiles, err := s.state.AppliedLXDProfileNames(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(profiles, tc.HasLen, 0)
}

func (s *stateSuite) TestAppliedLXDProfileNames(c *tc.C) {
	machineUUID, _ := s.addMachine(c)
	err := s.state.SetMachineCloudInstance(c.Context(), machineUUID.String(), instance.Id("123"), "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	// Insert 2 lxd profiles.
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO machine_lxd_profile VALUES
(?, "profile1", 0)`, machineUUID.String())
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO machine_lxd_profile VALUES
(?, "profile2", 1)`, machineUUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	profiles, err := s.state.AppliedLXDProfileNames(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(profiles, tc.DeepEquals, []string{"profile1", "profile2"})
}

func (s *stateSuite) TestAppliedLXDProfileNamesNotProvisioned(c *tc.C) {
	machineUUID, _ := s.addMachine(c)
	profiles, err := s.state.AppliedLXDProfileNames(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIs, machineerrors.NotProvisioned)
	c.Check(profiles, tc.HasLen, 0)
}

func (s *stateSuite) TestAppliedLXDProfileNamesNoErrorEmpty(c *tc.C) {
	machineUUID, _ := s.addMachine(c)
	err := s.state.SetMachineCloudInstance(c.Context(), machineUUID.String(), instance.Id("123"), "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
	profiles, err := s.state.AppliedLXDProfileNames(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(profiles, tc.HasLen, 0)
}

func (s *stateSuite) TestGetNamesForUUIDs(c *tc.C) {
	// Arrange
	machineUUID0, machineName0 := s.addMachine(c)
	machineUUID1, machineName1 := s.addMachine(c)
	machineUUID2, machineName2 := s.addMachine(c)
	expected := map[machine.UUID]machine.Name{
		machineUUID0: machineName0,
		machineUUID1: machineName1,
		machineUUID2: machineName2,
	}

	// Act
	obtained, err := s.state.GetNamesForUUIDs(c.Context(), []string{machineUUID0.String(), machineUUID1.String(), machineUUID2.String()})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained, tc.DeepEquals, expected)
}

func (s *stateSuite) TestGetNamesForUUIDsNotFound(c *tc.C) {
	// Act
	_, err := s.state.GetNamesForUUIDs(c.Context(), []string{"666"})

	// Assert
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestGetAllProvisionedMachineInstanceID(c *tc.C) {
	machineInstances, err := s.state.GetAllProvisionedMachineInstanceID(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machineInstances, tc.HasLen, 0)

	machineUUID, machineName := s.addMachine(c)

	machineInstances, err = s.state.GetAllProvisionedMachineInstanceID(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machineInstances, tc.HasLen, 0)

	err = s.state.SetMachineCloudInstance(c.Context(), machineUUID.String(), instance.Id("123"), "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	machineInstances, err = s.state.GetAllProvisionedMachineInstanceID(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machineInstances, tc.DeepEquals, map[machine.Name]string{
		machineName: "123",
	})
}

func (s *stateSuite) TestGetAllProvisionedMachineInstanceIDContainer(c *tc.C) {
	_, mNames, err := s.state.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Directive: deployment.Placement{
			Type: deployment.PlacementTypeContainer,
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	parentUUID, err := s.state.GetMachineUUID(c.Context(), mNames[0])
	c.Assert(err, tc.ErrorIsNil)
	childUUID, err := s.state.GetMachineUUID(c.Context(), mNames[1])
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetMachineCloudInstance(c.Context(), childUUID.String(), instance.Id("123"), "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SetMachineCloudInstance(c.Context(), parentUUID.String(), instance.Id("124"), "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	machineInstances, err := s.state.GetAllProvisionedMachineInstanceID(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machineInstances, tc.DeepEquals, map[machine.Name]string{
		mNames[0]: "124",
	})
}

func (s *stateSuite) TestSetMachineHostname(c *tc.C) {
	machineUUID, machineName := s.addMachine(c)

	err := s.state.SetMachineHostname(c.Context(), machineUUID.String(), "my-hostname")
	c.Assert(err, tc.ErrorIsNil)

	var hostname string
	err = s.TxnRunner().StdTxn(c.Context(), func(c context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(c, "SELECT hostname FROM machine WHERE name = ?", machineName).Scan(&hostname)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(hostname, tc.Equals, "my-hostname")
}

func (s *stateSuite) TestSetMachineHostnameEmpty(c *tc.C) {
	machineUUID, machineName := s.addMachine(c)

	err := s.state.SetMachineHostname(c.Context(), machineUUID.String(), "")
	c.Assert(err, tc.ErrorIsNil)

	var hostname *string
	err = s.TxnRunner().StdTxn(c.Context(), func(c context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(c, "SELECT hostname FROM machine WHERE name = ?", machineName).Scan(&hostname)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(hostname, tc.IsNil)
}

func (s *stateSuite) TestSetMachineHostnameNoMachine(c *tc.C) {
	err := s.state.SetMachineHostname(c.Context(), "666", "my-hostname")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestGetSupportedContainersTypes(c *tc.C) {
	machineUUID, _ := s.addMachine(c)

	containerTypes, err := s.state.GetSupportedContainersTypes(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(containerTypes, tc.DeepEquals, []string{"lxd"})
}

func (s *stateSuite) TestGetSupportedContainersTypesNoMachine(c *tc.C) {
	_, err := s.state.GetSupportedContainersTypes(c.Context(), "666")
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

func (s *stateSuite) TestGetMachineContainersNotContainers(c *tc.C) {
	// Arrange:
	_, mNames, err := s.state.AddMachine(c.Context(), domainmachine.AddMachineArgs{})
	c.Assert(err, tc.ErrorIsNil)
	mUUID, err := s.state.GetMachineUUID(c.Context(), mNames[0])
	c.Assert(err, tc.ErrorIsNil)

	// Act:
	containers, err := s.state.GetMachineContainers(c.Context(), mUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(containers, tc.HasLen, 0)
}

func (s *stateSuite) TestGetMachineContainersNotFound(c *tc.C) {
	_, err := s.state.GetMachineContainers(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestGetMachineContainers(c *tc.C) {
	// Arrange:
	_, mNames, err := s.state.AddMachine(c.Context(), domainmachine.AddMachineArgs{})
	c.Assert(err, tc.ErrorIsNil)
	mName := mNames[0]
	mUUID, err := s.state.GetMachineUUID(c.Context(), mName)
	c.Assert(err, tc.ErrorIsNil)

	_, _, err = s.state.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Directive: deployment.Placement{
			Type:      deployment.PlacementTypeContainer,
			Directive: mName.String(),
			Container: deployment.ContainerTypeLXD,
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	_, _, err = s.state.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Directive: deployment.Placement{
			Type:      deployment.PlacementTypeContainer,
			Directive: mName.String(),
			Container: deployment.ContainerTypeLXD,
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	// Act:
	res, err := s.state.GetMachineContainers(c.Context(), mUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.SameContents, []string{"0/lxd/0", "0/lxd/1"})
}

func (s *stateSuite) TestGetMachineContainersOnContainer(c *tc.C) {
	// Arrange:
	_, mNames, err := s.state.AddMachine(c.Context(), domainmachine.AddMachineArgs{})
	c.Assert(err, tc.ErrorIsNil)
	mName := mNames[0]
	c.Assert(err, tc.ErrorIsNil)

	_, _, err = s.state.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Directive: deployment.Placement{
			Type:      deployment.PlacementTypeContainer,
			Directive: mName.String(),
			Container: deployment.ContainerTypeLXD,
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	cUUID, err := s.state.GetMachineUUID(c.Context(), "0/lxd/0")
	c.Assert(err, tc.ErrorIsNil)

	// Act:
	res, err := s.state.GetMachineContainers(c.Context(), cUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.HasLen, 0)
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

func (s *stateSuite) addUnit(c *tc.C) machine.Name {
	machineName := machine.Name("1")
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		netNodeUUID := uuid.MustNewUUID().String()
		_, err := tx.ExecContext(ctx, `
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
`, "unit-uuid-1", "foo/1", "0", netNodeUUID, "app-uuid")
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO machine (uuid, name, life_id, net_node_uuid)
SELECT ?, ?, ?, net_node_uuid
FROM unit
WHERE uuid = ?
`, "machine-uuid-1", machineName, 0, "unit-uuid-1")
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	return machineName
}

func (s *stateSuite) TestGetMachinePlacementDirective(c *tc.C) {
	machineUUID, machineName := s.addMachine(c)

	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return insertMachineProviderPlacement(c.Context(), tx, s.state, machineUUID.String(), "0/lxd/42")
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
	networkstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c)).AddSpace(c.Context(), "space-uuid-0", "space0", "", []string{})
	networkstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c)).AddSpace(c.Context(), "space-uuid-1", "space1", "", []string{})

	_, machineNames, err := s.state.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:       deployment.Ubuntu,
			Channel:      "22.04/stable",
			Architecture: architecture.AMD64,
		},
		Constraints: constraints.Constraints{
			Arch:             ptr("amd64"),
			CpuCores:         ptr(uint64(2)),
			CpuPower:         ptr(uint64(42)),
			Mem:              ptr(uint64(8)),
			RootDisk:         ptr(uint64(256)),
			RootDiskSource:   ptr("root-disk-source"),
			InstanceRole:     ptr("instance-role"),
			InstanceType:     ptr("instance-type"),
			Container:        ptr(instance.LXD),
			VirtType:         ptr("virt-type"),
			AllocatePublicIP: ptr(true),
			ImageID:          ptr("image-id"),
			Tags:             ptr([]string{"tag0", "tag1"}),
			Spaces: ptr([]constraints.SpaceConstraint{
				{SpaceName: "space0", Exclude: false},
				{SpaceName: "space1", Exclude: true},
			}),
			Zones: ptr([]string{"zone0", "zone1"}),
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineName := machineNames[0]

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
	_, machineNames, err := s.state.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:       deployment.Ubuntu,
			Channel:      "22.04/stable",
			Architecture: architecture.AMD64,
		},
		Constraints: constraints.Constraints{
			Arch:             ptr("amd64"),
			CpuCores:         ptr(uint64(2)),
			AllocatePublicIP: ptr(true),
			ImageID:          ptr("image-id"),
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineName := machineNames[0]

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
	_, machineNames, err := s.state.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:       deployment.Ubuntu,
			Channel:      "22.04/stable",
			Architecture: architecture.AMD64,
		},
		Constraints: constraints.Constraints{
			CpuCores: ptr(uint64(2)),
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineName := machineNames[0]

	cons, err := s.state.GetMachineConstraints(c.Context(), machineName.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cons, tc.DeepEquals, constraints.Constraints{
		CpuCores: ptr(uint64(2)),
	})
}

func (s *stateSuite) TestConstraintEmpty(c *tc.C) {
	_, machineName := s.addMachine(c)

	cons, err := s.state.GetMachineConstraints(c.Context(), machineName.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cons, tc.DeepEquals, constraints.Constraints{})
}

func (s *stateSuite) TestConstraintsApplicationNotFound(c *tc.C) {
	_, err := s.state.GetMachineConstraints(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestGetMachineBase(c *tc.C) {
	_, machineNames, err := s.state.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:       deployment.Ubuntu,
			Channel:      "22.04/stable",
			Architecture: architecture.AMD64,
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineName := machineNames[0]

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

func (s *stateSuite) TestSetModelConstraints(c *tc.C) {
	s.createTestModel(c)

	runner := s.TxnRunnerFactory()
	state := modelstate.NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := s.DB().ExecContext(c.Context(), `
INSERT INTO space (uuid, name) VALUES
	(?, ?),
	(?, ?)`,
		uuid.MustNewUUID().String(), "space1",
		uuid.MustNewUUID().String(), "space2",
	)
	c.Assert(err, tc.ErrorIsNil)

	cons := constraints.Constraints{
		Arch:           ptr("amd64"),
		Container:      ptr(instance.LXD),
		CpuCores:       ptr(uint64(4)),
		Mem:            ptr(uint64(1024)),
		RootDisk:       ptr(uint64(1024)),
		RootDiskSource: ptr("root-disk-source"),
		Tags:           ptr([]string{"tag1", "tag2"}),
		InstanceRole:   ptr("instance-role"),
		InstanceType:   ptr("instance-type"),
		Spaces: ptr([]constraints.SpaceConstraint{
			{SpaceName: "space1", Exclude: false},
		}),
		VirtType:         ptr("virt-type"),
		Zones:            ptr([]string{"zone1", "zone2"}),
		AllocatePublicIP: ptr(true),
		ImageID:          ptr("image-id"),
	}

	err = state.SetModelConstraints(c.Context(), cons)
	c.Assert(err, tc.ErrorIsNil)

	getCons, err := state.GetModelConstraints(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(getCons, tc.DeepEquals, cons)
}

func (s *stateSuite) TestGetModelConstraintsNotFound(c *tc.C) {
	s.createTestModel(c)

	runner := s.TxnRunnerFactory()
	state := modelstate.NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := state.GetModelConstraints(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.ConstraintsNotFound)
}

func (s *stateSuite) TestGetModelConstraintsModelNotFound(c *tc.C) {
	runner := s.TxnRunnerFactory()
	state := modelstate.NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := state.GetModelConstraints(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *stateSuite) TestCountMachinesInSpace(c *tc.C) {
	// Setup: Create a space and subnet
	spaceUUID := network.SpaceUUID(uuid.MustNewUUID().String())
	networkState := networkstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err := networkState.AddSpace(c.Context(), spaceUUID, "test-space", "provider-space-id", []string{})
	c.Assert(err, tc.ErrorIsNil)
	subnetID := network.Id(uuid.MustNewUUID().String())
	err = networkState.AddSubnet(c.Context(), network.SubnetInfo{
		ID:      subnetID,
		SpaceID: spaceUUID,
		CIDR:    "10.0.0.0/24",
	})
	c.Assert(err, tc.ErrorIsNil)

	mUUID, _ := s.addMachine(c)
	nnUUID, err := networkState.GetMachineNetNodeUUID(c.Context(), mUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	devName := "eth0"
	err = networkState.SetMachineNetConfig(c.Context(), nnUUID, []domainnetwork.NetInterface{{
		Name:            devName,
		Type:            network.EthernetDevice,
		VirtualPortType: network.NonVirtualPort,
		IsAutoStart:     true,
		IsEnabled:       true,
		Addrs: []domainnetwork.NetAddr{{
			InterfaceName:    devName,
			AddressValue:     "10.0.0.42/24",
			AddressType:      network.IPv4Address,
			ConfigType:       network.ConfigDHCP,
			Origin:           network.OriginMachine,
			Scope:            network.ScopeCloudLocal,
			ProviderSubnetID: &subnetID,
		}},
	}})
	c.Assert(err, tc.ErrorIsNil)

	count, err := s.state.CountMachinesInSpace(c.Context(), spaceUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, int64(1))
}

func (s *stateSuite) TestCountMachinesInSpaceDoubleAddressSameMachine(c *tc.C) {
	// Setup: Create a space and subnet
	spaceUUID := network.SpaceUUID(uuid.MustNewUUID().String())
	networkState := networkstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err := networkState.AddSpace(c.Context(), spaceUUID, "test-space", "provider-space-id", []string{})
	c.Assert(err, tc.ErrorIsNil)
	subnetID := network.Id(uuid.MustNewUUID().String())
	err = networkState.AddSubnet(c.Context(), network.SubnetInfo{
		ID:      subnetID,
		SpaceID: spaceUUID,
		CIDR:    "10.0.0.0/24",
	})
	c.Assert(err, tc.ErrorIsNil)

	mUUID, _ := s.addMachine(c)
	nnUUID, err := networkState.GetMachineNetNodeUUID(c.Context(), mUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	devName := "eth0"
	err = networkState.SetMachineNetConfig(c.Context(), nnUUID, []domainnetwork.NetInterface{{
		Name:            devName,
		Type:            network.EthernetDevice,
		VirtualPortType: network.NonVirtualPort,
		IsAutoStart:     true,
		IsEnabled:       true,
		Addrs: []domainnetwork.NetAddr{
			{
				InterfaceName:    devName,
				AddressValue:     "10.0.0.42/24",
				AddressType:      network.IPv4Address,
				ConfigType:       network.ConfigDHCP,
				Origin:           network.OriginMachine,
				Scope:            network.ScopeCloudLocal,
				ProviderSubnetID: &subnetID,
			},
			{
				InterfaceName:    devName,
				AddressValue:     "10.0.0.255/24",
				AddressType:      network.IPv4Address,
				ConfigType:       network.ConfigDHCP,
				Origin:           network.OriginMachine,
				Scope:            network.ScopeCloudLocal,
				ProviderSubnetID: &subnetID,
			},
		},
	}})
	c.Assert(err, tc.ErrorIsNil)

	count, err := s.state.CountMachinesInSpace(c.Context(), spaceUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, int64(1))
}

func (s *stateSuite) TestCountMachinesInSpaceMultipleSubnets(c *tc.C) {
	// Setup: Create a space with multiple subnets
	spaceUUID := network.SpaceUUID(uuid.MustNewUUID().String())
	networkState := networkstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err := networkState.AddSpace(c.Context(), spaceUUID, "multi-subnet-space", "provider-space-id", []string{})
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID0 := network.Id(uuid.MustNewUUID().String())
	subnetUUID1 := network.Id(uuid.MustNewUUID().String())
	err = networkState.AddSubnet(c.Context(), network.SubnetInfo{
		ID:         subnetUUID0,
		SpaceID:    spaceUUID,
		ProviderId: network.Id(uuid.MustNewUUID().String()),
		CIDR:       "10.0.0.0/24",
	})
	c.Assert(err, tc.ErrorIsNil)
	// Second subnet, not in the same CIDR as the IP address we are going to
	// add later.
	err = networkState.AddSubnet(c.Context(), network.SubnetInfo{
		ID:         subnetUUID1,
		SpaceID:    spaceUUID,
		ProviderId: network.Id(uuid.MustNewUUID().String()),
		CIDR:       "192.168.0.0/24",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Create machines and assign them to different subnets
	machineUUIDs := []string{}
	for range 3 {
		mUUID, _ := s.addMachine(c)
		machineUUIDs = append(machineUUIDs, mUUID.String())
	}

	// Assign network configurations to machines
	for i, mUUID := range machineUUIDs {
		nnUUID, err := networkState.GetMachineNetNodeUUID(c.Context(), mUUID)
		c.Assert(err, tc.ErrorIsNil)
		devName := "eth0"
		err = networkState.SetMachineNetConfig(c.Context(), nnUUID, []domainnetwork.NetInterface{{
			Name:            devName,
			Type:            network.EthernetDevice,
			VirtualPortType: network.NonVirtualPort,
			IsAutoStart:     true,
			IsEnabled:       true,
			Addrs: []domainnetwork.NetAddr{{
				InterfaceName:    devName,
				AddressValue:     fmt.Sprintf("10.0.0.%d/24", i),
				AddressType:      network.IPv4Address,
				ConfigType:       network.ConfigDHCP,
				Origin:           network.OriginMachine,
				Scope:            network.ScopeCloudLocal,
				ProviderSubnetID: &subnetUUID0,
			}},
		}})
		c.Assert(err, tc.ErrorIsNil)
	}

	// Test counting machines in space with multiple subnets
	count, err := s.state.CountMachinesInSpace(c.Context(), spaceUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, int64(3))
}

func (s *stateSuite) TestIsContainer(c *tc.C) {
	_, mNames, err := s.state.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Directive: deployment.Placement{
			Type:      deployment.PlacementTypeContainer,
			Container: deployment.ContainerTypeLXD,
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	isContainerParent, err := s.state.IsContainer(c.Context(), mNames[0])
	c.Assert(err, tc.ErrorIsNil)
	isContainerChild, err := s.state.IsContainer(c.Context(), mNames[1])
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isContainerParent, tc.Equals, false)
	c.Check(isContainerChild, tc.Equals, true)
}

func (s *stateSuite) TestIsContainerWithNoParent(c *tc.C) {
	_, mName := s.addMachine(c)

	isContainer, err := s.state.IsContainer(c.Context(), mName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isContainer, tc.Equals, false)
}

func (s *stateSuite) TestIsContainerNotExists(c *tc.C) {
	_, err := s.state.IsContainer(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) addMachine(c *tc.C) (machine.UUID, machine.Name) {
	_, mNames, err := s.state.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineUUID, err := s.state.GetMachineUUID(c.Context(), mNames[0])
	c.Assert(err, tc.ErrorIsNil)
	return machineUUID, mNames[0]
}

func (s *stateSuite) createTestModel(c *tc.C) coremodel.UUID {
	runner := s.TxnRunnerFactory()
	state := modelstate.NewModelState(runner, loggertesting.WrapCheckLog(c))

	id := modeltesting.GenModelUUID(c)
	args := model.ModelDetailArgs{
		UUID:            id,
		AgentStream:     modelagent.AgentStreamReleased,
		AgentVersion:    jujuversion.Current,
		ControllerUUID:  uuid.MustNewUUID(),
		Name:            "my-awesome-model",
		Qualifier:       "prod",
		Type:            coremodel.IAAS,
		Cloud:           "aws",
		CloudType:       "ec2",
		CloudRegion:     "myregion",
		CredentialOwner: usertesting.GenNewName(c, "myowner"),
		CredentialName:  "mycredential",
	}
	err := state.Create(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	return id
}
