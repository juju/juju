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
	"github.com/juju/juju/domain/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
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

func (s *migrationStateSuite) TestCreateMachine(c *tc.C) {
	_, err := s.state.CreateMachine(c.Context(), machine.CreateMachineArgs{})
	c.Assert(err, tc.ErrorIsNil)

	machines, err := s.state.GetMachinesForExport(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machines, tc.HasLen, 0)
}

func (s *migrationStateSuite) TestCreateMachineAfterProvisionedNonce(c *tc.C) {
	_, err := s.state.CreateMachine(c.Context(), machine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetMachineCloudInstance(c.Context(), "deadbeef", "foo", "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(c context.Context, tx *sql.Tx) error {
		tx.ExecContext(c, `UPDATE machine SET password_hash = 'ssssh!' WHERE uuid = 'deadbeef'`)
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	machines, err := s.state.GetMachinesForExport(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machines, tc.HasLen, 1)
	c.Check(machines, tc.DeepEquals, []machine.ExportMachine{
		{
			Name:  coremachine.Name("0"),
			UUID:  "deadbeef",
			Nonce: "nonce",
		},
	})
}

func (s *migrationStateSuite) TestCreateMachineAfterProvisionedNoNonce(c *tc.C) {
	_, err := s.state.CreateMachine(c.Context(), machine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetMachineCloudInstance(c.Context(), "deadbeef", "foo", "", "", nil)
	c.Assert(err, tc.ErrorIsNil)

	machines, err := s.state.GetMachinesForExport(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machines, tc.HasLen, 1)
	c.Check(machines, tc.DeepEquals, []machine.ExportMachine{
		{
			Name:  coremachine.Name("0"),
			UUID:  "deadbeef",
			Nonce: "",
		},
	})
}

func (s *migrationStateSuite) TestInsertImportingMachineAlreadyExists(c *tc.C) {
	machineName, err := s.state.CreateMachine(c.Context(), machine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.InsertMigratingMachine(c.Context(), machineName.String(), machine.CreateMachineArgs{
		MachineUUID: "deadbeef",
	})
	c.Assert(err, tc.ErrorIs, machineerrors.MachineAlreadyExists)
}
