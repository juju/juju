// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/network"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type migrationStateSuite struct {
	schematesting.ModelSuite

	state *State
}

func TestMigrationSuite(t *stdtesting.T) {
	tc.Run(t, &migrationStateSuite{})
}

func (s *migrationStateSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

func (s *migrationStateSuite) TestGetMachinesForExport(c *tc.C) {
	s.addMachine(c)

	machines, err := s.state.GetMachinesForExport(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machines, tc.HasLen, 0)
}

func (s *migrationStateSuite) TestGetMachinesForExportAfterProvisionedNonce(c *tc.C) {
	machineUUID, machineName := s.addMachine(c)

	err := s.state.SetMachineCloudInstance(c.Context(), machineUUID.String(), "foo", "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(c context.Context, tx *sql.Tx) error {
		tx.ExecContext(c, `UPDATE machine SET password_hash = 'ssssh!'`)
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	machines, err := s.state.GetMachinesForExport(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machines, tc.HasLen, 1)
	c.Check(machines, tc.DeepEquals, []machine.ExportMachine{
		{
			Name:         machineName,
			UUID:         machineUUID,
			Nonce:        "nonce",
			PasswordHash: "ssssh!",
			Placement:    "place it here",
			Base:         "ubuntu@24.04/stable",
			InstanceID:   "foo",
		},
	})
}

func (s *migrationStateSuite) TestGetMachinesForExportAfterProvisionedNoNonce(c *tc.C) {
	machineUUID, machineName := s.addMachine(c)

	err := s.state.SetMachineCloudInstance(c.Context(), machineUUID.String(), "foo", "", "", nil)
	c.Assert(err, tc.ErrorIsNil)

	machines, err := s.state.GetMachinesForExport(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machines, tc.HasLen, 1)
	c.Check(machines, tc.DeepEquals, []machine.ExportMachine{
		{
			Name:       machineName,
			UUID:       machineUUID,
			Nonce:      "",
			Placement:  "place it here",
			Base:       "ubuntu@24.04/stable",
			InstanceID: "foo",
		},
	})
}

func (s *migrationStateSuite) TestInsertImportingMachineAlreadyExists(c *tc.C) {
	machineUUID, machineName := s.addMachine(c)

	err := s.state.InsertMigratingMachine(c.Context(), machineName.String(), machine.CreateMachineArgs{
		MachineUUID: machineUUID,
	})
	c.Assert(err, tc.ErrorIs, machineerrors.MachineAlreadyExists)
}

func (s *migrationStateSuite) TestInsertMigratingMachine(c *tc.C) {
	netNodeUUID := tc.Must(c, network.NewNetNodeUUID)

	machineUUID := tc.Must(c, coremachine.NewUUID)

	err := s.state.InsertMigratingMachine(c.Context(), "777", machine.CreateMachineArgs{
		MachineUUID: machineUUID,
		NetNodeUUID: netNodeUUID,
		Hostname:    "host-name-123",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Verify the machine was created with the correct net node UUID.
	var retrievedNetNodeUUID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, "SELECT net_node_uuid FROM machine WHERE uuid = ?", machineUUID.String())
		return row.Scan(&retrievedNetNodeUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(retrievedNetNodeUUID, tc.Equals, netNodeUUID.String())
	s.checkHostnameForMachine(c, "777", "host-name-123")
}

func (s *migrationStateSuite) TestInsertMigratingSubordinateMachineAlreadyExists(c *tc.C) {
	machineUUID, machineName := s.addMachine(c)
	containerUUID, containerName := s.addSubordinateMachine(c, machineName)

	err := s.state.InsertMigratingSubordinateMachine(c.Context(), containerName.String(), machineUUID.String(), machine.CreateMachineArgs{
		MachineUUID: containerUUID,
	})
	c.Assert(err, tc.ErrorIs, machineerrors.MachineAlreadyExists)
}

func (s *migrationStateSuite) TestInsertMigratingSubordinateMachine(c *tc.C) {
	parentUUID, _ := s.addMachine(c)

	netNodeUUID := tc.Must(c, network.NewNetNodeUUID)

	containerUUID := tc.Must(c, coremachine.NewUUID)

	err := s.state.InsertMigratingSubordinateMachine(c.Context(), "0/lxd/888", parentUUID.String(), machine.CreateMachineArgs{
		MachineUUID: containerUUID,
		NetNodeUUID: netNodeUUID,
		Hostname:    "host-name-123",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Verify the machine was created with the correct net node UUID.
	var retrievedNetNodeUUID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, "SELECT net_node_uuid FROM machine WHERE uuid = ?", containerUUID.String())
		return row.Scan(&retrievedNetNodeUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(retrievedNetNodeUUID, tc.Equals, netNodeUUID.String())

	retrievedParentUUID, err := s.state.GetMachineParentUUID(c.Context(), containerUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(retrievedParentUUID, tc.Equals, parentUUID)
	s.checkHostnameForMachine(c, "0/lxd/888", "host-name-123")
}

func (s *migrationStateSuite) addMachine(c *tc.C) (coremachine.UUID, coremachine.Name) {
	_, mNames, err := s.state.AddMachine(c.Context(), machine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
		Directive: deployment.Placement{
			Type:      deployment.PlacementTypeProvider,
			Directive: "place it here",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineUUID, err := s.state.GetMachineUUID(c.Context(), mNames[0])
	c.Assert(err, tc.ErrorIsNil)
	return machineUUID, mNames[0]
}

func (s *migrationStateSuite) addSubordinateMachine(c *tc.C, parentName coremachine.Name) (coremachine.UUID, coremachine.Name) {
	_, mNames, err := s.state.AddMachine(c.Context(), machine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
		Directive: deployment.Placement{
			Type:      deployment.PlacementTypeContainer,
			Container: deployment.ContainerTypeLXD,
			Directive: parentName.String(),
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineUUID, err := s.state.GetMachineUUID(c.Context(), mNames[0])
	c.Assert(err, tc.ErrorIsNil)
	return machineUUID, mNames[0]
}

func (s *migrationStateSuite) checkHostnameForMachine(c *tc.C, name string, expected string) {
	var hostname string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRow(`
SELECT hostname
FROM machine
WHERE name = ?
`, name).Scan(&hostname)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(hostname, tc.Equals, expected)
}
