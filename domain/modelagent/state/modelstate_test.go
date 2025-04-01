// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	corearch "github.com/juju/juju/core/arch"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	machinestate "github.com/juju/juju/domain/machine/state"
	modelerrors "github.com/juju/juju/domain/model/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type modelStateSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&modelStateSuite{})

// TestCheckMachineDoesNotExist is asserting that if no machine exists we get
// back an error satisfying [machineerrors.MachineNotFound].
func (s *modelStateSuite) TestCheckMachineDoesNotExist(c *gc.C) {
	err := NewState(s.TxnRunnerFactory()).CheckMachineExists(
		context.Background(),
		machine.Name("0"),
	)
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestCheckUnitDoesNotExist is asserting that if no unit exists we get back an
// error satisfying [applicationerrors.UnitNotFound].
func (s *modelStateSuite) TestCheckUnitDoesNotExist(c *gc.C) {
	err := NewState(s.TxnRunnerFactory()).CheckUnitExists(
		context.Background(),
		"foo/0",
	)
	c.Check(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestGetModelAgentVersionSuccess tests that State.GetModelAgentVersion is
// correct in the expected case when the model exists.
func (s *modelStateSuite) TestGetModelAgentVersionSuccess(c *gc.C) {
	expectedVersion, err := semversion.Parse("4.21.54")
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	s.setAgentVersion(c, expectedVersion.String())

	obtainedVersion, err := st.GetTargetAgentVersion(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(obtainedVersion, jc.DeepEquals, expectedVersion)
}

// TestGetModelAgentVersionModelNotFound tests that State.GetModelAgentVersion
// returns modelerrors.NotFound when the model does not exist in the DB.
func (s *modelStateSuite) TestGetModelAgentVersionModelNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetTargetAgentVersion(context.Background())
	c.Check(err, jc.ErrorIs, modelerrors.AgentVersionNotFound)
}

// TestGetModelAgentVersionCantParseVersion tests that State.GetModelAgentVersion
// returns an appropriate error when the agent version in the DB is invalid.
func (s *modelStateSuite) TestGetModelAgentVersionCantParseVersion(c *gc.C) {
	s.setAgentVersion(c, "invalid-version")

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetTargetAgentVersion(context.Background())
	c.Check(err, gc.ErrorMatches, `parsing agent version: invalid version "invalid-version".*`)
}

// Set the agent version for the given model in the DB.
func (s *modelStateSuite) setAgentVersion(c *gc.C, vers string) {
	db, err := domain.NewStateBase(s.TxnRunnerFactory()).DB()
	c.Assert(err, jc.ErrorIsNil)

	q := "INSERT INTO agent_version (target_version) values ($M.target_version)"

	args := sqlair.M{"target_version": vers}
	stmt, err := sqlair.Prepare(q, args)
	c.Assert(err, jc.ErrorIsNil)

	err = db.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, args).Run()
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelStateSuite) createMachine(c *gc.C) string {
	machineSt := machinestate.NewState(
		s.TxnRunnerFactory(),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	uuid, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = machineSt.CreateMachine(context.Background(), "666", "", uuid.String())
	c.Assert(err, jc.ErrorIsNil)

	return uuid.String()
}

// TestSetRunningAgentBinaryVersionSuccess asserts that if we attempt to set the
// running agent binary version for a machine that doesn't exist we get back
// an error that satisfies [machineerrors.MachineNotFound].
func (s *modelStateSuite) TestSetRunningAgentBinaryVersionMachineNotFound(c *gc.C) {
	machineUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	err = st.SetMachineRunningAgentBinaryVersion(
		context.Background(),
		machineUUID.String(),
		coreagentbinary.Version{
			Number: jujuversion.Current,
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *modelStateSuite) TestSetRunningAgentBinaryVersionMachineDead(c *gc.C) {
	machineUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	machineSt := machinestate.NewState(
		s.TxnRunnerFactory(),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	err = machineSt.CreateMachine(context.Background(), "666", "", machineUUID.String())
	c.Assert(err, jc.ErrorIsNil)

	err = machineSt.SetMachineLife(context.Background(), "666", life.Dead)
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	err = st.SetMachineRunningAgentBinaryVersion(
		context.Background(),
		machineUUID.String(),
		coreagentbinary.Version{
			Number: jujuversion.Current,
			Arch:   corearch.Arch("noexist"),
		},
	)
	c.Check(err, jc.ErrorIs, machineerrors.MachineIsDead)
}

// TestSetRunningAgentBinaryVersionNotSupportedArch tests that if we provide an
// architecture that isn't supported by the database we get back an error
// that satisfies [coreerrors.NotValid].
func (s *modelStateSuite) TestSetRunningAgentBinaryVersionNotSupportedArch(c *gc.C) {
	machineUUID := s.createMachine(c)

	st := NewState(s.TxnRunnerFactory())
	err := st.SetMachineRunningAgentBinaryVersion(
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
func (s *modelStateSuite) TestSetRunningAgentBinaryVersion(c *gc.C) {
	machineUUID := s.createMachine(c)

	st := NewState(s.TxnRunnerFactory())
	err := st.SetMachineRunningAgentBinaryVersion(
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
func (s *modelStateSuite) TestSetRunningAgentBinaryVersionUpdate(c *gc.C) {
	machineUUID := s.createMachine(c)

	st := NewState(s.TxnRunnerFactory())
	err := st.SetMachineRunningAgentBinaryVersion(
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
	err = st.SetMachineRunningAgentBinaryVersion(
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
