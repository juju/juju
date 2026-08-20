// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"fmt"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	usertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	domainagentbinary "github.com/juju/juju/domain/agentbinary"
	"github.com/juju/juju/domain/deployment"
	domainmachine "github.com/juju/juju/domain/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	machinestate "github.com/juju/juju/domain/machine/state"
	"github.com/juju/juju/domain/model"
	statemodel "github.com/juju/juju/domain/model/state/model"
	modelmigrationinternal "github.com/juju/juju/domain/modelmigration/internal"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type migrationSuite struct {
	schematesting.ModelSuite

	controllerUUID uuid.UUID
	modelUUID      coremodel.UUID
}

type caasMigrationSuite struct {
	schematesting.ModelSuite

	controllerUUID uuid.UUID
	modelUUID      coremodel.UUID
}

func TestMigrationSuite(t *testing.T) {
	tc.Run(t, &migrationSuite{})
}

func TestCAASMigrationSuite(t *testing.T) {
	tc.Run(t, &caasMigrationSuite{})
}

func (s *migrationSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.controllerUUID = uuid.MustNewUUID()
	s.modelUUID = tc.Must0(c, coremodel.NewUUID)

	runner := s.TxnRunnerFactory()
	state := statemodel.NewState(runner, loggertesting.WrapCheckLog(c))

	args := model.ModelDetailArgs{
		UUID:               s.modelUUID,
		AgentStream:        domainagentbinary.AgentStreamReleased,
		AgentVersion:       jujuversion.Current,
		LatestAgentVersion: jujuversion.Current,
		ControllerUUID:     s.controllerUUID,
		Name:               "my-awesome-model",
		Qualifier:          "prod",
		Type:               coremodel.IAAS,
		Cloud:              "aws",
		CloudType:          "ec2",
		CloudRegion:        "myregion",
		CredentialOwner:    usertesting.GenNewName(c, "myowner"),
		CredentialName:     "mycredential",
	}
	err := state.Create(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *caasMigrationSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.controllerUUID = uuid.MustNewUUID()
	s.modelUUID = tc.Must0(c, coremodel.NewUUID)

	runner := s.TxnRunnerFactory()
	state := statemodel.NewState(runner, loggertesting.WrapCheckLog(c))

	args := model.ModelDetailArgs{
		UUID:               s.modelUUID,
		AgentStream:        domainagentbinary.AgentStreamReleased,
		AgentVersion:       jujuversion.Current,
		LatestAgentVersion: jujuversion.Current,
		ControllerUUID:     s.controllerUUID,
		Name:               "my-awesome-model",
		Qualifier:          "prod",
		Type:               coremodel.CAAS,
		Cloud:              "k8s",
		CloudType:          "kubernetes",
		CloudRegion:        "myregion",
		CredentialOwner:    usertesting.GenNewName(c, "myowner"),
		CredentialName:     "mycredential",
	}
	err := state.Create(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
}

// TestGetControllerUUID is asserting the happy path of getting the controller
// uuid from the database.
func (s *migrationSuite) TestGetControllerUUID(c *tc.C) {
	controllerId, err := New(s.TxnRunnerFactory(), s.modelUUID).GetControllerUUID(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(controllerId, tc.Equals, s.controllerUUID.String())
}

// TestGetMachineInstanceIDs is asserting the happy path of mapping each
// provisioned machine's cloud instance ID to its machine name.
func (s *migrationSuite) TestGetMachineInstanceIDs(c *tc.C) {
	// Add two different instances.
	db := s.DB()
	machineState := machinestate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	_, machineNames0, err := machineState.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineUUID0, err := machineState.GetMachineUUID(c.Context(), machineNames0[0])
	c.Assert(err, tc.ErrorIsNil)

	// Add a reference AZ.
	_, err = db.ExecContext(c.Context(), fmt.Sprintf("INSERT INTO availability_zone VALUES(%q, 'az-1')", machineUUID0.String()))
	c.Assert(err, tc.ErrorIsNil)
	arch := "arm64"
	err = machineState.SetMachineCloudInstance(
		c.Context(),
		machineUUID0.String(),
		instance.Id("instance-0"),
		"",
		"nonce",
		&instance.HardwareCharacteristics{
			Arch: &arch,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	_, machineNames1, err := machineState.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineUUID1, err := machineState.GetMachineUUID(c.Context(), machineNames1[0])
	c.Assert(err, tc.ErrorIsNil)

	err = machineState.SetMachineCloudInstance(
		c.Context(),
		machineUUID1.String(),
		instance.Id("instance-1"),
		"",
		"nonce",
		&instance.HardwareCharacteristics{
			Arch: &arch,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	mapping, err := New(s.TxnRunnerFactory(), s.modelUUID).GetMachineInstanceIDs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mapping, tc.DeepEquals, map[string]string{
		"instance-0": machineNames0[0].String(),
		"instance-1": machineNames1[0].String(),
	})
}

// TestGetMachineInstanceIDsSkipsContainersAndManual asserts that container and
// manually provisioned machines are left out of the mapping. Neither is created
// by the provider, so requiring the cloud to report an instance for them would
// be a false discrepancy.
func (s *migrationSuite) TestGetMachineInstanceIDsSkipsContainersAndManual(c *tc.C) {
	db := s.DB()
	machineState := machinestate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	arch := "arm64"

	addProvisioned := func(instanceID string) (machine.Name, string) {
		_, names, err := machineState.AddMachine(c.Context(), domainmachine.AddMachineArgs{
			Platform: deployment.Platform{
				Channel: "24.04",
				OSType:  deployment.Ubuntu,
			},
		})
		c.Assert(err, tc.ErrorIsNil)
		mUUID, err := machineState.GetMachineUUID(c.Context(), names[0])
		c.Assert(err, tc.ErrorIsNil)
		err = machineState.SetMachineCloudInstance(
			c.Context(), mUUID.String(), instance.Id(instanceID), "", "nonce",
			&instance.HardwareCharacteristics{Arch: &arch},
		)
		c.Assert(err, tc.ErrorIsNil)
		return names[0], mUUID.String()
	}

	hostName, hostUUID := addProvisioned("instance-host")
	_, containerUUID := addProvisioned("instance-container")
	_, manualUUID := addProvisioned("instance-manual")

	_, err := db.ExecContext(c.Context(),
		"INSERT INTO machine_parent (machine_uuid, parent_uuid) VALUES (?, ?)", containerUUID, hostUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO machine_manual (machine_uuid) VALUES (?)", manualUUID)
	c.Assert(err, tc.ErrorIsNil)

	mapping, err := New(s.TxnRunnerFactory(), s.modelUUID).GetMachineInstanceIDs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mapping, tc.DeepEquals, map[string]string{
		"instance-host": hostName.String(),
	})
}

// TestEmptyInstanceIDs tests that no error is returned when there are no
// instances in the model.
func (s *migrationSuite) TestEmptyInstanceIDs(c *tc.C) {
	mapping, err := New(s.TxnRunnerFactory(), s.modelUUID).GetMachineInstanceIDs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mapping, tc.HasLen, 0)
}

// TestGetModelType asserts the model's deployment type is returned.
func (s *migrationSuite) TestGetModelType(c *tc.C) {
	modelType, err := New(s.TxnRunnerFactory(), s.modelUUID).GetModelType(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelType, tc.Equals, "iaas")
}

// TestGetSecretBackendUUIDsInUse asserts the distinct backend UUIDs across
// external value refs and deleted value refs are returned.
func (s *migrationSuite) TestGetSecretBackendUUIDsInUse(c *tc.C) {
	db := s.DB()
	// secret_deleted_value_ref has no foreign keys, so it is the cheapest way
	// to exercise the union query. Two rows share a backend to prove DISTINCT.
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO secret_deleted_value_ref (revision_uuid, backend_uuid, revision_id) VALUES "+
			"('rev-1', 'backend-a', 'r1'), ('rev-2', 'backend-a', 'r2'), ('rev-3', 'backend-b', 'r3')")
	c.Assert(err, tc.ErrorIsNil)

	backends, err := New(s.TxnRunnerFactory(), s.modelUUID).GetSecretBackendUUIDsInUse(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(backends, tc.SameContents, []string{"backend-a", "backend-b"})
}

// TestGetSecretBackendUUIDsInUseEmpty asserts no error and no rows for a model
// with no external secrets.
func (s *migrationSuite) TestGetSecretBackendUUIDsInUseEmpty(c *tc.C) {
	backends, err := New(s.TxnRunnerFactory(), s.modelUUID).GetSecretBackendUUIDsInUse(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(backends, tc.HasLen, 0)
}

// TestGetExternalSecretRevisionBackendsEmpty exercises the query against a
// model with no external secret revisions.
func (s *migrationSuite) TestGetExternalSecretRevisionBackendsEmpty(c *tc.C) {
	refs, err := New(s.TxnRunnerFactory(), s.modelUUID).GetExternalSecretRevisionBackends(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(refs, tc.HasLen, 0)
}

// TestGetRunningAgentArchitecturesEmpty exercises the query against a model
// with no reported machine or unit agent versions.
func (s *migrationSuite) TestGetRunningAgentArchitecturesEmpty(c *tc.C) {
	archs, err := New(s.TxnRunnerFactory(), s.modelUUID).GetRunningAgentArchitectures(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(archs, tc.HasLen, 0)
}

// TestGetAgentBinaryArchitecturesForVersionEmpty exercises the query against a
// model whose object store holds no agent binaries for the version.
func (s *migrationSuite) TestGetAgentBinaryArchitecturesForVersionEmpty(c *tc.C) {
	archs, err := New(s.TxnRunnerFactory(), s.modelUUID).GetAgentBinaryArchitecturesForVersion(c.Context(), "4.0.1")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(archs, tc.HasLen, 0)
}

func (s *migrationSuite) TestGetMigrationAgentsIAAS(c *tc.C) {
	db := s.DB()

	machineNetNodeUUID := uuid.MustNewUUID().String()
	machineUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO net_node (uuid) VALUES (?)",
		machineNetNodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO machine (uuid, name, net_node_uuid, life_id) VALUES (?, ?, ?, 0)",
		machineUUID, "0", machineNetNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	charmUUID := uuid.MustNewUUID().String()
	appUUID := uuid.MustNewUUID().String()
	unitNetNodeUUID := uuid.MustNewUUID().String()
	unitUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO charm (uuid, reference_name, architecture_id) VALUES (?, ?, 0)",
		charmUUID, "foo")
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, 0, ?, ?)",
		appUUID, "foo", charmUUID, "656b4a82-e28c-53d6-a014-f0dd53417eb6")
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO net_node (uuid) VALUES (?)",
		unitNetNodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO unit (uuid, name, life_id, application_uuid, net_node_uuid, charm_uuid) VALUES (?, ?, 0, ?, ?, ?)",
		unitUUID, "foo/0", appUUID, unitNetNodeUUID, charmUUID)
	c.Assert(err, tc.ErrorIsNil)

	// A synthetic cross-model-relation unit (CMR-source charm, source_id = 2)
	// runs no agent and must NOT be an expected migration agent.
	cmrCharmUUID := uuid.MustNewUUID().String()
	cmrAppUUID := uuid.MustNewUUID().String()
	cmrUnitNetNodeUUID := uuid.MustNewUUID().String()
	cmrUnitUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO charm (uuid, reference_name, source_id) VALUES (?, ?, 2)",
		cmrCharmUUID, "remote-app")
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, 0, ?, ?)",
		cmrAppUUID, "remote-app", cmrCharmUUID, "656b4a82-e28c-53d6-a014-f0dd53417eb6")
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO net_node (uuid) VALUES (?)", cmrUnitNetNodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO unit (uuid, name, life_id, application_uuid, net_node_uuid, charm_uuid) VALUES (?, ?, 0, ?, ?, ?)",
		cmrUnitUUID, "remote-app/0", cmrAppUUID, cmrUnitNetNodeUUID, cmrCharmUUID)
	c.Assert(err, tc.ErrorIsNil)

	agents, err := New(s.TxnRunnerFactory(), s.modelUUID).GetMigrationAgents(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(agents.Machines, tc.SameContents, []string{"0"})
	c.Check(agents.Units, tc.SameContents, []string{"foo/0"})
	c.Check(agents.Applications, tc.HasLen, 0)
}

func (s *caasMigrationSuite) TestGetMigrationAgentsCAAS(c *tc.C) {
	db := s.DB()

	charmUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO charm (uuid, reference_name, architecture_id) VALUES (?, ?, 0)",
		charmUUID, "foo")
	c.Assert(err, tc.ErrorIsNil)

	legacyAppUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, 0, ?, ?)",
		legacyAppUUID, "legacy", charmUUID, "656b4a82-e28c-53d6-a014-f0dd53417eb6")
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application_agent (application_uuid) VALUES (?)",
		legacyAppUUID)
	c.Assert(err, tc.ErrorIsNil)

	// A synthetic CMR application can retain a stale application agent row, but
	// it does not run an application agent and must be excluded.
	cmrCharmUUID := uuid.MustNewUUID().String()
	cmrAppUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO charm (uuid, reference_name, source_id) VALUES (?, ?, 2)",
		cmrCharmUUID, "remote-app")
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, 0, ?, ?)",
		cmrAppUUID, "remote-app", cmrCharmUUID, "656b4a82-e28c-53d6-a014-f0dd53417eb6")
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application_agent (application_uuid) VALUES (?)",
		cmrAppUUID)
	c.Assert(err, tc.ErrorIsNil)

	sidecarAppUUID := uuid.MustNewUUID().String()
	unitNetNodeUUID := uuid.MustNewUUID().String()
	unitUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, 0, ?, ?)",
		sidecarAppUUID, "sidecar", charmUUID, "656b4a82-e28c-53d6-a014-f0dd53417eb6")
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO net_node (uuid) VALUES (?)",
		unitNetNodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO unit (uuid, name, life_id, application_uuid, net_node_uuid, charm_uuid) VALUES (?, ?, 0, ?, ?, ?)",
		unitUUID, "sidecar/0", sidecarAppUUID, unitNetNodeUUID, charmUUID)
	c.Assert(err, tc.ErrorIsNil)

	agents, err := New(s.TxnRunnerFactory(), s.modelUUID).GetMigrationAgents(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(agents.Machines, tc.HasLen, 0)
	c.Check(agents.Units, tc.SameContents, []string{"sidecar/0"})
	c.Check(agents.Applications, tc.SameContents, []string{"legacy"})
}

// TestDeleteModelImportingStatusSuccess tests that clearing an existing
// model_migrating entry succeeds and actually removes the entry from the
// database.
func (s *migrationSuite) TestDeleteModelImportingStatusSuccess(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory(), s.modelUUID)

	// Get the model UUID from the database.
	var modelUUID string
	err := db.QueryRowContext(c.Context(), "SELECT uuid FROM model").Scan(&modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Insert a model_migrating entry.
	migratingUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO model_migrating (uuid, model_uuid) VALUES (?, ?)",
		migratingUUID, modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Verify the entry exists.
	var count int
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migrating WHERE model_uuid = ?",
		modelUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)

	// Clear the importing status.
	err = st.DeleteModelImportingStatus(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// Verify the entry has been deleted.
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migrating WHERE model_uuid = ?",
		modelUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

// TestDeleteModelImportingStatusNoEntry tests that clearing a non-existent
// model_migrating entry succeeds without error (idempotent behavior).
func (s *migrationSuite) TestDeleteModelImportingStatusNoEntry(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory(), s.modelUUID)

	// Verify no entry exists.
	var count int
	err := db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migrating WHERE model_uuid = ?",
		s.modelUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)

	// Clear should succeed even when there's nothing to delete.
	err = st.DeleteModelImportingStatus(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// Verify still no entries.
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migrating WHERE model_uuid = ?",
		s.modelUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

// TestDeleteModelImportingStatusVerifyCorrectEntry tests that clearing
// deletes the correct entry and verifies by UUID.
func (s *migrationSuite) TestDeleteModelImportingStatusVerifyCorrectEntry(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory(), s.modelUUID)

	// Insert a model_migrating entry with a specific UUID.
	migratingUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO model_migrating (uuid, model_uuid) VALUES (?, ?)",
		migratingUUID, s.modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Verify we can query the specific entry by its UUID.
	var retrievedModelUUID string
	err = db.QueryRowContext(c.Context(),
		"SELECT model_uuid FROM model_migrating WHERE uuid = ?",
		migratingUUID).Scan(&retrievedModelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(retrievedModelUUID, tc.Equals, s.modelUUID.String())

	// Clear the importing status.
	err = st.DeleteModelImportingStatus(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// Verify the entry no longer exists.
	var count int
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migrating WHERE uuid = ?",
		migratingUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

// TestDeleteModelImportingStatusIdempotent tests that calling
// DeleteModelImportingStatus multiple times is safe and idempotent.
func (s *migrationSuite) TestDeleteModelImportingStatusIdempotent(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory(), s.modelUUID)

	// Insert a model_migrating entry.
	migratingUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO model_migrating (uuid, model_uuid) VALUES (?, ?)",
		migratingUUID, s.modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Clear the importing status multiple times.
	err = st.DeleteModelImportingStatus(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	err = st.DeleteModelImportingStatus(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// Verify no entries exist.
	var count int
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migrating WHERE model_uuid = ?",
		s.modelUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

// TestSetModelTargetAgentVersion is a happy path test for
// [State.SetModelTargetAgentVersion].
func (s *migrationSuite) TestSetModelTargetAgentVersion(c *tc.C) {
	toVersion := semversion.MustParse("5.2.0").String()

	st := New(s.TxnRunnerFactory(), s.modelUUID)

	err := st.SetModelTargetAgentVersion(c.Context(), jujuversion.Current.String(), toVersion)
	c.Assert(err, tc.ErrorIsNil)

	ver, err := st.GetModelTargetAgentVersion(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ver, tc.Equals, "5.2.0")
}

func (s *migrationSuite) TestSetModelTargetAgentVersionDifferentVersion(c *tc.C) {
	toVersion := semversion.MustParse("5.2.0").String()

	st := New(s.TxnRunnerFactory(), s.modelUUID)

	err := st.SetModelTargetAgentVersion(c.Context(), "6.6.6", toVersion)
	c.Assert(err, tc.ErrorMatches, `.*expected current version "6.6.6"`)
}

// TestGetRelationValidationDataEmpty verifies a model with no relations
// returns no rows and no error.
func (s *migrationSuite) TestGetRelationValidationDataEmpty(c *tc.C) {
	relations, err := New(s.TxnRunnerFactory(), s.modelUUID).GetRelationValidationData(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(relations, tc.HasLen, 0)
}

// TestGetRelationValidationData verifies relation identity and display keys
// are returned for validation.
func (s *migrationSuite) TestGetRelationValidationData(c *tc.C) {
	db := s.DB()
	charmUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO charm (uuid, reference_name, architecture_id) VALUES (?, ?, 0)",
		charmUUID, "wordpress")
	c.Assert(err, tc.ErrorIsNil)

	charmRelationUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, scope_id, interface, optional, capacity) VALUES (?, ?, 'db', 1, 1, 'mysql', false, 1)",
		charmRelationUUID, charmUUID)
	c.Assert(err, tc.ErrorIsNil)

	appUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, 'wordpress', 0, ?, ?)",
		appUUID, charmUUID, "656b4a82-e28c-53d6-a014-f0dd53417eb6")
	c.Assert(err, tc.ErrorIsNil)

	endpointUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application_endpoint (uuid, application_uuid, space_uuid, charm_relation_uuid) VALUES (?, ?, NULL, ?)",
		endpointUUID, appUUID, charmRelationUUID)
	c.Assert(err, tc.ErrorIsNil)

	relationUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO relation (uuid, life_id, relation_id, scope_id) VALUES (?, 0, 7, 1)",
		relationUUID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = db.ExecContext(c.Context(),
		"INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid) VALUES (?, ?, ?)",
		uuid.MustNewUUID().String(), relationUUID, endpointUUID)
	c.Assert(err, tc.ErrorIsNil)

	relations, err := New(s.TxnRunnerFactory(), s.modelUUID).GetRelationValidationData(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(relations, tc.HasLen, 1)
	c.Check(relations[0].UUID, tc.Equals, relationUUID)
	c.Check(relations[0].ID, tc.Equals, 7)
	c.Check(relations[0].Key, tc.Equals, "wordpress:db")
	c.Check(relations[0].Endpoints, tc.DeepEquals, []modelmigrationinternal.RelationValidationEndpoint{
		{ApplicationName: "wordpress", ContainerScoped: true},
	})
}

// TestGetRelationValidationDataEndpointScope verifies each endpoint reports the
// charm relation's scope and whether its application runs a subordinate charm,
// which is what decides which units are expected in the relation's scope.
func (s *migrationSuite) TestGetRelationValidationDataEndpointScope(c *tc.C) {
	db := s.DB()

	// A principal (ubuntu, global endpoint) and a subordinate (nrpe, container
	// endpoint) joined by one relation.
	principalCharmUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO charm (uuid, reference_name, architecture_id) VALUES (?, 'ubuntu', 0)", principalCharmUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO charm_metadata (charm_uuid, name, subordinate) VALUES (?, 'ubuntu', false)", principalCharmUUID)
	c.Assert(err, tc.ErrorIsNil)

	subordinateCharmUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO charm (uuid, reference_name, architecture_id) VALUES (?, 'nrpe', 0)", subordinateCharmUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO charm_metadata (charm_uuid, name, subordinate) VALUES (?, 'nrpe', true)", subordinateCharmUUID)
	c.Assert(err, tc.ErrorIsNil)

	relationUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO relation (uuid, life_id, relation_id, scope_id) VALUES (?, 0, 7, 1)", relationUUID)
	c.Assert(err, tc.ErrorIsNil)

	// scope_id 0 is global, 1 is container.
	addEndpoint := func(charmUUID, appName, endpointName string, scopeID int) {
		charmRelationUUID := uuid.MustNewUUID().String()
		_, err := db.ExecContext(c.Context(),
			"INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, scope_id, interface, optional, capacity) VALUES (?, ?, ?, 1, ?, 'juju-info', false, 1)",
			charmRelationUUID, charmUUID, endpointName, scopeID)
		c.Assert(err, tc.ErrorIsNil)

		appUUID := uuid.MustNewUUID().String()
		_, err = db.ExecContext(c.Context(),
			"INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, 0, ?, ?)",
			appUUID, appName, charmUUID, "656b4a82-e28c-53d6-a014-f0dd53417eb6")
		c.Assert(err, tc.ErrorIsNil)

		endpointUUID := uuid.MustNewUUID().String()
		_, err = db.ExecContext(c.Context(),
			"INSERT INTO application_endpoint (uuid, application_uuid, space_uuid, charm_relation_uuid) VALUES (?, ?, NULL, ?)",
			endpointUUID, appUUID, charmRelationUUID)
		c.Assert(err, tc.ErrorIsNil)

		_, err = db.ExecContext(c.Context(),
			"INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid) VALUES (?, ?, ?)",
			uuid.MustNewUUID().String(), relationUUID, endpointUUID)
		c.Assert(err, tc.ErrorIsNil)
	}
	addEndpoint(subordinateCharmUUID, "nrpe", "general-info", 1)
	addEndpoint(principalCharmUUID, "ubuntu", "juju-info", 0)

	relations, err := New(s.TxnRunnerFactory(), s.modelUUID).GetRelationValidationData(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(relations, tc.HasLen, 1)
	c.Check(relations[0].Key, tc.Equals, "nrpe:general-info ubuntu:juju-info")
	c.Check(relations[0].Endpoints, tc.DeepEquals, []modelmigrationinternal.RelationValidationEndpoint{
		{ApplicationName: "nrpe", ContainerScoped: true, Subordinate: true},
		{ApplicationName: "ubuntu", ContainerScoped: false, Subordinate: false},
	})
}

// TestGetRelationValidationDataExcludesNonAlive verifies dying and dead
// relations are not returned: their units legitimately depart scope, so they
// must not be checked for relation-unit consistency.
func (s *migrationSuite) TestGetRelationValidationDataExcludesNonAlive(c *tc.C) {
	db := s.DB()
	charmUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO charm (uuid, reference_name, architecture_id) VALUES (?, ?, 0)",
		charmUUID, "wordpress")
	c.Assert(err, tc.ErrorIsNil)

	charmRelationUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, scope_id, interface, optional, capacity) VALUES (?, ?, 'db', 1, 1, 'mysql', false, 1)",
		charmRelationUUID, charmUUID)
	c.Assert(err, tc.ErrorIsNil)

	appUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, 'wordpress', 0, ?, ?)",
		appUUID, charmUUID, "656b4a82-e28c-53d6-a014-f0dd53417eb6")
	c.Assert(err, tc.ErrorIsNil)

	endpointUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application_endpoint (uuid, application_uuid, space_uuid, charm_relation_uuid) VALUES (?, ?, NULL, ?)",
		endpointUUID, appUUID, charmRelationUUID)
	c.Assert(err, tc.ErrorIsNil)

	// One dying and one dead relation, both with endpoints: neither is
	// returned.
	for i, lifeID := range []int{1, 2} {
		relationUUID := uuid.MustNewUUID().String()
		_, err = db.ExecContext(c.Context(),
			"INSERT INTO relation (uuid, life_id, relation_id, scope_id) VALUES (?, ?, ?, 1)",
			relationUUID, lifeID, 7+i)
		c.Assert(err, tc.ErrorIsNil)

		_, err = db.ExecContext(c.Context(),
			"INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid) VALUES (?, ?, ?)",
			uuid.MustNewUUID().String(), relationUUID, endpointUUID)
		c.Assert(err, tc.ErrorIsNil)
	}

	relations, err := New(s.TxnRunnerFactory(), s.modelUUID).GetRelationValidationData(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(relations, tc.HasLen, 0)
}

// TestGetRelationValidationDataExcludesCMRRelations verifies relations with
// a cross-model relation (CMR) endpoint are excluded from validation data.
// CMR consumer relations (offering-model side) are imported by the
// crossmodelrelation domain, which does not create relation_unit rows, so
// membership validation cannot apply to them.
func (s *migrationSuite) TestGetRelationValidationDataExcludesCMRRelations(c *tc.C) {
	db := s.DB()

	// A CMR relation with a CMR-sourced remote app on one side.
	// This simulates the offering-model side of a cross-model relation:
	// dummy-source (local) <-> remote-* (CMR).
	cmrCharmUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO charm (uuid, reference_name, source_id) VALUES (?, ?, 2)",
		cmrCharmUUID, "dummy-source-cmr")
	c.Assert(err, tc.ErrorIsNil)

	cmrCR := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, scope_id, interface, optional, capacity) VALUES (?, ?, 'sink', 0, 0, 'token', false, 1)",
		cmrCR, cmrCharmUUID)
	c.Assert(err, tc.ErrorIsNil)

	cmrAppUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, 'remote-d29ba13b', 0, ?, ?)",
		cmrAppUUID, cmrCharmUUID, "656b4a82-e28c-53d6-a014-f0dd53417eb6")
	c.Assert(err, tc.ErrorIsNil)

	cmrRelationUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO relation (uuid, life_id, relation_id, scope_id) VALUES (?, 0, 99, 1)",
		cmrRelationUUID)
	c.Assert(err, tc.ErrorIsNil)

	cmrEndpointUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application_endpoint (uuid, application_uuid, space_uuid, charm_relation_uuid) VALUES (?, ?, NULL, ?)",
		cmrEndpointUUID, cmrAppUUID, cmrCR)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid) VALUES (?, ?, ?)",
		uuid.MustNewUUID().String(), cmrRelationUUID, cmrEndpointUUID)
	c.Assert(err, tc.ErrorIsNil)

	relations, err := New(s.TxnRunnerFactory(), s.modelUUID).GetRelationValidationData(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(relations, tc.HasLen, 0)
}

// TestGetRelationValidationDataKeepsLocalWhileExcludingCMR verifies that
// when a model has both local and CMR relations, only the local relations
// are returned for validation.
func (s *migrationSuite) TestGetRelationValidationDataKeepsLocalWhileExcludingCMR(c *tc.C) {
	db := s.DB()

	// --- Local relation (wordpress:db <-> mysql:db) ---
	localCharmUUID1 := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO charm (uuid, reference_name, architecture_id) VALUES (?, ?, 0)",
		localCharmUUID1, "wordpress")
	c.Assert(err, tc.ErrorIsNil)
	localCR1 := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, scope_id, interface, optional, capacity) VALUES (?, ?, 'db', 0, 0, 'mysql', false, 1)",
		localCR1, localCharmUUID1)
	c.Assert(err, tc.ErrorIsNil)
	localAppUUID1 := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, 'wordpress', 0, ?, ?)",
		localAppUUID1, localCharmUUID1, "656b4a82-e28c-53d6-a014-f0dd53417eb6")
	c.Assert(err, tc.ErrorIsNil)

	localCharmUUID2 := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO charm (uuid, reference_name, architecture_id) VALUES (?, ?, 0)",
		localCharmUUID2, "mysql")
	c.Assert(err, tc.ErrorIsNil)
	localCR2 := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, scope_id, interface, optional, capacity) VALUES (?, ?, 'db', 1, 0, 'mysql', false, 1)",
		localCR2, localCharmUUID2)
	c.Assert(err, tc.ErrorIsNil)
	localAppUUID2 := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, 'mysql', 0, ?, ?)",
		localAppUUID2, localCharmUUID2, "656b4a82-e28c-53d6-a014-f0dd53417eb6")
	c.Assert(err, tc.ErrorIsNil)

	localRelationUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO relation (uuid, life_id, relation_id, scope_id) VALUES (?, 0, 7, 1)",
		localRelationUUID)
	c.Assert(err, tc.ErrorIsNil)

	addLocalEndpoint := func(appUUID, charmRelUUID string) {
		epUUID := uuid.MustNewUUID().String()
		_, err := db.ExecContext(c.Context(),
			"INSERT INTO application_endpoint (uuid, application_uuid, space_uuid, charm_relation_uuid) VALUES (?, ?, NULL, ?)",
			epUUID, appUUID, charmRelUUID)
		c.Assert(err, tc.ErrorIsNil)
		_, err = db.ExecContext(c.Context(),
			"INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid) VALUES (?, ?, ?)",
			uuid.MustNewUUID().String(), localRelationUUID, epUUID)
		c.Assert(err, tc.ErrorIsNil)
	}
	addLocalEndpoint(localAppUUID1, localCR1)
	addLocalEndpoint(localAppUUID2, localCR2)

	// --- CMR relation (dummy-source:sink <-> remote-*:source) ---
	cmrCharmUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO charm (uuid, reference_name, source_id) VALUES (?, ?, 2)",
		cmrCharmUUID, "dummy-source-cmr")
	c.Assert(err, tc.ErrorIsNil)
	cmrCR := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, scope_id, interface, optional, capacity) VALUES (?, ?, 'source', 0, 0, 'token', false, 1)",
		cmrCR, cmrCharmUUID)
	c.Assert(err, tc.ErrorIsNil)
	cmrAppUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, 'remote-d29ba13b', 0, ?, ?)",
		cmrAppUUID, cmrCharmUUID, "656b4a82-e28c-53d6-a014-f0dd53417eb6")
	c.Assert(err, tc.ErrorIsNil)

	cmrLocalCharmUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO charm (uuid, reference_name, architecture_id) VALUES (?, ?, 0)",
		cmrLocalCharmUUID, "dummy-source")
	c.Assert(err, tc.ErrorIsNil)
	cmrCR2 := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, scope_id, interface, optional, capacity) VALUES (?, ?, 'sink', 1, 0, 'token', false, 1)",
		cmrCR2, cmrLocalCharmUUID)
	c.Assert(err, tc.ErrorIsNil)
	cmrLocalAppUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, 'dummy-source-local', 0, ?, ?)",
		cmrLocalAppUUID, cmrLocalCharmUUID, "656b4a82-e28c-53d6-a014-f0dd53417eb6")
	c.Assert(err, tc.ErrorIsNil)

	cmrRelationUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO relation (uuid, life_id, relation_id, scope_id) VALUES (?, 0, 99, 1)",
		cmrRelationUUID)
	c.Assert(err, tc.ErrorIsNil)

	cmrEP1 := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application_endpoint (uuid, application_uuid, space_uuid, charm_relation_uuid) VALUES (?, ?, NULL, ?)",
		cmrEP1, cmrAppUUID, cmrCR)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid) VALUES (?, ?, ?)",
		uuid.MustNewUUID().String(), cmrRelationUUID, cmrEP1)
	c.Assert(err, tc.ErrorIsNil)

	cmrEP2 := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application_endpoint (uuid, application_uuid, space_uuid, charm_relation_uuid) VALUES (?, ?, NULL, ?)",
		cmrEP2, cmrLocalAppUUID, cmrCR2)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid) VALUES (?, ?, ?)",
		uuid.MustNewUUID().String(), cmrRelationUUID, cmrEP2)
	c.Assert(err, tc.ErrorIsNil)

	relations, err := New(s.TxnRunnerFactory(), s.modelUUID).GetRelationValidationData(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	// Only the local wordpress:db <-> mysql:db relation is returned.
	c.Assert(relations, tc.HasLen, 1)
	c.Check(relations[0].UUID, tc.Equals, localRelationUUID)
	c.Check(relations[0].ID, tc.Equals, 7)
}

// TestGetRelationValidationDataExcludesCMRBothSides verifies a relation
// where both endpoints are CMR-sourced is excluded from validation data.
func (s *migrationSuite) TestGetRelationValidationDataExcludesCMRBothSides(c *tc.C) {
	db := s.DB()

	cmrCharmUUID1 := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO charm (uuid, reference_name, source_id) VALUES (?, ?, 2)",
		cmrCharmUUID1, "remote-app-1")
	c.Assert(err, tc.ErrorIsNil)
	cmrCR1 := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, scope_id, interface, optional, capacity) VALUES (?, ?, 'endpoint1', 0, 0, 'token', false, 1)",
		cmrCR1, cmrCharmUUID1)
	c.Assert(err, tc.ErrorIsNil)
	cmrAppUUID1 := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, 'remote-app-1', 0, ?, ?)",
		cmrAppUUID1, cmrCharmUUID1, "656b4a82-e28c-53d6-a014-f0dd53417eb6")
	c.Assert(err, tc.ErrorIsNil)

	cmrCharmUUID2 := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO charm (uuid, reference_name, source_id) VALUES (?, ?, 2)",
		cmrCharmUUID2, "remote-app-2")
	c.Assert(err, tc.ErrorIsNil)
	cmrCR2 := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, scope_id, interface, optional, capacity) VALUES (?, ?, 'endpoint2', 1, 0, 'token', false, 1)",
		cmrCR2, cmrCharmUUID2)
	c.Assert(err, tc.ErrorIsNil)
	cmrAppUUID2 := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, 'remote-app-2', 0, ?, ?)",
		cmrAppUUID2, cmrCharmUUID2, "656b4a82-e28c-53d6-a014-f0dd53417eb6")
	c.Assert(err, tc.ErrorIsNil)

	cmrRelationUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO relation (uuid, life_id, relation_id, scope_id) VALUES (?, 0, 99, 1)",
		cmrRelationUUID)
	c.Assert(err, tc.ErrorIsNil)

	addCMREndpoint := func(appUUID, charmRelUUID string) {
		epUUID := uuid.MustNewUUID().String()
		_, err := db.ExecContext(c.Context(),
			"INSERT INTO application_endpoint (uuid, application_uuid, space_uuid, charm_relation_uuid) VALUES (?, ?, NULL, ?)",
			epUUID, appUUID, charmRelUUID)
		c.Assert(err, tc.ErrorIsNil)
		_, err = db.ExecContext(c.Context(),
			"INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid) VALUES (?, ?, ?)",
			uuid.MustNewUUID().String(), cmrRelationUUID, epUUID)
		c.Assert(err, tc.ErrorIsNil)
	}
	addCMREndpoint(cmrAppUUID1, cmrCR1)
	addCMREndpoint(cmrAppUUID2, cmrCR2)

	relations, err := New(s.TxnRunnerFactory(), s.modelUUID).GetRelationValidationData(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(relations, tc.HasLen, 0)
}
func (s *migrationSuite) TestGetApplicationUnitNamesEmpty(c *tc.C) {
	units, err := New(s.TxnRunnerFactory(), s.modelUUID).GetApplicationUnitNames(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(units, tc.HasLen, 0)
}

// TestGetApplicationUnitNames verifies units are grouped by application.
func (s *migrationSuite) TestGetApplicationUnitNames(c *tc.C) {
	db := s.DB()
	charmUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO charm (uuid, reference_name, architecture_id) VALUES (?, ?, 0)",
		charmUUID, "wordpress")
	c.Assert(err, tc.ErrorIsNil)

	appUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, 'wordpress', 0, ?, ?)",
		appUUID, charmUUID, "656b4a82-e28c-53d6-a014-f0dd53417eb6")
	c.Assert(err, tc.ErrorIsNil)

	unitNetNodeUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(), "INSERT INTO net_node (uuid) VALUES (?)", unitNetNodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO unit (uuid, name, life_id, application_uuid, net_node_uuid, charm_uuid) VALUES (?, 'wordpress/0', 0, ?, ?, ?)",
		unitUUID, appUUID, unitNetNodeUUID, charmUUID)
	c.Assert(err, tc.ErrorIsNil)

	units, err := New(s.TxnRunnerFactory(), s.modelUUID).GetApplicationUnitNames(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(units, tc.DeepEquals, map[string][]string{"wordpress": {"wordpress/0"}})
}

// TestGetSubordinateUnitPrincipalsEmpty verifies a model with only principal
// units returns an empty map.
func (s *migrationSuite) TestGetSubordinateUnitPrincipalsEmpty(c *tc.C) {
	principals, err := New(s.TxnRunnerFactory(), s.modelUUID).GetSubordinateUnitPrincipals(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(principals, tc.HasLen, 0)
}

// TestGetSubordinateUnitPrincipals verifies subordinate units are mapped to the
// application of the principal unit they run alongside, and that principal
// units are absent from the result.
func (s *migrationSuite) TestGetSubordinateUnitPrincipals(c *tc.C) {
	db := s.DB()

	addUnit := func(appName, unitName string) string {
		charmUUID := uuid.MustNewUUID().String()
		_, err := db.ExecContext(c.Context(),
			"INSERT INTO charm (uuid, reference_name, architecture_id) VALUES (?, ?, 0)", charmUUID, appName)
		c.Assert(err, tc.ErrorIsNil)

		appUUID := uuid.MustNewUUID().String()
		_, err = db.ExecContext(c.Context(),
			"INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, 0, ?, ?)",
			appUUID, appName, charmUUID, "656b4a82-e28c-53d6-a014-f0dd53417eb6")
		c.Assert(err, tc.ErrorIsNil)

		netNodeUUID := uuid.MustNewUUID().String()
		_, err = db.ExecContext(c.Context(), "INSERT INTO net_node (uuid) VALUES (?)", netNodeUUID)
		c.Assert(err, tc.ErrorIsNil)

		unitUUID := uuid.MustNewUUID().String()
		_, err = db.ExecContext(c.Context(),
			"INSERT INTO unit (uuid, name, life_id, application_uuid, net_node_uuid, charm_uuid) VALUES (?, ?, 0, ?, ?, ?)",
			unitUUID, unitName, appUUID, netNodeUUID, charmUUID)
		c.Assert(err, tc.ErrorIsNil)
		return unitUUID
	}

	ubuntuUnitUUID := addUnit("ubuntu", "ubuntu/0")
	nrpeUnitUUID := addUnit("nrpe", "nrpe/0")

	_, err := db.ExecContext(c.Context(),
		"INSERT INTO unit_principal (unit_uuid, principal_uuid) VALUES (?, ?)", nrpeUnitUUID, ubuntuUnitUUID)
	c.Assert(err, tc.ErrorIsNil)

	principals, err := New(s.TxnRunnerFactory(), s.modelUUID).GetSubordinateUnitPrincipals(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(principals, tc.DeepEquals, map[string]string{"nrpe/0": "ubuntu"})
}

// TestGetApplicationUnitNamesExcludesNonAlive verifies dying and dead
// applications and units are not returned: they legitimately leave relation
// scope before removal, so they must not be checked for relation-unit
// consistency.
func (s *migrationSuite) TestGetApplicationUnitNamesExcludesNonAlive(c *tc.C) {
	db := s.DB()
	charmUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO charm (uuid, reference_name, architecture_id) VALUES (?, ?, 0)",
		charmUUID, "wordpress")
	c.Assert(err, tc.ErrorIsNil)

	appUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, 'wordpress', 0, ?, ?)",
		appUUID, charmUUID, "656b4a82-e28c-53d6-a014-f0dd53417eb6")
	c.Assert(err, tc.ErrorIsNil)

	// A dying and a dead unit on the alive application.
	for i, lifeID := range []int{1, 2} {
		unitNetNodeUUID := uuid.MustNewUUID().String()
		_, err = db.ExecContext(c.Context(), "INSERT INTO net_node (uuid) VALUES (?)", unitNetNodeUUID)
		c.Assert(err, tc.ErrorIsNil)
		_, err = db.ExecContext(c.Context(),
			"INSERT INTO unit (uuid, name, life_id, application_uuid, net_node_uuid, charm_uuid) VALUES (?, ?, ?, ?, ?, ?)",
			uuid.MustNewUUID().String(), fmt.Sprintf("wordpress/%d", i), lifeID, appUUID, unitNetNodeUUID, charmUUID)
		c.Assert(err, tc.ErrorIsNil)
	}

	// A dying application with an (anachronistically) alive unit.
	dyingAppUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, 'mysql', 1, ?, ?)",
		dyingAppUUID, charmUUID, "656b4a82-e28c-53d6-a014-f0dd53417eb6")
	c.Assert(err, tc.ErrorIsNil)
	unitNetNodeUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(), "INSERT INTO net_node (uuid) VALUES (?)", unitNetNodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO unit (uuid, name, life_id, application_uuid, net_node_uuid, charm_uuid) VALUES (?, 'mysql/0', 0, ?, ?, ?)",
		uuid.MustNewUUID().String(), dyingAppUUID, unitNetNodeUUID, charmUUID)
	c.Assert(err, tc.ErrorIsNil)

	units, err := New(s.TxnRunnerFactory(), s.modelUUID).GetApplicationUnitNames(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(units, tc.HasLen, 0)
}

// TestGetRelationUnitsByApplicationEmpty verifies no relation units return an
// empty map.
func (s *migrationSuite) TestGetRelationUnitsByApplicationEmpty(c *tc.C) {
	units, err := New(s.TxnRunnerFactory(), s.modelUUID).GetRelationUnitsByApplication(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(units, tc.HasLen, 0)
}

// TestGetRelationUnitsByApplication verifies in-scope units are grouped by
// relation and application.
func (s *migrationSuite) TestGetRelationUnitsByApplication(c *tc.C) {
	db := s.DB()
	charmUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO charm (uuid, reference_name, architecture_id) VALUES (?, ?, 0)",
		charmUUID, "wordpress")
	c.Assert(err, tc.ErrorIsNil)

	charmRelationUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, scope_id, interface, optional, capacity) VALUES (?, ?, 'db', 1, 1, 'mysql', false, 1)",
		charmRelationUUID, charmUUID)
	c.Assert(err, tc.ErrorIsNil)

	appUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, 'wordpress', 0, ?, ?)",
		appUUID, charmUUID, "656b4a82-e28c-53d6-a014-f0dd53417eb6")
	c.Assert(err, tc.ErrorIsNil)

	endpointUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO application_endpoint (uuid, application_uuid, space_uuid, charm_relation_uuid) VALUES (?, ?, NULL, ?)",
		endpointUUID, appUUID, charmRelationUUID)
	c.Assert(err, tc.ErrorIsNil)

	relationUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO relation (uuid, life_id, relation_id, scope_id) VALUES (?, 0, 7, 1)",
		relationUUID)
	c.Assert(err, tc.ErrorIsNil)

	relationEndpointUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid) VALUES (?, ?, ?)",
		relationEndpointUUID, relationUUID, endpointUUID)
	c.Assert(err, tc.ErrorIsNil)

	unitNetNodeUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(), "INSERT INTO net_node (uuid) VALUES (?)", unitNetNodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	unitUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO unit (uuid, name, life_id, application_uuid, net_node_uuid, charm_uuid) VALUES (?, 'wordpress/0', 0, ?, ?, ?)",
		unitUUID, appUUID, unitNetNodeUUID, charmUUID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = db.ExecContext(c.Context(),
		"INSERT INTO relation_unit (uuid, relation_endpoint_uuid, unit_uuid) VALUES (?, ?, ?)",
		uuid.MustNewUUID().String(), relationEndpointUUID, unitUUID)
	c.Assert(err, tc.ErrorIsNil)

	units, err := New(s.TxnRunnerFactory(), s.modelUUID).GetRelationUnitsByApplication(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(units, tc.DeepEquals, map[string]map[string][]string{
		relationUUID: {"wordpress": {"wordpress/0"}},
	})
}

// TestGetOfferUUIDsEmpty verifies that a model with no offers returns an empty
// slice and no error.
func (s *migrationSuite) TestGetOfferUUIDsEmpty(c *tc.C) {
	st := New(s.TxnRunnerFactory(), s.modelUUID)

	uuids, err := st.GetOfferUUIDs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuids, tc.HasLen, 0)
}

// TestGetOfferUUIDs verifies all hosted offer UUIDs are returned.
func (s *migrationSuite) TestGetOfferUUIDs(c *tc.C) {
	st := New(s.TxnRunnerFactory(), s.modelUUID)
	db := s.DB()

	offer1 := uuid.MustNewUUID().String()
	offer2 := uuid.MustNewUUID().String()
	for _, o := range []string{offer1, offer2} {
		_, err := db.ExecContext(c.Context(), `INSERT INTO offer (uuid, name) VALUES (?, ?)`, o, "offer-"+o[:8])
		c.Assert(err, tc.ErrorIsNil)
	}

	uuids, err := st.GetOfferUUIDs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuids, tc.SameContents, []string{offer1, offer2})
}

// TestGetThirdPartyOffererModelsEmpty verifies that a model with no remote
// applications returns an empty slice and no error.
func (s *migrationSuite) TestGetThirdPartyOffererModelsEmpty(c *tc.C) {
	st := New(s.TxnRunnerFactory(), s.modelUUID)

	models, err := st.GetThirdPartyOffererModels(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(models, tc.HasLen, 0)
}

// TestGetThirdPartyOffererModels verifies non-null offerer controller/model
// pairs are returned once, even when multiple remote applications reference
// the same third-party offerer model, and that pairs offered by this model's
// own controller are excluded.
func (s *migrationSuite) TestGetThirdPartyOffererModels(c *tc.C) {
	st := New(s.TxnRunnerFactory(), s.modelUUID)
	db := s.DB()

	charmUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO charm (uuid, reference_name, architecture_id) VALUES (?, ?, 0)",
		charmUUID, "remote")
	c.Assert(err, tc.ErrorIsNil)

	controllerUUID := uuid.MustNewUUID().String()
	modelUUID := uuid.MustNewUUID().String()
	otherControllerUUID := uuid.MustNewUUID().String()
	otherModelUUID := uuid.MustNewUUID().String()

	addRemoteOfferer := func(name string, controller any, model string) {
		appUUID := uuid.MustNewUUID().String()
		_, err := db.ExecContext(c.Context(),
			"INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, 0, ?, ?)",
			appUUID, name, charmUUID, "656b4a82-e28c-53d6-a014-f0dd53417eb6")
		c.Assert(err, tc.ErrorIsNil)
		_, err = db.ExecContext(c.Context(), `
INSERT INTO application_remote_offerer (
    uuid, life_id, application_uuid, offer_uuid, offer_url,
    offerer_controller_uuid, offerer_model_uuid, macaroon
) VALUES (?, 0, ?, ?, ?, ?, ?, 'macaroon')`,
			uuid.MustNewUUID().String(),
			appUUID,
			uuid.MustNewUUID().String(),
			"admin/"+name+".remote",
			controller,
			model,
		)
		c.Assert(err, tc.ErrorIsNil)
	}

	addRemoteOfferer("remote-a", controllerUUID, modelUUID)
	addRemoteOfferer("remote-b", controllerUUID, modelUUID)
	addRemoteOfferer("remote-c", otherControllerUUID, otherModelUUID)
	addRemoteOfferer("remote-null", nil, uuid.MustNewUUID().String())
	addRemoteOfferer("remote-local", s.controllerUUID.String(), uuid.MustNewUUID().String())

	models, err := st.GetThirdPartyOffererModels(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(models, tc.SameContents, []modelmigrationinternal.OffererModel{
		{ControllerUUID: controllerUUID, ModelUUID: modelUUID},
		{ControllerUUID: otherControllerUUID, ModelUUID: otherModelUUID},
	})
}

// TestGetCredentialValidationInfo verifies the model's identity, cloud
// placement and stored configuration are read together from the model record.
func (s *migrationSuite) TestGetCredentialValidationInfo(c *tc.C) {
	db := s.DB()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO model_config (key, value) VALUES ('ftp-proxy', 'http://proxy'), ('apt-mirror', 'http://mirror')")
	c.Assert(err, tc.ErrorIsNil)

	info, err := New(s.TxnRunnerFactory(), s.modelUUID).GetCredentialValidationInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.ControllerUUID, tc.Equals, s.controllerUUID.String())
	c.Check(info.ModelType, tc.Equals, "iaas")
	c.Check(info.CloudName, tc.Equals, "aws")
	c.Check(info.CloudType, tc.Equals, "ec2")
	c.Check(info.CloudRegion, tc.Equals, "myregion")
	c.Check(info.Config["ftp-proxy"], tc.Equals, "http://proxy")
	c.Check(info.Config["apt-mirror"], tc.Equals, "http://mirror")
}

// TestGetMachineInstanceID verifies the provisioned machine's instance ID is
// returned for its machine UUID.
func (s *migrationSuite) TestGetMachineInstanceID(c *tc.C) {
	machineState := machinestate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	_, machineNames, err := machineState.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineUUID, err := machineState.GetMachineUUID(c.Context(), machineNames[0])
	c.Assert(err, tc.ErrorIsNil)

	err = machineState.SetMachineCloudInstance(
		c.Context(),
		machineUUID.String(),
		instance.Id("instance-0"),
		"",
		"nonce",
		nil,
	)
	c.Assert(err, tc.ErrorIsNil)

	instanceID, err := New(s.TxnRunnerFactory(), s.modelUUID).GetMachineInstanceID(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(instanceID, tc.Equals, "instance-0")
}

// TestGetMachineInstanceIDNotProvisioned verifies an error satisfying
// [machineerrors.NotProvisioned] is returned when the machine has no cloud
// instance.
func (s *migrationSuite) TestGetMachineInstanceIDNotProvisioned(c *tc.C) {
	machineState := machinestate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	_, machineNames, err := machineState.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineUUID, err := machineState.GetMachineUUID(c.Context(), machineNames[0])
	c.Assert(err, tc.ErrorIsNil)

	_, err = New(s.TxnRunnerFactory(), s.modelUUID).GetMachineInstanceID(c.Context(), machineUUID.String())
	c.Assert(err, tc.ErrorIs, machineerrors.NotProvisioned)
}
