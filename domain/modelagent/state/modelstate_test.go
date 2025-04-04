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
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	machinestate "github.com/juju/juju/domain/machine/state"
	modelagenterrors "github.com/juju/juju/domain/modelagent/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type modelStateSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&modelStateSuite{})

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

	st := NewState(s.TxnRunnerFactory())
	machineUUID, err := st.GetMachineUUID(context.Background(), machine.Name("666"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineUUID, gc.Equals, uuid.String())

	return uuid.String()
}

// Set the agent version for the given model in the DB.
func (s *modelStateSuite) setModelAgentVersion(c *gc.C, vers string) {
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

func (s *modelStateSuite) setMachineAgentVersion(c *gc.C, machineUUID, target, running string) {
	s.setModelAgentVersion(c, target)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO machine_agent_version (machine_uuid, version, architecture_id) values (?, ?, ?)",
			machineUUID, running, 0)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelStateSuite) setUnitAgentVersion(c *gc.C, unitUUID, target, running string) {
	s.setModelAgentVersion(c, target)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO unit_agent_version (unit_uuid, version, architecture_id) values (?, ?, ?)",
			unitUUID, running, 0)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelStateSuite) createTestingUnit(
	c *gc.C,
) coreunit.UUID {
	appState := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	platform := application.Platform{
		Channel:      "22.04/stable",
		OSType:       application.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &application.Channel{
		Track:  "track",
		Risk:   "stable",
		Branch: "branch",
	}
	ctx := context.Background()

	appID, err := appState.CreateApplication(ctx, "foo", application.AddApplicationArg{
		Platform: platform,
		Channel:  channel,
		Charm: charm.Charm{
			Metadata: charm.Metadata{
				Name: "foo",
				Provides: map[string]charm.Relation{
					"endpoint": {
						Name:  "endpoint",
						Role:  charm.RoleProvider,
						Scope: charm.ScopeGlobal,
					},
					"misc": {
						Name:  "misc",
						Role:  charm.RoleProvider,
						Scope: charm.ScopeGlobal,
					},
				},
			},
			Manifest: charm.Manifest{
				Bases: []charm.Base{
					{
						Name: "ubuntu",
						Channel: charm.Channel{
							Risk: charm.RiskStable,
						},
						Architectures: []string{"amd64"},
					},
				},
			},
			ReferenceName: "foo",
			Source:        charm.CharmHubSource,
			Revision:      42,
			Hash:          "hash",
		},
		CharmDownloadInfo: &charm.DownloadInfo{
			Provenance:         charm.ProvenanceDownload,
			CharmhubIdentifier: "ident",
			DownloadURL:        "https://example.com",
			DownloadSize:       42,
		},
		Scale: 1,
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	unitName := coreunit.Name("foo/123")
	appState.AddIAASUnits(ctx, "", appID, application.AddUnitArg{
		UnitName: unitName,
	})

	st := NewState(s.TxnRunnerFactory())
	unitUUID, err := st.GetUnitUUIDByName(context.Background(), unitName)
	c.Check(err, jc.ErrorIsNil)

	return unitUUID
}

// TestGetModelAgentVersionSuccess tests that State.GetModelAgentVersion is
// correct in the expected case when the model exists.
func (s *modelStateSuite) TestGetModelAgentVersionSuccess(c *gc.C) {
	expectedVersion, err := semversion.Parse("4.21.54")
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	s.setModelAgentVersion(c, expectedVersion.String())

	obtainedVersion, err := st.GetModelTargetAgentVersion(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(obtainedVersion, jc.DeepEquals, expectedVersion)
}

// TestGetModelAgentVersionModelNotFound tests that State.GetModelAgentVersion
// returns modelerrors.NotFound when the model does not exist in the DB.
func (s *modelStateSuite) TestGetModelAgentVersionModelNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetModelTargetAgentVersion(context.Background())
	c.Check(err, jc.ErrorIs, modelagenterrors.AgentVersionNotFound)
}

// TestGetModelAgentVersionCantParseVersion tests that State.GetModelAgentVersion
// returns an appropriate error when the agent version in the DB is invalid.
func (s *modelStateSuite) TestGetModelAgentVersionCantParseVersion(c *gc.C) {
	s.setModelAgentVersion(c, "invalid-version")

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetModelTargetAgentVersion(context.Background())
	c.Check(err, gc.ErrorMatches, `parsing agent version: invalid version "invalid-version".*`)
}

func (s *modelStateSuite) TestGetMachineTargetAgentBinaryVersion(c *gc.C) {
	machineUUID := s.createMachine(c)
	s.setMachineAgentVersion(c, machineUUID, "4.0.1", "4.0.0")

	st := NewState(s.TxnRunnerFactory())
	vers, err := st.GetMachineTargetAgentVersion(
		context.Background(),
		machineUUID,
	)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(vers, jc.DeepEquals, coreagentbinary.Version{
		Number: semversion.MustParse("4.0.1"),
		Arch:   "amd64",
	})
}

func (s *modelStateSuite) TestGetMachineAgentVersionCantParseVersion(c *gc.C) {
	machineUUID := s.createMachine(c)
	s.setMachineAgentVersion(c, machineUUID, "invalid-version", "4.0.0")

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetMachineTargetAgentVersion(context.Background(), machineUUID)
	c.Check(err, gc.ErrorMatches, `parsing machine agent version: invalid version "invalid-version".*`)
}

func (s *modelStateSuite) TestGetMachineAgentVersionNotFound(c *gc.C) {
	machineUUID := s.createMachine(c)

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetMachineTargetAgentVersion(context.Background(), machineUUID)
	c.Check(err, gc.ErrorMatches, `agent version not found`)
}

func (s *modelStateSuite) TestGetUnitTargetAgentBinaryVersion(c *gc.C) {
	unitUUID := s.createTestingUnit(c)
	s.setUnitAgentVersion(c, unitUUID.String(), "4.0.1", "4.0.0")

	st := NewState(s.TxnRunnerFactory())
	vers, err := st.GetUnitTargetAgentVersion(
		context.Background(),
		unitUUID,
	)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(vers, jc.DeepEquals, coreagentbinary.Version{
		Number: semversion.MustParse("4.0.1"),
		Arch:   "amd64",
	})
}

func (s *modelStateSuite) TestGetUnitAgentVersionCantParseVersion(c *gc.C) {
	unitUUID := s.createTestingUnit(c)
	s.setUnitAgentVersion(c, unitUUID.String(), "invalid-version", "4.0.0")

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetUnitTargetAgentVersion(context.Background(), unitUUID)
	c.Check(err, gc.ErrorMatches, `parsing unit agent version: invalid version "invalid-version".*`)
}

func (s *modelStateSuite) TestGetUnitAgentVersionNotFound(c *gc.C) {
	unitUUID := s.createTestingUnit(c)

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetUnitTargetAgentVersion(context.Background(), unitUUID)
	c.Check(err, gc.ErrorMatches, `agent version not found`)
}

// TestSetMachineRunningAgentBinaryVersionSuccess asserts that if we attempt to
// set the running agent binary version for a machine that doesn't exist we get
// back an error that satisfies [machineerrors.MachineNotFound].
func (s *modelStateSuite) TestSetMachineRunningAgentBinaryVersionMachineNotFound(c *gc.C) {
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

func (s *modelStateSuite) TestMachineSetRunningAgentBinaryVersionMachineDead(c *gc.C) {
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

// TestSetMachineRunningAgentBinaryVersionNotSupportedArch tests that if we provide an
// architecture that isn't supported by the database we get back an error
// that satisfies [coreerrors.NotValid].
func (s *modelStateSuite) TestSetMachineRunningAgentBinaryVersionNotSupportedArch(c *gc.C) {
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

// TestSetMachineRunningAgentBinaryVersion asserts setting the initial agent
// binary version (happy path).
func (s *modelStateSuite) TestSetMachineRunningAgentBinaryVersion(c *gc.C) {
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

// TestSetMachineRunningAgentBinaryVersion asserts setting the initial agent
// binary version (happy path) and then updating the value.
func (s *modelStateSuite) TestSetMachineRunningAgentBinaryVersionUpdate(c *gc.C) {
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

// TestSetUnitRunningAgentBinaryVersionUnitNotFound asserts that if we attempt to set the
// running agent binary version for a unit that doesn't exist we get back
// an error that satisfies [applicationerrors.UnitNotFound].
func (s *modelStateSuite) TestSetUnitRunningAgentBinaryVersionUnitNotFound(c *gc.C) {
	unitUUID := unittesting.GenUnitUUID(c)

	st := NewState(s.TxnRunnerFactory())
	err := st.SetUnitRunningAgentBinaryVersion(
		context.Background(),
		unitUUID,
		coreagentbinary.Version{
			Number: jujuversion.Current,
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestSetUnitRunningAgentBinaryVersionNotSupportedArch tests that if we provide
// an architecture that isn't supported by the database we get back an error
// that satisfies [coreerrors.NotSupported].
func (s *modelStateSuite) TestSetUnitRunningAgentBinaryVersionNotSupportedArch(c *gc.C) {
	unitUUID := s.createTestingUnit(c)
	st := NewState(s.TxnRunnerFactory())
	err := st.SetUnitRunningAgentBinaryVersion(
		context.Background(),
		unitUUID,
		coreagentbinary.Version{
			Number: jujuversion.Current,
			Arch:   corearch.Arch("noexist"),
		},
	)
	c.Check(err, jc.ErrorIs, coreerrors.NotSupported)
}

// TestSetUnitRunningAgentBinaryVersion asserts setting the initial agent binary
// version (happy path).
func (s *modelStateSuite) TestSetUnitRunningAgentBinaryVersion(c *gc.C) {
	unitUUID := s.createTestingUnit(c)
	st := NewState(s.TxnRunnerFactory())
	err := st.SetUnitRunningAgentBinaryVersion(
		context.Background(),
		unitUUID,
		coreagentbinary.Version{
			Number: jujuversion.Current,
			Arch:   corearch.ARM64,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	var (
		obtainedUnitUUID string
		obtainedVersion  string
		obtainedArch     string
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		stmt := `
SELECT unit_uuid,
       version,
       name
FROM unit_agent_version
INNER JOIN architecture ON unit_agent_version.architecture_id = architecture.id
WHERE unit_uuid = ?
	`

		return tx.QueryRowContext(ctx, stmt, unitUUID).Scan(
			&obtainedUnitUUID,
			&obtainedVersion,
			&obtainedArch,
		)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtainedUnitUUID, gc.Equals, unitUUID.String())
	c.Check(obtainedVersion, gc.Equals, jujuversion.Current.String())
	c.Check(obtainedArch, gc.Equals, corearch.ARM64)
}

// TestSetRunningAgentBinaryVersionUpdate asserts setting the initial agent binary
// version (happy path) and then updating the value.
func (s *modelStateSuite) TestSetUnitRunningAgentBinaryVersionUpdate(c *gc.C) {
	unitUUID := s.createTestingUnit(c)
	st := NewState(s.TxnRunnerFactory())
	err := st.SetUnitRunningAgentBinaryVersion(
		context.Background(),
		unitUUID,
		coreagentbinary.Version{
			Number: jujuversion.Current,
			Arch:   corearch.ARM64,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	var (
		obtainedUnitUUID string
		obtainedVersion  string
		obtainedArch     string
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		stmt := `
SELECT unit_uuid,
       version,
       name
FROM unit_agent_version
INNER JOIN architecture ON unit_agent_version.architecture_id = architecture.id
WHERE unit_uuid = ?
	`

		return tx.QueryRowContext(ctx, stmt, unitUUID).Scan(
			&obtainedUnitUUID,
			&obtainedVersion,
			&obtainedArch,
		)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtainedUnitUUID, gc.Equals, unitUUID.String())
	c.Check(obtainedVersion, gc.Equals, jujuversion.Current.String())

	// Update
	newVersion := jujuversion.Current
	newVersion.Patch++
	err = st.SetUnitRunningAgentBinaryVersion(
		context.Background(),
		unitUUID,
		coreagentbinary.Version{
			Number: newVersion,
			Arch:   corearch.ARM64,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		stmt := `
SELECT unit_uuid,
       version,
       name
FROM unit_agent_version
INNER JOIN architecture ON unit_agent_version.architecture_id = architecture.id
WHERE unit_uuid = ?
	`

		return tx.QueryRowContext(ctx, stmt, unitUUID).Scan(
			&obtainedUnitUUID,
			&obtainedVersion,
			&obtainedArch,
		)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtainedUnitUUID, gc.Equals, unitUUID.String())
	c.Check(obtainedVersion, gc.Equals, newVersion.String())
	c.Check(obtainedArch, gc.Equals, corearch.ARM64)
}
