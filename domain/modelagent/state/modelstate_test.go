// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	coreapplication "github.com/juju/juju/core/application"
	corearch "github.com/juju/juju/core/arch"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	coremachinetesting "github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/semversion"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/agentbinary"
	agentbinarystate "github.com/juju/juju/domain/agentbinary/state"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	machinestate "github.com/juju/juju/domain/machine/state"
	"github.com/juju/juju/domain/modelagent"
	modelagenterrors "github.com/juju/juju/domain/modelagent/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type modelStateSuite struct {
	schematesting.ModelSuite
}

var _ = tc.Suite(&modelStateSuite{})

func (s *modelStateSuite) createMachine(c *tc.C) string {
	return s.createMachineWithName(c, machine.Name("6666"))
}

func (s *modelStateSuite) createMachineWithName(c *tc.C, name machine.Name) string {
	machineSt := machinestate.NewState(
		s.TxnRunnerFactory(),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	uuid := coremachinetesting.GenUUID(c)
	err := machineSt.CreateMachine(c.Context(), name, uuid.String(), uuid)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	machineUUID, err := st.GetMachineUUIDByName(c.Context(), name)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machineUUID, tc.Equals, uuid.String())

	return uuid.String()
}

// registerAgentBinary is a testing utility function that registers the fact
// that an agent binary exists in the models store for the provided version. The
// metadata for the newly created binary is returned to the caller upon creation.
func (s *modelStateSuite) registerAgentBinary(
	c *tc.C,
	version coreagentbinary.Version,
) coreagentbinary.Metadata {
	runner := s.TxnRunner()

	type objectStoreMeta struct {
		UUID   string `db:"uuid"`
		SHA256 string `db:"sha_256"`
		SHA384 string `db:"sha_384"`
		Size   int    `db:"size"`
	}

	storeUUID := uuid.MustNewUUID().String()
	stmt, err := sqlair.Prepare(`
INSERT INTO object_store_metadata (uuid, sha_256, sha_384, size)
VALUES ($objectStoreMeta.uuid, $objectStoreMeta.sha_256, $objectStoreMeta.sha_384, $objectStoreMeta.size)
`, objectStoreMeta{})
	c.Assert(err, tc.ErrorIsNil)

	hasher256 := sha256.New()
	hasher384 := sha512.New384()
	_, err = io.Copy(io.MultiWriter(hasher256, hasher384), strings.NewReader(storeUUID))
	c.Assert(err, tc.ErrorIsNil)
	sha256Hash := hex.EncodeToString(hasher256.Sum(nil))
	sha384Hash := hex.EncodeToString(hasher384.Sum(nil))

	metaRecord := objectStoreMeta{
		UUID:   storeUUID,
		SHA256: sha256Hash,
		SHA384: sha384Hash,
		Size:   1234,
	}
	err = runner.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, metaRecord).Run()
	})
	c.Assert(err, tc.ErrorIsNil)

	type dbMetadataPath struct {
		// UUID is the uuid for the metadata.
		UUID string `db:"metadata_uuid"`
		// Path is the path to the object.
		Path string `db:"path"`
	}
	path := "/path/" + storeUUID
	pathRecord := dbMetadataPath{
		UUID: storeUUID,
		Path: path,
	}
	pathStmt, err := sqlair.Prepare(`
INSERT INTO object_store_metadata_path (path, metadata_uuid)
VALUES ($dbMetadataPath.*)`, pathRecord)
	c.Assert(err, tc.ErrorIsNil)
	err = runner.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, pathStmt, pathRecord).Run()
	})
	c.Assert(err, tc.ErrorIsNil)

	err = agentbinarystate.NewState(s.TxnRunnerFactory()).RegisterAgentBinary(
		c.Context(),
		agentbinary.RegisterAgentBinaryArg{
			Arch:            version.Arch,
			ObjectStoreUUID: objectstore.UUID(storeUUID),
			Version:         version.Number.String(),
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	return coreagentbinary.Metadata{
		SHA256:  sha256Hash,
		SHA384:  sha384Hash,
		Size:    1234,
		Version: version,
	}
}

// Set the agent version for the given model in the DB.
func (s *modelStateSuite) setModelTargetAgentVersion(c *tc.C, vers string) {
	db, err := domain.NewStateBase(s.TxnRunnerFactory()).DB()
	c.Assert(err, tc.ErrorIsNil)

	q := "INSERT INTO agent_version (*) VALUES ($M.stream_id, $M.target_version)"

	args := sqlair.M{
		"target_version": vers,
		"stream_id":      modelagent.AgentStreamReleased,
	}
	stmt, err := sqlair.Prepare(q, args)
	c.Assert(err, tc.ErrorIsNil)

	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, args).Run()
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelStateSuite) setMachineAgentVersion(c *tc.C, machineUUID, running string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO machine_agent_version (machine_uuid, version, architecture_id) values (?, ?, ?)",
			machineUUID, running, 0)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelStateSuite) setUnitAgentVersion(
	c *tc.C, unitUUID coreunit.UUID, running string,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO unit_agent_version (unit_uuid, version, architecture_id) values (?, ?, ?)",
			unitUUID.String(), running, 0)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelStateSuite) createTestingUnit(
	c *tc.C,
) coreunit.UUID {
	s.createTestingApplicationWithName(c, "foo")
	return s.createTestingUnitForApplication(c, "foo")
}

func (s *modelStateSuite) createTestingUnitForApplication(
	c *tc.C,
	appName string,
) coreunit.UUID {
	appState := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	appID, err := appState.GetApplicationIDByName(c.Context(), appName)
	c.Assert(err, tc.ErrorIsNil)

	unitNames, err := appState.AddIAASUnits(c.Context(), appID, application.AddUnitArg{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitNames, tc.HasLen, 1)
	unitName := unitNames[0]

	unitUUID, err := appState.GetUnitUUIDByName(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)

	return unitUUID
}

func (s *modelStateSuite) createTestingApplicationWithName(
	c *tc.C,
	appName string,
) coreapplication.ID {
	appState := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	platform := deployment.Platform{
		Channel:      "22.04/stable",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "stable",
		Branch: "branch",
	}
	ctx := c.Context()

	appID, err := appState.CreateApplication(ctx, appName, application.AddApplicationArg{
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
	c.Assert(err, tc.ErrorIsNil)
	return appID
}

// TestGetModelAgentVersionSuccess tests that State.GetModelAgentVersion is
// correct in the expected case when the model exists.
func (s *modelStateSuite) TestGetModelAgentVersionSuccess(c *tc.C) {
	expectedVersion, err := semversion.Parse("4.21.54")
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	s.setModelTargetAgentVersion(c, expectedVersion.String())

	obtainedVersion, err := st.GetModelTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(obtainedVersion, tc.DeepEquals, expectedVersion)
}

// TestGetModelAgentVersionModelNotFound tests that State.GetModelAgentVersion
// returns modelerrors.NotFound when the model does not exist in the DB.
func (s *modelStateSuite) TestGetModelAgentVersionModelNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetModelTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIs, modelagenterrors.AgentVersionNotFound)
}

// TestGetModelAgentVersionCantParseVersion tests that State.GetModelAgentVersion
// returns an appropriate error when the agent version in the DB is invalid.
func (s *modelStateSuite) TestGetModelAgentVersionCantParseVersion(c *tc.C) {
	s.setModelTargetAgentVersion(c, "invalid-version")

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetModelTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorMatches, `parsing agent version: invalid version "invalid-version".*`)
}

// TestGetMachineTargetAgentVersion asserts the happy path of getting the target
// agent version for machine that exists.
func (s *modelStateSuite) TestGetMachineTargetAgentVersion(c *tc.C) {
	machineUUID := s.createMachine(c)
	s.setMachineAgentVersion(c, machineUUID, "4.0.0")
	s.setModelTargetAgentVersion(c, "4.0.1")

	st := NewState(s.TxnRunnerFactory())
	vers, err := st.GetMachineTargetAgentVersion(
		c.Context(),
		machineUUID,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(vers, tc.DeepEquals, coreagentbinary.Version{
		Number: semversion.MustParse("4.0.1"),
		Arch:   "amd64",
	})
}

// TestGetMachineTargetAgentVersionTargetVersionNotSet test that if no target
// agent version has been set we get back an error satisfying
// [modelagenterrors.AgentVersionNotFound].
func (s *modelStateSuite) TestGetMachineTargetAgentVersionTargetVersionNotSet(c *tc.C) {
	machineUUID := s.createMachine(c)
	s.setMachineAgentVersion(c, machineUUID, "4.0.0")

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetMachineTargetAgentVersion(
		c.Context(),
		machineUUID,
	)
	c.Check(err, tc.ErrorIs, modelagenterrors.AgentVersionNotFound)
}

func (s *modelStateSuite) TestGetMachineTargetAgentVersionCantParseVersion(c *tc.C) {
	machineUUID := s.createMachine(c)
	s.setMachineAgentVersion(c, machineUUID, "4.0.0")
	s.setModelTargetAgentVersion(c, "invalid-version")

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetMachineTargetAgentVersion(c.Context(), machineUUID)
	c.Check(err, tc.ErrorMatches, `parsing machine .* agent version: invalid version "invalid-version".*`)
}

// TestGetMachineTargetAgentVersionNotFound is asserting that if the machine exists
// and the model's target agent version has been set but we don't have reported
// agent version for the machine we back an error satisfying
// [modelagenterrors.AgentVersionNotFound].
func (s *modelStateSuite) TestGetMachineTargetAgentVersionNotFound(c *tc.C) {
	machineUUID := s.createMachine(c)
	s.setModelTargetAgentVersion(c, "4.0.1")

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetMachineTargetAgentVersion(c.Context(), machineUUID)
	c.Check(err, tc.ErrorIs, modelagenterrors.AgentVersionNotFound)
}

// TestGetMachineTargetAgentVersionMachineNotFound is testing that if we try and get
// the target agent version for a machine that does not exist we get back a
// [machineerrors.MachineNotFound] error.
func (s *modelStateSuite) TestGetMachineTargetAgentVersionMachineNotFound(c *tc.C) {
	machineUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	_, err = st.GetMachineTargetAgentVersion(c.Context(), machineUUID.String())
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetMachineRunningAgentBinaryVersion is testing that if we try and get
// the running agent version for a machine that does not exist we get back a
// [machineerrors.MachineNotFound] error.
func (s *modelStateSuite) TestGetMachineRunningAgentBinaryVersionMachineNotFound(c *tc.C) {
	machineUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	_, err = st.GetMachineRunningAgentBinaryVersion(c.Context(), machineUUID.String())
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetMachineRunningAgentBinaryVersionNotFound is testing that if machine
// has not set it's running agent binary version and we ask for it we get back
// an error satisfying [modelagenterrors.AgentVersionNotFound].
func (s *modelStateSuite) TestGetMachineRunningAgentBinaryVersionNotFound(c *tc.C) {
	machineUUID := s.createMachine(c)

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetMachineRunningAgentBinaryVersion(c.Context(), machineUUID)
	c.Check(err, tc.ErrorIs, modelagenterrors.AgentVersionNotFound)
}

// TestGetMachineRunningAgentBinaryVersion asserts the happy path.
func (s *modelStateSuite) TestGetMachineRunningAgentBinaryVersion(c *tc.C) {
	machineUUID := s.createMachine(c)
	s.setMachineAgentVersion(c, machineUUID, "4.1.1")

	st := NewState(s.TxnRunnerFactory())
	ver, err := st.GetMachineRunningAgentBinaryVersion(c.Context(), machineUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver, tc.DeepEquals, coreagentbinary.Version{
		Number: semversion.MustParse("4.1.1"),
		Arch:   corearch.AMD64,
	})
}

// TestGetUnitTargetAgentVersion is testing the happy path of getting a units
// target agent binary version.
func (s *modelStateSuite) TestGetUnitTargetAgentBinaryVersion(c *tc.C) {
	unitUUID := s.createTestingUnit(c)
	s.setUnitAgentVersion(c, unitUUID, "4.0.0")
	s.setModelTargetAgentVersion(c, "4.0.1")

	st := NewState(s.TxnRunnerFactory())
	vers, err := st.GetUnitTargetAgentVersion(
		c.Context(),
		unitUUID,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(vers, tc.DeepEquals, coreagentbinary.Version{
		Number: semversion.MustParse("4.0.1"),
		Arch:   "amd64",
	})
}

// TestGetUnitAgentVersionCantParseVersion test that when the target agent
// version can't be parsed by state we get back an error.
func (s *modelStateSuite) TestGetUnitTargetAgentVersionCantParseVersion(c *tc.C) {
	unitUUID := s.createTestingUnit(c)
	s.setUnitAgentVersion(c, unitUUID, "4.0.0")
	s.setModelTargetAgentVersion(c, "invalid-version")

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetUnitTargetAgentVersion(c.Context(), unitUUID)
	c.Check(err, tc.ErrorMatches, `parsing unit .* target agent version "invalid-version": invalid version "invalid-version".*`)
}

// TestGetUnitAgentVersionNotFound asserts that if the unit has not record a
// agent binary version yet we get back a
// [modelagenterrors.AgentVersionNotFound] error.
func (s *modelStateSuite) TestGetUnitTargetAgentVersionNotFound(c *tc.C) {
	unitUUID := s.createTestingUnit(c)

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetUnitTargetAgentVersion(c.Context(), unitUUID)
	c.Check(err, tc.ErrorIs, modelagenterrors.AgentVersionNotFound)
}

// TestGetUnitTargetAgentVersionModelVersionNotFound is testing that if no
// target agent version has been set for the model we get back a
// [modelagenterrors.AgentVersionNotFound] error.
func (s *modelStateSuite) TestGetUnitTargetAgentVersionModelVersionNotFound(c *tc.C) {
	unitUUID := s.createTestingUnit(c)
	s.setUnitAgentVersion(c, unitUUID, "4.1.1")

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetUnitTargetAgentVersion(c.Context(), unitUUID)
	c.Check(err, tc.ErrorIs, modelagenterrors.AgentVersionNotFound)
}

// TestSetMachineRunningAgentBinaryVersionSuccess asserts that if we attempt to
// set the running agent binary version for a machine that doesn't exist we get
// back an error that satisfies [machineerrors.MachineNotFound].
func (s *modelStateSuite) TestSetMachineRunningAgentBinaryVersionMachineNotFound(c *tc.C) {
	machineUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	err = st.SetMachineRunningAgentBinaryVersion(
		c.Context(),
		machineUUID.String(),
		coreagentbinary.Version{
			Number: jujuversion.Current,
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *modelStateSuite) TestMachineSetRunningAgentBinaryVersionMachineDead(c *tc.C) {
	machineUUID := coremachinetesting.GenUUID(c)
	machineSt := machinestate.NewState(
		s.TxnRunnerFactory(),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	err := machineSt.CreateMachine(c.Context(), "666", "", machineUUID)
	c.Assert(err, tc.ErrorIsNil)

	err = machineSt.SetMachineLife(c.Context(), "666", life.Dead)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	err = st.SetMachineRunningAgentBinaryVersion(
		c.Context(),
		machineUUID.String(),
		coreagentbinary.Version{
			Number: jujuversion.Current,
			Arch:   corearch.Arch("noexist"),
		},
	)
	c.Check(err, tc.ErrorIs, machineerrors.MachineIsDead)
}

// TestSetMachineRunningAgentBinaryVersionNotSupportedArch tests that if we provide an
// architecture that isn't supported by the database we get back an error
// that satisfies [coreerrors.NotValid].
func (s *modelStateSuite) TestSetMachineRunningAgentBinaryVersionNotSupportedArch(c *tc.C) {
	machineUUID := s.createMachine(c)

	st := NewState(s.TxnRunnerFactory())
	err := st.SetMachineRunningAgentBinaryVersion(
		c.Context(),
		machineUUID,
		coreagentbinary.Version{
			Number: jujuversion.Current,
			Arch:   corearch.Arch("noexist"),
		},
	)
	c.Check(err, tc.ErrorIs, coreerrors.NotSupported)
}

// TestSetMachineRunningAgentBinaryVersion asserts setting the initial agent
// binary version (happy path).
func (s *modelStateSuite) TestSetMachineRunningAgentBinaryVersion(c *tc.C) {
	machineUUID := s.createMachine(c)

	st := NewState(s.TxnRunnerFactory())
	err := st.SetMachineRunningAgentBinaryVersion(
		c.Context(),
		machineUUID,
		coreagentbinary.Version{
			Number: jujuversion.Current,
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, tc.ErrorIsNil)

	var (
		obtainedMachineUUID string
		obtainedVersion     string
		obtainedArch        string
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
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
	c.Check(err, tc.ErrorIsNil)
	c.Check(obtainedMachineUUID, tc.Equals, machineUUID)
	c.Check(obtainedVersion, tc.Equals, jujuversion.Current.String())
	c.Check(obtainedArch, tc.Equals, corearch.ARM64)
}

// TestSetMachineRunningAgentBinaryVersion asserts setting the initial agent
// binary version (happy path) and then updating the value.
func (s *modelStateSuite) TestSetMachineRunningAgentBinaryVersionUpdate(c *tc.C) {
	machineUUID := s.createMachine(c)

	st := NewState(s.TxnRunnerFactory())
	err := st.SetMachineRunningAgentBinaryVersion(
		c.Context(),
		machineUUID,
		coreagentbinary.Version{
			Number: jujuversion.Current,
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, tc.ErrorIsNil)

	var (
		obtainedMachineUUID string
		obtainedVersion     string
		obtainedArch        string
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
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
	c.Check(err, tc.ErrorIsNil)
	c.Check(obtainedMachineUUID, tc.Equals, machineUUID)
	c.Check(obtainedVersion, tc.Equals, jujuversion.Current.String())

	// Update
	err = st.SetMachineRunningAgentBinaryVersion(
		c.Context(),
		machineUUID,
		coreagentbinary.Version{
			Number: jujuversion.Current,
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(obtainedArch, tc.Equals, corearch.ARM64)
}

// TestSetUnitRunningAgentBinaryVersionUnitNotFound asserts that if we attempt to set the
// running agent binary version for a unit that doesn't exist we get back
// an error that satisfies [applicationerrors.UnitNotFound].
func (s *modelStateSuite) TestSetUnitRunningAgentBinaryVersionUnitNotFound(c *tc.C) {
	unitUUID := unittesting.GenUnitUUID(c)

	st := NewState(s.TxnRunnerFactory())
	err := st.SetUnitRunningAgentBinaryVersion(
		c.Context(),
		unitUUID,
		coreagentbinary.Version{
			Number: jujuversion.Current,
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestSetUnitRunningAgentBinaryVersionNotSupportedArch tests that if we provide
// an architecture that isn't supported by the database we get back an error
// that satisfies [coreerrors.NotSupported].
func (s *modelStateSuite) TestSetUnitRunningAgentBinaryVersionNotSupportedArch(c *tc.C) {
	unitUUID := s.createTestingUnit(c)
	st := NewState(s.TxnRunnerFactory())
	err := st.SetUnitRunningAgentBinaryVersion(
		c.Context(),
		unitUUID,
		coreagentbinary.Version{
			Number: jujuversion.Current,
			Arch:   corearch.Arch("noexist"),
		},
	)
	c.Check(err, tc.ErrorIs, coreerrors.NotSupported)
}

// TestSetUnitRunningAgentBinaryVersion asserts setting the initial agent binary
// version (happy path).
func (s *modelStateSuite) TestSetUnitRunningAgentBinaryVersion(c *tc.C) {
	unitUUID := s.createTestingUnit(c)
	st := NewState(s.TxnRunnerFactory())
	err := st.SetUnitRunningAgentBinaryVersion(
		c.Context(),
		unitUUID,
		coreagentbinary.Version{
			Number: jujuversion.Current,
			Arch:   corearch.ARM64,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	ver, err := st.GetUnitRunningAgentBinaryVersion(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ver.Arch, tc.Equals, corearch.ARM64)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ver.Number.String(), tc.Equals, jujuversion.Current.String())
	c.Check(ver.Arch, tc.Equals, corearch.ARM64)
}

// TestSetRunningAgentBinaryVersionUpdate asserts setting the initial agent binary
// version (happy path) and then updating the value.
func (s *modelStateSuite) TestSetUnitRunningAgentBinaryVersionUpdate(c *tc.C) {
	unitUUID := s.createTestingUnit(c)
	st := NewState(s.TxnRunnerFactory())
	err := st.SetUnitRunningAgentBinaryVersion(
		c.Context(),
		unitUUID,
		coreagentbinary.Version{
			Number: jujuversion.Current,
			Arch:   corearch.ARM64,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	ver, err := st.GetUnitRunningAgentBinaryVersion(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ver.Arch, tc.Equals, corearch.ARM64)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ver.Number.String(), tc.Equals, jujuversion.Current.String())
	c.Check(ver.Arch, tc.Equals, corearch.ARM64)

	// Update
	newVersion := jujuversion.Current
	newVersion.Patch++
	err = st.SetUnitRunningAgentBinaryVersion(
		c.Context(),
		unitUUID,
		coreagentbinary.Version{
			Number: newVersion,
			Arch:   corearch.ARM64,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	ver, err = st.GetUnitRunningAgentBinaryVersion(c.Context(), unitUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver.Number.String(), tc.Equals, newVersion.String())
	c.Check(ver.Arch, tc.Equals, corearch.ARM64)
}

// TestGetUnitRunningAgentBinaryVersionUnitNotFound tests that if we ask for the
// running unit agent binary version for a unit that doesn't exist we get
// [applicationerrors.UnitNotFound] error.
func (s *modelStateSuite) TestGetUnitRunningAgentBinaryVersionUnitNotFound(c *tc.C) {
	unitUUID := unittesting.GenUnitUUID(c)
	_, err := NewState(s.TxnRunnerFactory()).GetUnitRunningAgentBinaryVersion(
		c.Context(), unitUUID,
	)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestGetUnitRunningAgentBinaryVersionNotFound tests that if no reported
// running agent binary version has been set for a unit we get an error that
// satisfies [modelagenterrors.AgentVersionNotFound].
func (s *modelStateSuite) TestGetUnitRunningAgentBinaryVersionNotFound(c *tc.C) {
	unitUUID := s.createTestingUnit(c)
	_, err := NewState(s.TxnRunnerFactory()).GetUnitRunningAgentBinaryVersion(
		c.Context(),
		unitUUID,
	)
	c.Check(err, tc.ErrorIs, modelagenterrors.AgentVersionNotFound)
}

// TestMachinesNotAtTargetAgentVersionEmpty tests that if the model has no
// machines we get back an empty list for
// [State.GetMachinesNotAtTargetAgentVersion].
func (s *modelStateSuite) TestMachinesNotAtTargetAgentVersionEmpty(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	list, err := st.GetMachinesNotAtTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(len(list), tc.Equals, 0)
}

// TestMachinesNotAtTargetAgentVersionUnreported is testing that when we have
// a machine that has no reported agent version in the database it appears in
// the list returned.
func (s *modelStateSuite) TestMachinesNotAtTargetAgentVersionUnreported(c *tc.C) {
	notRegName := machine.Name("1")
	regName := machine.Name("2")
	s.createMachineWithName(c, notRegName)
	regUUID := s.createMachineWithName(c, regName)
	s.setModelTargetAgentVersion(c, "4.0.1")
	s.setMachineAgentVersion(c, regUUID, "4.0.1")
	st := NewState(s.TxnRunnerFactory())
	list, err := st.GetMachinesNotAtTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(list, tc.DeepEquals, []machine.Name{
		notRegName,
	})
}

// TestMachinesNotAtTargetAgentVersionFallingBehind is testing that when a
// machine's agent version is behind that of the target for the model it is
// reported in the list.
func (s *modelStateSuite) TestMachinesNotAtTargetAgentVersionFallingBehind(c *tc.C) {
	m1Name := machine.Name("1")
	m2Name := machine.Name("2")
	m1UUID := s.createMachineWithName(c, m1Name)
	m2UUID := s.createMachineWithName(c, m2Name)
	s.setModelTargetAgentVersion(c, "4.1.0")
	s.setMachineAgentVersion(c, m1UUID, "4.0.1")
	s.setMachineAgentVersion(c, m2UUID, "4.0.1")
	st := NewState(s.TxnRunnerFactory())
	list, err := st.GetMachinesNotAtTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(list, tc.DeepEquals, []machine.Name{
		m1Name, m2Name,
	})
}

// TestMachinesNotAtTargetAgentVersionAllUptoDate is testing that all the
// machines are at the same version as that of the target model agent version
// the list returned is empty.
func (s *modelStateSuite) TestMachinesNotAtTargetAgentVersionAllUptoDate(c *tc.C) {
	m1Name := machine.Name("1")
	m2Name := machine.Name("2")
	m1UUID := s.createMachineWithName(c, m1Name)
	m2UUID := s.createMachineWithName(c, m2Name)
	s.setModelTargetAgentVersion(c, "4.1.0")
	s.setMachineAgentVersion(c, m1UUID, "4.1.0")
	s.setMachineAgentVersion(c, m2UUID, "4.1.0")
	st := NewState(s.TxnRunnerFactory())
	list, err := st.GetMachinesNotAtTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(len(list), tc.Equals, 0)
}

// TestUnitsNotAtTargetAgentVersionEmpty tests that if the model has no
// units we get back an empty list for
// [State.GetUnitsNotAtTargetAgentVersion].
func (s *modelStateSuite) TestUnitsNotAtTargetAgentVersionEmpty(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	list, err := st.GetUnitsNotAtTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(len(list), tc.Equals, 0)
}

// TestUnitsNotAtTargetAgentVersionUnreported is testing that when we have
// a unit that has no reported agent version in the database it appears in
// the list returned.
func (s *modelStateSuite) TestUnitsNotAtTargetAgentVersionUnreported(c *tc.C) {
	s.createTestingApplicationWithName(c, "foo")
	s.createTestingUnitForApplication(c, "foo")
	unit2UUID := s.createTestingUnitForApplication(c, "foo")
	s.setModelTargetAgentVersion(c, "4.0.1")
	s.setUnitAgentVersion(c, unit2UUID, "4.0.1")
	st := NewState(s.TxnRunnerFactory())
	list, err := st.GetUnitsNotAtTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(list, tc.DeepEquals, []coreunit.Name{
		coreunit.Name("foo/0"),
	})
}

// TestUnitsNotAtTargetAgentVersionFallingBehind is testing that when a
// unit's agent version is behind that of the target for the model it is
// reported in the list.
func (s *modelStateSuite) TestUnitsNotAtTargetAgentVersionFallingBehind(c *tc.C) {
	s.createTestingApplicationWithName(c, "foo")
	unit1UUID := s.createTestingUnitForApplication(c, "foo")
	unit2UUID := s.createTestingUnitForApplication(c, "foo")
	s.setModelTargetAgentVersion(c, "4.1.0")
	s.setUnitAgentVersion(c, unit1UUID, "4.0.1")
	s.setUnitAgentVersion(c, unit2UUID, "4.0.1")
	st := NewState(s.TxnRunnerFactory())
	list, err := st.GetUnitsNotAtTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(list, tc.DeepEquals, []coreunit.Name{
		coreunit.Name("foo/0"), coreunit.Name("foo/1"),
	})
}

// TestUnitsNotAtTargetAgentVersionAllUptoDate is testing that all the
// units are at the same version as that of the target model agent version
// the list returned is empty.
func (s *modelStateSuite) TestUnitsNotAtTargetAgentVersionAllUptoDate(c *tc.C) {
	s.createTestingApplicationWithName(c, "foo")
	unit1UUID := s.createTestingUnitForApplication(c, "foo")
	unit2UUID := s.createTestingUnitForApplication(c, "foo")
	s.setModelTargetAgentVersion(c, "4.1.0")
	s.setUnitAgentVersion(c, unit1UUID, "4.1.0")
	s.setUnitAgentVersion(c, unit2UUID, "4.1.0")
	st := NewState(s.TxnRunnerFactory())
	list, err := st.GetUnitsNotAtTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(len(list), tc.Equals, 0)
}

// TestGetMachinesAgentBinaryMetadataNoMachines is testing that if the model
// has no machines we get back an empty list of machine agent binary metadata.
func (s *modelStateSuite) TestGetMachinesAgentBinaryMetadataNoMachines(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	data, err := st.GetMachinesAgentBinaryMetadata(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(len(data), tc.Equals, 0)
}

// TestGetMachinesAgentBinaryMetadata tests the happy path of
// [State.GetMachinesAgentBinaryMetadata]. We assert that with multiple machines
// on different agent binaries the each machine is correctly associated.
func (s *modelStateSuite) TestGetMachinesAgentBinaryMetadata(c *tc.C) {
	versionAMD64 := coreagentbinary.Version{
		Number: semversion.MustParse("4.1.0"),
		Arch:   corearch.AMD64,
	}
	versionARM64 := coreagentbinary.Version{
		Number: semversion.MustParse("4.1.0"),
		Arch:   corearch.ARM64,
	}

	metaAMD64 := s.registerAgentBinary(c, versionAMD64)
	metaARM64 := s.registerAgentBinary(c, versionARM64)

	st := NewState(s.TxnRunnerFactory())
	expected := map[machine.Name]coreagentbinary.Metadata{}

	for i := range 5 {
		machineName := machine.Name(fmt.Sprintf("amd64-%d", i))
		machineUUID := s.createMachineWithName(c, machineName)
		err := st.SetMachineRunningAgentBinaryVersion(
			c.Context(), machineUUID, versionAMD64,
		)
		c.Assert(err, tc.ErrorIsNil)
		expected[machineName] = metaAMD64
	}
	for i := range 5 {
		machineName := machine.Name(fmt.Sprintf("arm64-%d", i))
		machineUUID := s.createMachineWithName(c, machineName)
		err := st.SetMachineRunningAgentBinaryVersion(
			c.Context(), machineUUID, versionARM64,
		)
		c.Assert(err, tc.ErrorIsNil)
		expected[machineName] = metaARM64
	}

	data, err := st.GetMachinesAgentBinaryMetadata(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(data, tc.DeepEquals, expected)
}

// TestGetMachinesAgentBinaryMetadataMachineNotSet is testing that given a set
// of machines within the model if at least one of these machines does not have
// an agent binary version set we get back an error that satisfies
// [modelagenterrors.AgentVersionNotSet].
//
// We would expect to see this situation arise when a machine has been
// provisioned by Juju but the machine agent running on the machine has not yet
// come to life and reported their agent version back up to the controller.
func (s *modelStateSuite) TestGetMachinesAgentBinaryMetadataMachineNotSet(c *tc.C) {
	versionAMD64 := coreagentbinary.Version{
		Number: semversion.MustParse("4.1.0"),
		Arch:   corearch.AMD64,
	}
	s.registerAgentBinary(c, versionAMD64)

	st := NewState(s.TxnRunnerFactory())

	for i := range 5 {
		machineName := machine.Name(fmt.Sprintf("amd64-%d", i))
		machineUUID := s.createMachineWithName(c, machineName)
		err := st.SetMachineRunningAgentBinaryVersion(
			c.Context(), machineUUID, versionAMD64,
		)
		c.Assert(err, tc.ErrorIsNil)
	}

	// This is our rogue machine with no agent version set.
	machineName := machine.Name("amd64-6")
	s.createMachineWithName(c, machineName)

	data, err := st.GetMachinesAgentBinaryMetadata(c.Context())
	c.Check(err, tc.ErrorIs, modelagenterrors.AgentVersionNotSet)
	c.Check(len(data), tc.Equals, 0)
}

// TestGetMachinesAgentBinaryMetadataMissingAgentBinary is testing that if every
// machine in the model correctly has their agent version set but the agent
// binary store is missing records for at least one of the agent binaries on
// the machine we get back an error that satisfies
// [modelagenterrors.MissingAgentBinaries]
func (s *modelStateSuite) TestGetMachinesAgentBinaryMetadataMissingAgentBinary(c *tc.C) {
	versionAMD64 := coreagentbinary.Version{
		Number: semversion.MustParse("4.1.0"),
		Arch:   corearch.AMD64,
	}
	s.registerAgentBinary(c, versionAMD64)

	st := NewState(s.TxnRunnerFactory())

	for i := range 5 {
		machineName := machine.Name(fmt.Sprintf("amd64-%d", i))
		machineUUID := s.createMachineWithName(c, machineName)
		err := st.SetMachineRunningAgentBinaryVersion(
			c.Context(), machineUUID, versionAMD64,
		)
		c.Assert(err, tc.ErrorIsNil)
	}

	// This is the machine that is running an agent version for which there
	// exists no agent binaries in the store.
	machineName := machine.Name("arm64-6")
	machineUUID := s.createMachineWithName(c, machineName)
	err := st.SetMachineRunningAgentBinaryVersion(
		c.Context(),
		machineUUID,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.1.0"),
			Arch:   corearch.ARM64,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	data, err := st.GetMachinesAgentBinaryMetadata(c.Context())
	c.Check(err, tc.ErrorIs, modelagenterrors.MissingAgentBinaries)
	c.Check(len(data), tc.Equals, 0)
}

// TestGetUnitAgentBinaryMetadata tests the happy path of
// [State.GetUnitsAgentBinaryMetadata]. We assert that with multiple units on
// different agent binaries the each unit is correctly associated.
func (s *modelStateSuite) TestGetUnitAgentBinaryMetadata(c *tc.C) {
	versionAMD64 := coreagentbinary.Version{
		Number: semversion.MustParse("4.1.0"),
		Arch:   corearch.AMD64,
	}
	versionARM64 := coreagentbinary.Version{
		Number: semversion.MustParse("4.1.0"),
		Arch:   corearch.ARM64,
	}

	metaAMD64 := s.registerAgentBinary(c, versionAMD64)
	metaARM64 := s.registerAgentBinary(c, versionARM64)

	s.createTestingApplicationWithName(c, "foo")

	st := NewState(s.TxnRunnerFactory())
	expected := map[coreunit.Name]coreagentbinary.Metadata{}

	for i := range 5 {
		unitUUID := s.createTestingUnitForApplication(c, "foo")
		err := st.SetUnitRunningAgentBinaryVersion(c.Context(), unitUUID, versionAMD64)
		c.Assert(err, tc.ErrorIsNil)
		unitName, err := coreunit.NewNameFromParts("foo", i)
		c.Assert(err, tc.ErrorIsNil)
		expected[unitName] = metaAMD64
	}

	s.createTestingApplicationWithName(c, "foo1")
	for i := range 5 {
		unitUUID := s.createTestingUnitForApplication(c, "foo1")
		err := st.SetUnitRunningAgentBinaryVersion(c.Context(), unitUUID, versionARM64)
		c.Assert(err, tc.ErrorIsNil)
		unitName, err := coreunit.NewNameFromParts("foo1", i)
		c.Assert(err, tc.ErrorIsNil)
		expected[unitName] = metaARM64
	}

	data, err := st.GetUnitsAgentBinaryMetadata(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(data, tc.DeepEquals, expected)
}

// TestGetUnitsAgentBinaryMetadataUnitNotSet is testing that given a set
// of units within the model if at least one of these units does not have
// an agent binary version set we get back an error that satisfies
// [modelagenterrors.AgentVersionNotSet].
//
// We would expect to see this situation arise when a unit has been
// provisioned by Juju but the agent running on the unit has not yet
// come to life and reported their agent version back up to the controller.
func (s *modelStateSuite) TestGetUnitsAgentBinaryMetadataUnitNotSet(c *tc.C) {
	versionAMD64 := coreagentbinary.Version{
		Number: semversion.MustParse("4.1.0"),
		Arch:   corearch.AMD64,
	}
	s.registerAgentBinary(c, versionAMD64)

	s.createTestingApplicationWithName(c, "foo")
	st := NewState(s.TxnRunnerFactory())

	for range 5 {
		unitUUID := s.createTestingUnitForApplication(c, "foo")
		err := st.SetUnitRunningAgentBinaryVersion(c.Context(), unitUUID, versionAMD64)
		c.Assert(err, tc.ErrorIsNil)
	}

	// This is our rogue machine with no agent version set.
	s.createTestingUnitForApplication(c, "foo")

	data, err := st.GetUnitsAgentBinaryMetadata(c.Context())
	c.Check(err, tc.ErrorIs, modelagenterrors.AgentVersionNotSet)
	c.Check(len(data), tc.Equals, 0)
}

// TestGetUnitsAgentBinaryMetadataMissingAgentBinary is testing that if every
// unit in the model correctly has their agent version set but the agent
// binary store is missing records for at least one of the agent binaries on
// the unit we get back an error that satisfies
// [modelagenterrors.MissingAgentBinaries]
func (s *modelStateSuite) TestGetUnitAgentBinaryMetadataMissingAgentBinary(c *tc.C) {
	versionAMD64 := coreagentbinary.Version{
		Number: semversion.MustParse("4.1.0"),
		Arch:   corearch.AMD64,
	}
	s.registerAgentBinary(c, versionAMD64)

	s.createTestingApplicationWithName(c, "foo")
	st := NewState(s.TxnRunnerFactory())

	for range 5 {
		unitUUID := s.createTestingUnitForApplication(c, "foo")
		err := st.SetUnitRunningAgentBinaryVersion(c.Context(), unitUUID, versionAMD64)
		c.Assert(err, tc.ErrorIsNil)
	}

	// This is the unit that is running an agent version for which there
	// exists no agent binaries in the store.
	unitUUID := s.createTestingUnitForApplication(c, "foo")
	err := st.SetUnitRunningAgentBinaryVersion(
		c.Context(),
		unitUUID,
		coreagentbinary.Version{
			Number: semversion.MustParse("4.1.0"),
			Arch:   corearch.ARM64,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	data, err := st.GetUnitsAgentBinaryMetadata(c.Context())
	c.Check(err, tc.ErrorIs, modelagenterrors.MissingAgentBinaries)
	c.Check(len(data), tc.Equals, 0)
}

// TestSetAgentVersionStream asserts that setting the agent version stream for
// a model correctly updates the database value and is what gets returned when
// asking what is the value that has been set.
func (s *modelStateSuite) TestSetAgentVersionStream(c *tc.C) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		insertStmt := `
INSERT INTO agent_version (stream_id, target_version) VALUES (1, '4.1.1')
`
		_, err := tx.ExecContext(ctx, insertStmt)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	err = st.SetModelAgentStream(c.Context(), modelagent.AgentStreamTesting)
	c.Check(err, tc.ErrorIsNil)

	agentStream, err := st.GetModelAgentStream(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(agentStream, tc.Equals, modelagent.AgentStreamTesting)

	// One more change for good measure.
	err = st.SetModelAgentStream(c.Context(), modelagent.AgentStreamProposed)
	c.Check(err, tc.ErrorIsNil)

	agentStream, err = st.GetModelAgentStream(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(agentStream, tc.Equals, modelagent.AgentStreamProposed)
}
