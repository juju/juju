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
	coreapplication "github.com/juju/juju/core/application"
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
	return s.createMachineWithName(c, machine.Name("6666"))
}

func (s *modelStateSuite) createMachineWithName(c *gc.C, name machine.Name) string {
	machineSt := machinestate.NewState(
		s.TxnRunnerFactory(),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	uuid, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = machineSt.CreateMachine(context.Background(), name, uuid.String(), uuid.String())
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	machineUUID, err := st.GetMachineUUIDByName(context.Background(), name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineUUID, gc.Equals, uuid.String())

	return uuid.String()
}

// Set the agent version for the given model in the DB.
func (s *modelStateSuite) setModelTargetAgentVersion(c *gc.C, vers string) {
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

func (s *modelStateSuite) setMachineAgentVersion(c *gc.C, machineUUID, running string) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO machine_agent_version (machine_uuid, version, architecture_id) values (?, ?, ?)",
			machineUUID, running, 0)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelStateSuite) setUnitAgentVersion(
	c *gc.C, unitUUID coreunit.UUID, running string,
) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO unit_agent_version (unit_uuid, version, architecture_id) values (?, ?, ?)",
			unitUUID.String(), running, 0)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelStateSuite) createTestingUnit(
	c *gc.C,
) coreunit.UUID {
	appID := s.createTestingApplicationWithName(c, "foo")
	return s.createTestingUnitWithName(c, "foo", appID, coreunit.Name("foo/1"))
}

func (s *modelStateSuite) createTestingApplicationWithName(
	c *gc.C,
	appName string,
) coreapplication.ID {
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
				Name: appName,
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
			ReferenceName: appName,
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
	return appID
}

func (s *modelStateSuite) createTestingUnitWithName(
	c *gc.C,
	appName string,
	appID coreapplication.ID,
	unitName coreunit.Name,
) coreunit.UUID {
	appState := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	charmUUID, err := appState.GetCharmIDByApplicationName(context.Background(), appName)
	c.Assert(err, jc.ErrorIsNil)
	appState.AddIAASUnits(context.Background(), "", appID, charmUUID, application.AddUnitArg{
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
	s.setModelTargetAgentVersion(c, expectedVersion.String())

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
	s.setModelTargetAgentVersion(c, "invalid-version")

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetModelTargetAgentVersion(context.Background())
	c.Check(err, gc.ErrorMatches, `parsing agent version: invalid version "invalid-version".*`)
}

// TestGetMachineTargetAgentVersion asserts the happy path of getting the target
// agent version for machine that exists.
func (s *modelStateSuite) TestGetMachineTargetAgentVersion(c *gc.C) {
	machineUUID := s.createMachine(c)
	s.setMachineAgentVersion(c, machineUUID, "4.0.0")
	s.setModelTargetAgentVersion(c, "4.0.1")

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

// TestGetMachineTargetAgentVersionTargetVersionNotSet test that if no target
// agent version has been set we get back an error satisfying
// [modelagenterrors.AgentVersionNotFound].
func (s *modelStateSuite) TestGetMachineTargetAgentVersionTargetVersionNotSet(c *gc.C) {
	machineUUID := s.createMachine(c)
	s.setMachineAgentVersion(c, machineUUID, "4.0.0")

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetMachineTargetAgentVersion(
		context.Background(),
		machineUUID,
	)
	c.Check(err, jc.ErrorIs, modelagenterrors.AgentVersionNotFound)
}

func (s *modelStateSuite) TestGetMachineTargetAgentVersionCantParseVersion(c *gc.C) {
	machineUUID := s.createMachine(c)
	s.setMachineAgentVersion(c, machineUUID, "4.0.0")
	s.setModelTargetAgentVersion(c, "invalid-version")

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetMachineTargetAgentVersion(context.Background(), machineUUID)
	c.Check(err, gc.ErrorMatches, `parsing machine .* agent version: invalid version "invalid-version".*`)
}

// TestGetMachineTargetAgentVersionNotFound is asserting that if the machine exists
// and the model's target agent version has been set but we don't have reported
// agent version for the machine we back an error satisfying
// [modelagenterrors.AgentVersionNotFound].
func (s *modelStateSuite) TestGetMachineTargetAgentVersionNotFound(c *gc.C) {
	machineUUID := s.createMachine(c)
	s.setModelTargetAgentVersion(c, "4.0.1")

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetMachineTargetAgentVersion(context.Background(), machineUUID)
	c.Check(err, jc.ErrorIs, modelagenterrors.AgentVersionNotFound)
}

// TestGetMachineTargetAgentVersionMachineNotFound is testing that if we try and get
// the target agent version for a machine that does not exist we get back a
// [machineerrors.MachineNotFound] error.
func (s *modelStateSuite) TestGetMachineTargetAgentVersionMachineNotFound(c *gc.C) {
	machineUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	_, err = st.GetMachineTargetAgentVersion(context.Background(), machineUUID.String())
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetMachineRunningAgentBinaryVersion is testing that if we try and get
// the running agent version for a machine that does not exist we get back a
// [machineerrors.MachineNotFound] error.
func (s *modelStateSuite) TestGetMachineRunningAgentBinaryVersionMachineNotFound(c *gc.C) {
	machineUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	_, err = st.GetMachineRunningAgentBinaryVersion(context.Background(), machineUUID.String())
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetMachineRunningAgentBinaryVersionNotFound is testing that if machine
// has not set it's running agent binary version and we ask for it we get back
// an error satisfying [modelagenterrors.AgentVersionNotFound].
func (s *modelStateSuite) TestGetMachineRunningAgentBinaryVersionNotFound(c *gc.C) {
	machineUUID := s.createMachine(c)

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetMachineRunningAgentBinaryVersion(context.Background(), machineUUID)
	c.Check(err, jc.ErrorIs, modelagenterrors.AgentVersionNotFound)
}

// TestGetMachineRunningAgentBinaryVersion asserts the happy path.
func (s *modelStateSuite) TestGetMachineRunningAgentBinaryVersion(c *gc.C) {
	machineUUID := s.createMachine(c)
	s.setMachineAgentVersion(c, machineUUID, "4.1.1")

	st := NewState(s.TxnRunnerFactory())
	ver, err := st.GetMachineRunningAgentBinaryVersion(context.Background(), machineUUID)
	c.Check(err, jc.ErrorIsNil)
	c.Check(ver, jc.DeepEquals, coreagentbinary.Version{
		Number: semversion.MustParse("4.1.1"),
		Arch:   corearch.AMD64,
	})
}

// TestGetUnitTargetAgentVersion is testing the happy path of getting a units
// target agent binary version.
func (s *modelStateSuite) TestGetUnitTargetAgentBinaryVersion(c *gc.C) {
	unitUUID := s.createTestingUnit(c)
	s.setUnitAgentVersion(c, unitUUID, "4.0.0")
	s.setModelTargetAgentVersion(c, "4.0.1")

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

// TestGetUnitAgentVersionCantParseVersion test that when the target agent
// version can't be parsed by state we get back an error.
func (s *modelStateSuite) TestGetUnitTargetAgentVersionCantParseVersion(c *gc.C) {
	unitUUID := s.createTestingUnit(c)
	s.setUnitAgentVersion(c, unitUUID, "4.0.0")
	s.setModelTargetAgentVersion(c, "invalid-version")

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetUnitTargetAgentVersion(context.Background(), unitUUID)
	c.Check(err, gc.ErrorMatches, `parsing unit .* target agent version "invalid-version": invalid version "invalid-version".*`)
}

// TestGetUnitAgentVersionNotFound asserts that if the unit has not record a
// agent binary version yet we get back a
// [modelagenterrors.AgentVersionNotFound] error.
func (s *modelStateSuite) TestGetUnitTargetAgentVersionNotFound(c *gc.C) {
	unitUUID := s.createTestingUnit(c)

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetUnitTargetAgentVersion(context.Background(), unitUUID)
	c.Check(err, jc.ErrorIs, modelagenterrors.AgentVersionNotFound)
}

// TestGetUnitTargetAgentVersionModelVersionNotFound is testing that if no
// target agent version has been set for the model we get back a
// [modelagenterrors.AgentVersionNotFound] error.
func (s *modelStateSuite) TestGetUnitTargetAgentVersionModelVersionNotFound(c *gc.C) {
	unitUUID := s.createTestingUnit(c)
	s.setUnitAgentVersion(c, unitUUID, "4.1.1")

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetUnitTargetAgentVersion(context.Background(), unitUUID)
	c.Check(err, jc.ErrorIs, modelagenterrors.AgentVersionNotFound)
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

	ver, err := st.GetUnitRunningAgentBinaryVersion(context.Background(), unitUUID)
	c.Check(ver.Arch, gc.Equals, corearch.ARM64)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ver.Number.String(), gc.Equals, jujuversion.Current.String())
	c.Check(ver.Arch, gc.Equals, corearch.ARM64)
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

	ver, err := st.GetUnitRunningAgentBinaryVersion(context.Background(), unitUUID)
	c.Check(ver.Arch, gc.Equals, corearch.ARM64)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ver.Number.String(), gc.Equals, jujuversion.Current.String())
	c.Check(ver.Arch, gc.Equals, corearch.ARM64)

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

	ver, err = st.GetUnitRunningAgentBinaryVersion(context.Background(), unitUUID)
	c.Check(err, jc.ErrorIsNil)
	c.Check(ver.Number.String(), gc.Equals, newVersion.String())
	c.Check(ver.Arch, gc.Equals, corearch.ARM64)
}

// TestGetUnitRunningAgentBinaryVersionUnitNotFound tests that if we ask for the
// running unit agent binary version for a unit that doesn't exist we get
// [applicationerrors.UnitNotFound] error.
func (s *modelStateSuite) TestGetUnitRunningAgentBinaryVersionUnitNotFound(c *gc.C) {
	unitUUID := unittesting.GenUnitUUID(c)
	_, err := NewState(s.TxnRunnerFactory()).GetUnitRunningAgentBinaryVersion(
		context.Background(), unitUUID,
	)
	c.Check(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestGetUnitRunningAgentBinaryVersionNotFound tests that if no reported
// running agent binary version has been set for a unit we get an error that
// satisfies [modelagenterrors.AgentVersionNotFound].
func (s *modelStateSuite) TestGetUnitRunningAgentBinaryVersionNotFound(c *gc.C) {
	unitUUID := s.createTestingUnit(c)
	_, err := NewState(s.TxnRunnerFactory()).GetUnitRunningAgentBinaryVersion(
		context.Background(),
		unitUUID,
	)
	c.Check(err, jc.ErrorIs, modelagenterrors.AgentVersionNotFound)
}

// TestMachinesNotAtTargetAgentVersionEmpty tests that if the model has no
// machines we get back an empty list for
// [State.GetMachinesNotAtTargetAgentVersion].
func (s *modelStateSuite) TestMachinesNotAtTargetAgentVersionEmpty(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	list, err := st.GetMachinesNotAtTargetAgentVersion(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(len(list), gc.Equals, 0)
}

// TestMachinesNotAtTargetAgentVersionUnreported is testing that when we have
// a machine that has no reported agent version in the database it appears in
// the list returned.
func (s *modelStateSuite) TestMachinesNotAtTargetAgentVersionUnreported(c *gc.C) {
	notRegName := machine.Name("1")
	regName := machine.Name("2")
	s.createMachineWithName(c, notRegName)
	regUUID := s.createMachineWithName(c, regName)
	s.setModelTargetAgentVersion(c, "4.0.1")
	s.setMachineAgentVersion(c, regUUID, "4.0.1")
	st := NewState(s.TxnRunnerFactory())
	list, err := st.GetMachinesNotAtTargetAgentVersion(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(list, jc.DeepEquals, []machine.Name{
		notRegName,
	})
}

// TestMachinesNotAtTargetAgentVersionFallingBehind is testing that when a
// machine's agent version is behind that of the target for the model it is
// reported in the list.
func (s *modelStateSuite) TestMachinesNotAtTargetAgentVersionFallingBehind(c *gc.C) {
	m1Name := machine.Name("1")
	m2Name := machine.Name("2")
	m1UUID := s.createMachineWithName(c, m1Name)
	m2UUID := s.createMachineWithName(c, m2Name)
	s.setModelTargetAgentVersion(c, "4.1.0")
	s.setMachineAgentVersion(c, m1UUID, "4.0.1")
	s.setMachineAgentVersion(c, m2UUID, "4.0.1")
	st := NewState(s.TxnRunnerFactory())
	list, err := st.GetMachinesNotAtTargetAgentVersion(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(list, jc.DeepEquals, []machine.Name{
		m1Name, m2Name,
	})
}

// TestMachinesNotAtTargetAgentVersionAllUptoDate is testing that all the
// machines are at the same version as that of the target model agent version
// the list returned is empty.
func (s *modelStateSuite) TestMachinesNotAtTargetAgentVersionAllUptoDate(c *gc.C) {
	m1Name := machine.Name("1")
	m2Name := machine.Name("2")
	m1UUID := s.createMachineWithName(c, m1Name)
	m2UUID := s.createMachineWithName(c, m2Name)
	s.setModelTargetAgentVersion(c, "4.1.0")
	s.setMachineAgentVersion(c, m1UUID, "4.1.0")
	s.setMachineAgentVersion(c, m2UUID, "4.1.0")
	st := NewState(s.TxnRunnerFactory())
	list, err := st.GetMachinesNotAtTargetAgentVersion(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(len(list), gc.Equals, 0)
}

// TestUnitsNotAtTargetAgentVersionEmpty tests that if the model has no
// units we get back an empty list for
// [State.GetUnitsNotAtTargetAgentVersion].
func (s *modelStateSuite) TestUnitsNotAtTargetAgentVersionEmpty(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	list, err := st.GetUnitsNotAtTargetAgentVersion(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(len(list), gc.Equals, 0)
}

// TestUnitsNotAtTargetAgentVersionUnreported is testing that when we have
// a unit that has no reported agent version in the database it appears in
// the list returned.
func (s *modelStateSuite) TestUnitsNotAtTargetAgentVersionUnreported(c *gc.C) {
	appID := s.createTestingApplicationWithName(c, "foo")
	s.createTestingUnitWithName(c, "foo", appID, coreunit.Name("foo/1"))
	unit2UUID := s.createTestingUnitWithName(c, "foo", appID, coreunit.Name("foo/2"))
	s.setModelTargetAgentVersion(c, "4.0.1")
	s.setUnitAgentVersion(c, unit2UUID, "4.0.1")
	st := NewState(s.TxnRunnerFactory())
	list, err := st.GetUnitsNotAtTargetAgentVersion(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(list, jc.DeepEquals, []coreunit.Name{
		coreunit.Name("foo/1"),
	})
}

// TestUnitsNotAtTargetAgentVersionFallingBehind is testing that when a
// unit's agent version is behind that of the target for the model it is
// reported in the list.
func (s *modelStateSuite) TestUnitsNotAtTargetAgentVersionFallingBehind(c *gc.C) {
	appID := s.createTestingApplicationWithName(c, "foo")
	unit1UUID := s.createTestingUnitWithName(c, "foo", appID, coreunit.Name("foo/1"))
	unit2UUID := s.createTestingUnitWithName(c, "foo", appID, coreunit.Name("foo/2"))
	s.setModelTargetAgentVersion(c, "4.1.0")
	s.setUnitAgentVersion(c, unit1UUID, "4.0.1")
	s.setUnitAgentVersion(c, unit2UUID, "4.0.1")
	st := NewState(s.TxnRunnerFactory())
	list, err := st.GetUnitsNotAtTargetAgentVersion(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(list, jc.DeepEquals, []coreunit.Name{
		coreunit.Name("foo/1"), coreunit.Name("foo/2"),
	})
}

// TestUnitsNotAtTargetAgentVersionAllUptoDate is testing that all the
// units are at the same version as that of the target model agent version
// the list returned is empty.
func (s *modelStateSuite) TestUnitsNotAtTargetAgentVersionAllUptoDate(c *gc.C) {
	appID := s.createTestingApplicationWithName(c, "foo")
	unit1UUID := s.createTestingUnitWithName(c, "foo", appID, coreunit.Name("foo/1"))
	unit2UUID := s.createTestingUnitWithName(c, "foo", appID, coreunit.Name("foo/2"))
	s.setModelTargetAgentVersion(c, "4.1.0")
	s.setUnitAgentVersion(c, unit1UUID, "4.1.0")
	s.setUnitAgentVersion(c, unit2UUID, "4.1.0")
	st := NewState(s.TxnRunnerFactory())
	list, err := st.GetUnitsNotAtTargetAgentVersion(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(len(list), gc.Equals, 0)
}
