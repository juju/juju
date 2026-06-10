// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"fmt"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	usertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	domainagentbinary "github.com/juju/juju/domain/agentbinary"
	"github.com/juju/juju/domain/deployment"
	domainmachine "github.com/juju/juju/domain/machine"
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

// TestGetAllInstanceIDs is asserting the happy path of getting all instance
// IDs for the model.
func (s *migrationSuite) TestGetAllInstanceIDs(c *tc.C) {
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

	instanceIDs, err := New(s.TxnRunnerFactory(), s.modelUUID).GetAllInstanceIDs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(instanceIDs, tc.HasLen, 2)
	c.Check(instanceIDs.Values(), tc.SameContents, []string{"instance-0", "instance-1"})
}

// TestEmptyInstanceIDs tests that no error is returned when there are no
// instances in the model.
func (s *migrationSuite) TestEmptyInstanceIDs(c *tc.C) {
	instanceIDs, err := New(s.TxnRunnerFactory(), s.modelUUID).GetAllInstanceIDs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(instanceIDs, tc.HasLen, 0)
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

// TestGetOffererModelsEmpty verifies that a model with no remote applications
// returns an empty slice and no error.
func (s *migrationSuite) TestGetOffererModelsEmpty(c *tc.C) {
	st := New(s.TxnRunnerFactory(), s.modelUUID)

	models, err := st.GetOffererModels(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(models, tc.HasLen, 0)
}

// TestGetOffererModels verifies non-null offerer controller/model pairs are
// returned once, even when multiple remote applications reference the same
// third-party offerer model.
func (s *migrationSuite) TestGetOffererModels(c *tc.C) {
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
	addRemoteOfferer("remote-local", nil, uuid.MustNewUUID().String())

	models, err := st.GetOffererModels(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(models, tc.SameContents, []modelmigrationinternal.OffererModel{
		{ControllerUUID: controllerUUID, ModelUUID: modelUUID},
		{ControllerUUID: otherControllerUUID, ModelUUID: otherModelUUID},
	})
}
