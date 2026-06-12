// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	corecloud "github.com/juju/juju/core/cloud"
	cloudtesting "github.com/juju/juju/core/cloud/testing"
	corecredential "github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/migration"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	accessstate "github.com/juju/juju/domain/access/state"
	dbcloud "github.com/juju/juju/domain/cloud/state"
	"github.com/juju/juju/domain/cloudimagemetadata"
	"github.com/juju/juju/domain/credential"
	credentialstate "github.com/juju/juju/domain/credential/state"
	"github.com/juju/juju/domain/model"
	"github.com/juju/juju/domain/model/state/controller"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	modelmigrationinternal "github.com/juju/juju/domain/modelmigration/internal"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/secretbackend/bootstrap"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ControllerModelSuite

	modelState *controller.State

	controllerModelUUID coremodel.UUID

	modelUUID coremodel.UUID
	userUUID  user.UUID
	userName  user.Name

	cloudUUID      corecloud.UUID
	credentialUUID corecredential.UUID
}

func TestMigrationSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.ControllerModelSuite.SetUpTest(c)

	s.modelState = controller.NewState(s.TxnRunnerFactory())

	s.controllerModelUUID = tc.Must(c, coremodel.NewUUID)

	// We need to generate a user in the database so that we can set the model
	// owner.
	s.modelUUID = tc.Must(c, coremodel.NewUUID)
	s.userName = usertesting.GenNewName(c, "test-user")
	accessState := accessstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	s.userUUID = usertesting.GenUserUUID(c)
	err := accessState.AddUser(
		c.Context(),
		s.userUUID,
		s.userName,
		s.userName.Name(),
		false,
		s.userUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	// We need to generate a cloud in the database so that we can set the model
	// cloud.
	cloudSt := dbcloud.NewState(s.TxnRunnerFactory())
	s.cloudUUID = cloudtesting.GenCloudUUID(c)
	err = cloudSt.CreateCloud(c.Context(), s.userName, s.cloudUUID.String(),
		cloud.Cloud{
			Name:      "my-cloud",
			Type:      "ec2",
			AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
			Regions: []cloud.Region{
				{
					Name: "my-region",
				},
			},
		})
	c.Assert(err, tc.ErrorIsNil)
	err = cloudSt.CreateCloud(c.Context(), s.userName, uuid.MustNewUUID().String(),
		cloud.Cloud{
			Name:      "other-cloud",
			Type:      "ec2",
			AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
			Regions: []cloud.Region{
				{
					Name: "other-region",
				},
			},
		})
	c.Assert(err, tc.ErrorIsNil)

	// We need to generate a cloud credential in the database so that we can set
	// the models cloud credential.
	cred := credential.CloudCredentialInfo{
		Label:    "foobar",
		AuthType: string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	}

	credSt := credentialstate.NewState(s.TxnRunnerFactory())
	err = credSt.UpsertCloudCredential(
		c.Context(), corecredential.Key{
			Cloud: "my-cloud",
			Owner: usertesting.GenNewName(c, "test-user"),
			Name:  "foobar",
		},
		cred,
	)
	c.Assert(err, tc.ErrorIsNil)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT uuid FROM cloud_credential WHERE owner_uuid = ? AND name = ? AND cloud_uuid = ?", s.userUUID, "foobar", s.cloudUUID).
			Scan(&s.credentialUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	err = credSt.UpsertCloudCredential(
		c.Context(), corecredential.Key{
			Cloud: "other-cloud",
			Owner: usertesting.GenNewName(c, "test-user"),
			Name:  "foobar",
		},
		cred,
	)
	c.Assert(err, tc.ErrorIsNil)

	err = bootstrap.CreateDefaultBackends(coremodel.IAAS)(c.Context(), s.ControllerTxnRunner(), s.TxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	s.createControllerModel(c, s.controllerModelUUID, s.userUUID)
	s.createModel(c, s.modelUUID, s.userUUID)
}

// TestDeleteModelImportingStatusSuccess tests that clearing an existing
// model_migration_import entry succeeds and actually removes the entry from the
// database.
func (s *stateSuite) TestDeleteModelImportingStatusSuccess(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	// Insert a model_migration_import entry.
	migratingUUID := uuid.MustNewUUID().String()
	sourceMigrationUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid) VALUES (?, ?, ?)",
		migratingUUID, s.modelUUID, sourceMigrationUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Verify the entry exists.
	var count int
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migration_import WHERE model_uuid = ?",
		s.modelUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)

	// Clear the importing status.
	err = st.DeleteModelImportingStatus(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// Verify the entry has been deleted.
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migration_import WHERE model_uuid = ?",
		s.modelUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

// TestDeleteModelImportingStatusNoEntry tests that clearing a non-existent
// model_migration_import entry succeeds without error (idempotent behavior).
func (s *stateSuite) TestDeleteModelImportingStatusNoEntry(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	// Verify no entry exists.
	var count int
	err := db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migration_import WHERE model_uuid = ?",
		s.modelUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)

	// Clear should succeed even when there's nothing to delete.
	err = st.DeleteModelImportingStatus(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// Verify still no entries.
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migration_import WHERE model_uuid = ?",
		s.modelUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

// TestDeleteModelImportingStatusVerifyCorrectEntry tests that clearing
// deletes the correct entry and verifies by UUID.
func (s *stateSuite) TestDeleteModelImportingStatusVerifyCorrectEntry(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	// Insert a model_migration_import entry with a specific UUID.
	migratingUUID := uuid.MustNewUUID().String()
	sourceMigrationUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid) VALUES (?, ?, ?)",
		migratingUUID, s.modelUUID, sourceMigrationUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Verify we can query the specific entry by its UUID.
	var retrievedModelUUID string
	err = db.QueryRowContext(c.Context(),
		"SELECT model_uuid FROM model_migration_import WHERE uuid = ?",
		migratingUUID).Scan(&retrievedModelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(retrievedModelUUID, tc.Equals, s.modelUUID.String())

	// Clear the importing status.
	err = st.DeleteModelImportingStatus(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// Verify the entry no longer exists.
	var count int
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migration_import WHERE uuid = ?",
		migratingUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

// TestDeleteModelImportingStatusWrongModelUUID tests that clearing with a
// non-existent model UUID succeeds without error and doesn't affect other
// entries.
func (s *stateSuite) TestDeleteModelImportingStatusWrongModelUUID(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	// Insert a model_migration_import entry.
	migratingUUID := uuid.MustNewUUID().String()
	sourceMigrationUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid) VALUES (?, ?, ?)",
		migratingUUID, s.modelUUID, sourceMigrationUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Try to clear with a different (non-existent) model UUID.
	differentModelUUID := uuid.MustNewUUID().String()
	err = st.DeleteModelImportingStatus(c.Context(), differentModelUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Verify the original entry still exists.
	var count int
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migration_import WHERE model_uuid = ?",
		s.modelUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)
}

// TestDeleteModelImportingStatusIdempotent tests that calling
// DeleteModelImportingStatus multiple times is safe and idempotent.
func (s *stateSuite) TestDeleteModelImportingStatusIdempotent(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	// Insert a model_migration_import entry.
	migratingUUID := uuid.MustNewUUID().String()
	sourceMigrationUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid) VALUES (?, ?, ?)",
		migratingUUID, s.modelUUID, sourceMigrationUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Clear the importing status multiple times.
	err = st.DeleteModelImportingStatus(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	err = st.DeleteModelImportingStatus(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	err = st.DeleteModelImportingStatus(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// Verify no entries exist.
	var count int
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migration_import WHERE model_uuid = ?",
		s.modelUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

func (s *stateSuite) TestGetControllerTargetVersion(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	ver, err := st.GetControllerTargetVersion(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ver, tc.Equals, jujuversion.Current.String())
}

// newMigrationSpec builds a migration spec targeting a freshly-generated
// external controller UUID.
func (s *stateSuite) newMigrationSpec() modelmigrationinternal.MigrationSpec {
	return modelmigrationinternal.MigrationSpec{
		MigrationUUID:         uuid.MustNewUUID().String(),
		ModelUUID:             s.modelUUID.String(),
		TargetControllerUUID:  uuid.MustNewUUID().String(),
		TargetControllerAlias: "target-controller",
		TargetAddrs: []modelmigrationinternal.ExternalControllerAddress{
			{UUID: uuid.MustNewUUID().String(), Address: "10.0.0.1:17070"},
			{UUID: uuid.MustNewUUID().String(), Address: "10.0.0.2:17070"},
		},
		TargetCACert: "ca-cert-data",
		TargetUser:   "admin",
		TargetToken:  "super-token",
	}
}

// TestInsertExport asserts that recording a new export migration writes the
// export row, its target-auth companion, the seeded phase history, and ensures
// the target external controller and its addresses exist.
func (s *stateSuite) TestInsertExport(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	spec := s.newMigrationSpec()
	err := st.InsertExport(c.Context(), spec)
	c.Assert(err, tc.ErrorIsNil)

	// Export row exists, in QUIESCE (phase id 1).
	var (
		modelUUID  string
		targetUUID string
		phaseID    int
	)
	err = db.QueryRowContext(c.Context(),
		"SELECT model_uuid, target_controller_uuid, current_phase_id FROM model_migration_export WHERE uuid = ?",
		spec.MigrationUUID).Scan(&modelUUID, &targetUUID, &phaseID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelUUID, tc.Equals, s.modelUUID.String())
	c.Check(targetUUID, tc.Equals, spec.TargetControllerUUID)
	c.Check(phaseID, tc.Equals, 1)

	// Target auth companion row exists.
	var user, token string
	err = db.QueryRowContext(c.Context(),
		"SELECT target_user, target_token FROM model_migration_export_target_auth WHERE migration_uuid = ?",
		spec.MigrationUUID).Scan(&user, &token)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(user, tc.Equals, "admin")
	c.Check(token, tc.Equals, "super-token")

	// Phase history seeded with QUIESCE.
	var phaseCount int
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migration_export_phase WHERE migration_uuid = ? AND phase_id = 1",
		spec.MigrationUUID).Scan(&phaseCount)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(phaseCount, tc.Equals, 1)

	// Target external controller + addresses created.
	var caCert string
	err = db.QueryRowContext(c.Context(),
		"SELECT ca_cert FROM external_controller WHERE uuid = ?", spec.TargetControllerUUID).Scan(&caCert)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(caCert, tc.Equals, "ca-cert-data")

	var addrCount int
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM external_controller_address WHERE controller_uuid = ?", spec.TargetControllerUUID).Scan(&addrCount)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addrCount, tc.Equals, 2)
}

// TestInsertExportAlreadyActive asserts that a second active export for the same
// model is rejected by the unique partial index and surfaced as
// [modelmigrationerrors.ErrMigrationAlreadyActive].
func (s *stateSuite) TestInsertExportAlreadyActive(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	err := st.InsertExport(c.Context(), s.newMigrationSpec())
	c.Assert(err, tc.ErrorIsNil)

	err = st.InsertExport(c.Context(), s.newMigrationSpec())
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrMigrationAlreadyActive)
}

// TestInsertExportAfterEnded asserts a new export is allowed once a previous one
// has ended.
func (s *stateSuite) TestInsertExportAfterEnded(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	first := s.newMigrationSpec()
	err := st.InsertExport(c.Context(), first)
	c.Assert(err, tc.ErrorIsNil)

	err = st.MarkExportEnded(c.Context(), first.MigrationUUID, migration.ABORTDONE)
	c.Assert(err, tc.ErrorIsNil)

	err = st.InsertExport(c.Context(), s.newMigrationSpec())
	c.Assert(err, tc.ErrorIsNil)
}

// TestGetActiveExportUUID returns the UUID of the active export migration for
// the model.
func (s *stateSuite) TestGetActiveExportUUID(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	spec := s.newMigrationSpec()
	err := st.InsertExport(c.Context(), spec)
	c.Assert(err, tc.ErrorIsNil)

	got, err := st.GetActiveExportUUID(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.Equals, spec.MigrationUUID)
}

// TestGetActiveExportUUIDNotFound asserts ErrMigrationNotFound is returned when
// the model has no export migration.
func (s *stateSuite) TestGetActiveExportUUIDNotFound(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	_, err := st.GetActiveExportUUID(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrMigrationNotFound)
}

// TestGetActiveExportUUIDIgnoresTerminalExport asserts a terminal export is no
// longer considered active.
func (s *stateSuite) TestGetActiveExportUUIDIgnoresTerminalExport(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	spec := s.newMigrationSpec()
	err := st.InsertExport(c.Context(), spec)
	c.Assert(err, tc.ErrorIsNil)

	err = st.MarkExportEnded(c.Context(), spec.MigrationUUID, migration.ABORTDONE)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.GetActiveExportUUID(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrMigrationNotFound)
}

func (s *stateSuite) TestInsertExportUpdatesExternalController(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	first := s.newMigrationSpec()
	err := st.InsertExport(c.Context(), first)
	c.Assert(err, tc.ErrorIsNil)
	err = st.MarkExportEnded(c.Context(), first.MigrationUUID, migration.ABORTDONE)
	c.Assert(err, tc.ErrorIsNil)

	second := s.newMigrationSpec()
	second.TargetControllerUUID = first.TargetControllerUUID
	second.TargetControllerAlias = "updated-controller"
	second.TargetCACert = "updated-ca"
	second.TargetAddrs = []modelmigrationinternal.ExternalControllerAddress{
		{UUID: uuid.MustNewUUID().String(), Address: "10.0.1.1:17070"},
	}
	err = st.InsertExport(c.Context(), second)
	c.Assert(err, tc.ErrorIsNil)

	var alias, caCert string
	err = db.QueryRowContext(c.Context(),
		"SELECT alias, ca_cert FROM external_controller WHERE uuid = ?",
		first.TargetControllerUUID).Scan(&alias, &caCert)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(alias, tc.Equals, "updated-controller")
	c.Check(caCert, tc.Equals, "updated-ca")

	var addresses []string
	rows, err := db.QueryContext(c.Context(),
		"SELECT address FROM external_controller_address WHERE controller_uuid = ?",
		first.TargetControllerUUID)
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()
	for rows.Next() {
		var address string
		err := rows.Scan(&address)
		c.Assert(err, tc.ErrorIsNil)
		addresses = append(addresses, address)
	}
	c.Assert(rows.Err(), tc.ErrorIsNil)
	c.Check(addresses, tc.SameContents, []string{"10.0.1.1:17070"})
}

func (s *stateSuite) TestInsertExportExternalControllerMatchNoDuplicate(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	first := s.newMigrationSpec()
	err := st.InsertExport(c.Context(), first)
	c.Assert(err, tc.ErrorIsNil)
	err = st.MarkExportEnded(c.Context(), first.MigrationUUID, migration.ABORTDONE)
	c.Assert(err, tc.ErrorIsNil)

	second := s.newMigrationSpec()
	second.TargetControllerUUID = first.TargetControllerUUID
	second.TargetControllerAlias = first.TargetControllerAlias
	second.TargetCACert = first.TargetCACert
	second.TargetAddrs = []modelmigrationinternal.ExternalControllerAddress{
		{UUID: uuid.MustNewUUID().String(), Address: "10.0.0.2:17070"},
		{UUID: uuid.MustNewUUID().String(), Address: "10.0.0.1:17070"},
	}
	err = st.InsertExport(c.Context(), second)
	c.Assert(err, tc.ErrorIsNil)

	var addrCount int
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM external_controller_address WHERE controller_uuid = ?",
		first.TargetControllerUUID).Scan(&addrCount)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addrCount, tc.Equals, 2)
}

// TestGetActiveExport asserts the active export is returned with its
// reconstructed target connection details.
func (s *stateSuite) TestGetActiveExport(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	spec := s.newMigrationSpec()
	err := st.InsertExport(c.Context(), spec)
	c.Assert(err, tc.ErrorIsNil)

	mig, err := st.GetActiveExport(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mig.UUID, tc.Equals, spec.MigrationUUID)
	c.Check(mig.Phase, tc.Equals, migration.QUIESCE)
	c.Check(mig.Target.ControllerUUID, tc.Equals, spec.TargetControllerUUID)
	c.Check(mig.Target.ControllerAlias, tc.Equals, "target-controller")
	c.Check(mig.Target.CACert, tc.Equals, "ca-cert-data")
	c.Check(mig.Target.User, tc.Equals, "admin")
	c.Check(mig.Target.Token, tc.Equals, "super-token")
	c.Check(mig.Target.Addrs, tc.SameContents, []string{"10.0.0.1:17070", "10.0.0.2:17070"})
}

// TestGetActiveExportNotFound asserts a missing active export is reported as
// [modelmigrationerrors.ErrMigrationNotFound].
func (s *stateSuite) TestGetActiveExportNotFound(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	_, err := st.GetActiveExport(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrMigrationNotFound)
}

// TestSetPhaseValidTransition asserts a valid transition updates the current
// phase and records phase history.
func (s *stateSuite) TestSetPhaseValidTransition(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	spec := s.newMigrationSpec()
	err := st.InsertExport(c.Context(), spec)
	c.Assert(err, tc.ErrorIsNil)

	err = st.SetPhase(c.Context(), spec.MigrationUUID, migration.IMPORT)
	c.Assert(err, tc.ErrorIsNil)

	var phaseID int
	err = db.QueryRowContext(c.Context(),
		"SELECT current_phase_id FROM model_migration_export WHERE uuid = ?", spec.MigrationUUID).Scan(&phaseID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(phaseID, tc.Equals, 2)

	var historyCount int
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migration_export_phase WHERE migration_uuid = ? AND phase_id = 2",
		spec.MigrationUUID).Scan(&historyCount)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(historyCount, tc.Equals, 1)
}

// TestSetPhaseInvalidTransition asserts an invalid transition is rejected with
// [modelmigrationerrors.ErrPhaseTransitionInvalid] and leaves the phase unchanged.
func (s *stateSuite) TestSetPhaseInvalidTransition(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	spec := s.newMigrationSpec()
	err := st.InsertExport(c.Context(), spec)
	c.Assert(err, tc.ErrorIsNil)

	// QUIESCE cannot jump straight to SUCCESS.
	err = st.SetPhase(c.Context(), spec.MigrationUUID, migration.SUCCESS)
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrPhaseTransitionInvalid)

	var phaseID int
	err = db.QueryRowContext(c.Context(),
		"SELECT current_phase_id FROM model_migration_export WHERE uuid = ?", spec.MigrationUUID).Scan(&phaseID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(phaseID, tc.Equals, 1)
}

// TestSetPhaseIdempotent asserts re-setting the current phase is a no-op.
func (s *stateSuite) TestSetPhaseIdempotent(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	spec := s.newMigrationSpec()
	err := st.InsertExport(c.Context(), spec)
	c.Assert(err, tc.ErrorIsNil)

	err = st.SetPhase(c.Context(), spec.MigrationUUID, migration.QUIESCE)
	c.Assert(err, tc.ErrorIsNil)
}

// TestSetPhaseFullSuccessCycle walks the full success phase chain and asserts
// reaching the terminal DONE phase ends the export.
func (s *stateSuite) TestSetPhaseFullSuccessCycle(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	spec := s.newMigrationSpec()
	err := st.InsertExport(c.Context(), spec)
	c.Assert(err, tc.ErrorIsNil)

	for _, phase := range []migration.Phase{
		migration.IMPORT,
		migration.VALIDATION,
		migration.SUCCESS,
		migration.LOGTRANSFER,
		migration.REAP,
		migration.DONE,
	} {
		err = st.SetPhase(c.Context(), spec.MigrationUUID, phase)
		c.Assert(err, tc.ErrorIsNil, tc.Commentf("phase %v", phase))
	}

	// Reaching DONE ends the export.
	var phaseID int
	err = db.QueryRowContext(c.Context(),
		"SELECT current_phase_id FROM model_migration_export WHERE uuid = ?", spec.MigrationUUID).Scan(&phaseID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(phaseID, tc.Equals, 8)

	// No active export remains.
	_, err = st.GetActiveExport(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrMigrationNotFound)
}

// TestSetStatusMessage asserts the current status message is updated in place.
func (s *stateSuite) TestSetStatusMessage(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	spec := s.newMigrationSpec()
	err := st.InsertExport(c.Context(), spec)
	c.Assert(err, tc.ErrorIsNil)

	err = st.SetStatusMessage(c.Context(), spec.MigrationUUID, "uploading binaries")
	c.Assert(err, tc.ErrorIsNil)
	err = st.SetStatusMessage(c.Context(), spec.MigrationUUID, "import complete")
	c.Assert(err, tc.ErrorIsNil)

	var count int
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migration_export_status WHERE migration_uuid = ?", spec.MigrationUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)

	var message string
	err = db.QueryRowContext(c.Context(),
		"SELECT message FROM model_migration_export_status WHERE migration_uuid = ?", spec.MigrationUUID).Scan(&message)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(message, tc.Equals, "import complete")
}

// TestMinionReports asserts minion reports are recorded and aggregated by
// success for the requested phase.
func (s *stateSuite) TestMinionReports(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	spec := s.newMigrationSpec()
	err := st.InsertExport(c.Context(), spec)
	c.Assert(err, tc.ErrorIsNil)

	err = st.InsertMinionReport(c.Context(), spec.MigrationUUID, migration.QUIESCE, "machine-0", true)
	c.Assert(err, tc.ErrorIsNil)
	err = st.InsertMinionReport(c.Context(), spec.MigrationUUID, migration.QUIESCE, "unit-foo-0", false)
	c.Assert(err, tc.ErrorIsNil)
	// A report for a different phase must not be aggregated.
	err = st.InsertMinionReport(c.Context(), spec.MigrationUUID, migration.IMPORT, "machine-1", true)
	c.Assert(err, tc.ErrorIsNil)

	reports, err := st.AggregateMinionReports(c.Context(), spec.MigrationUUID, migration.QUIESCE)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(reports.Phase, tc.Equals, migration.QUIESCE)
	c.Check(reports.Succeeded, tc.SameContents, []string{"machine-0"})
	c.Check(reports.Failed, tc.SameContents, []string{"unit-foo-0"})
}

// TestInsertMinionReportIdempotent asserts that re-submitting a report for the
// same agent and phase with the same success value is an idempotent no-op.
func (s *stateSuite) TestInsertMinionReportIdempotent(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	spec := s.newMigrationSpec()
	err := st.InsertExport(c.Context(), spec)
	c.Assert(err, tc.ErrorIsNil)

	err = st.InsertMinionReport(c.Context(), spec.MigrationUUID, migration.QUIESCE, "machine-0", true)
	c.Assert(err, tc.ErrorIsNil)
	err = st.InsertMinionReport(c.Context(), spec.MigrationUUID, migration.QUIESCE, "machine-0", true)
	c.Assert(err, tc.ErrorIsNil)

	reports, err := st.AggregateMinionReports(c.Context(), spec.MigrationUUID, migration.QUIESCE)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(reports.Succeeded, tc.SameContents, []string{"machine-0"})
	c.Check(reports.Failed, tc.HasLen, 0)
}

// TestInsertMinionReportConflictRejected asserts that a re-submitted report for
// the same agent and phase with a different success value is rejected rather
// than silently overwriting the originally recorded result.
func (s *stateSuite) TestInsertMinionReportConflictRejected(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	spec := s.newMigrationSpec()
	err := st.InsertExport(c.Context(), spec)
	c.Assert(err, tc.ErrorIsNil)

	err = st.InsertMinionReport(c.Context(), spec.MigrationUUID, migration.QUIESCE, "machine-0", false)
	c.Assert(err, tc.ErrorIsNil)
	err = st.InsertMinionReport(c.Context(), spec.MigrationUUID, migration.QUIESCE, "machine-0", true)
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrConflictingMinionReport)

	// The originally recorded result is preserved.
	reports, err := st.AggregateMinionReports(c.Context(), spec.MigrationUUID, migration.QUIESCE)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(reports.Failed, tc.SameContents, []string{"machine-0"})
	c.Check(reports.Succeeded, tc.HasLen, 0)
}

// TestMarkExportEnded asserts an export can be force-ended and is then no longer
// active.
func (s *stateSuite) TestMarkExportEnded(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	spec := s.newMigrationSpec()
	err := st.InsertExport(c.Context(), spec)
	c.Assert(err, tc.ErrorIsNil)

	err = st.MarkExportEnded(c.Context(), spec.MigrationUUID, migration.ABORTDONE)
	c.Assert(err, tc.ErrorIsNil)

	var phaseID int
	err = db.QueryRowContext(c.Context(),
		"SELECT current_phase_id FROM model_migration_export WHERE uuid = ?", spec.MigrationUUID).Scan(&phaseID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(phaseID, tc.Equals, 10)

	// Ending an already-ended export reports not found.
	err = st.MarkExportEnded(c.Context(), spec.MigrationUUID, migration.ABORTDONE)
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrMigrationNotFound)
}

// TestGetMigrationMode asserts the derived mode reflects active export/import
// state.
func (s *stateSuite) TestGetMigrationMode(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	// No migration: none.
	mode, err := st.GetMigrationMode(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mode, tc.Equals, modelmigration.MigrationModeNone)

	// Active export: exporting.
	spec := s.newMigrationSpec()
	err = st.InsertExport(c.Context(), spec)
	c.Assert(err, tc.ErrorIsNil)
	mode, err = st.GetMigrationMode(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mode, tc.Equals, modelmigration.MigrationModeExporting)

	// After ending the export and adding an import claim: importing.
	err = st.MarkExportEnded(c.Context(), spec.MigrationUUID, migration.DONE)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(c.Context(),
		"INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid) VALUES (?, ?, 'src')",
		uuid.MustNewUUID().String(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	mode, err = st.GetMigrationMode(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mode, tc.Equals, modelmigration.MigrationModeImporting)
}

// TestGetControllerModelInfoIdentity verifies that the model bootstrap
// identity, credential, the seeded admin model permission and the model secret
// backend are read back in target-portable form for a model created by the
// test fixture.
func (s *stateSuite) TestGetControllerModelInfoIdentity(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	info, err := st.GetControllerModelInfo(c.Context(), s.modelUUID.String(), nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(info.ModelInfo.UUID, tc.Equals, s.modelUUID.String())
	c.Check(info.ModelInfo.Name, tc.Equals, "my-test-model")
	c.Check(info.ModelInfo.Qualifier, tc.Equals, "prod")
	c.Check(info.ModelInfo.Type, tc.Equals, "iaas")
	c.Check(info.ModelInfo.Cloud, tc.Equals, "my-cloud")
	c.Check(info.ModelInfo.CloudRegion, tc.Equals, "my-region")
	c.Check(info.ModelInfo.CredentialName, tc.Equals, "foobar")
	c.Check(info.ModelInfo.CredentialOwner, tc.Equals, "test-user")
	c.Check(info.ModelInfo.Life, tc.Equals, "alive")

	// The model was created with an admin user; that permission must travel.
	var foundAdmin bool
	for _, p := range info.Permissions {
		if p.ObjectType == "model" && p.GrantOn == s.modelUUID.String() &&
			p.SubjectName == "test-user" && p.Access == "admin" {
			foundAdmin = true
		}
	}
	c.Check(foundAdmin, tc.IsTrue, tc.Commentf("expected model admin permission, got %#v", info.Permissions))

	// The permission principal is recreatable as a model user.
	var foundUser bool
	for _, u := range info.Users {
		if u.Name == "test-user" {
			foundUser = true
		}
	}
	c.Check(foundUser, tc.IsTrue, tc.Commentf("expected test-user in users, got %#v", info.Users))

	c.Assert(info.ModelCredential, tc.NotNil)
	c.Check(info.ModelCredential.Cloud, tc.Equals, "my-cloud")
	c.Check(info.ModelCredential.Owner, tc.Equals, "test-user")
	c.Check(info.ModelCredential.Name, tc.Equals, "foobar")
	c.Check(info.ModelCredential.AuthType, tc.Equals, "access-key")
	c.Check(info.ModelCredential.Attributes, tc.DeepEquals, map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	})

	c.Assert(info.ModelCredential, tc.NotNil)
	// The fixture creates the model with the juju (internal) secret backend.
	c.Assert(info.SecretBackend, tc.NotNil)
	c.Check(info.SecretBackend.Name, tc.Not(tc.Equals), "")
}

// TestGetControllerModelInfoIncludesCredentialOwner verifies the user profile
// set includes the model credential owner even when that user has no model or
// offer permission grant.
func (s *stateSuite) TestGetControllerModelInfoIncludesCredentialOwner(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	db := s.DB()

	ownerName := usertesting.GenNewName(c, "credential-owner")
	ownerUUID := usertesting.GenUserUUID(c)
	accessState := accessstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	err := accessState.AddUser(
		c.Context(),
		ownerUUID,
		ownerName,
		ownerName.Name(),
		false,
		s.userUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	credSt := credentialstate.NewState(s.TxnRunnerFactory())
	err = credSt.UpsertCloudCredential(
		c.Context(), corecredential.Key{
			Cloud: "my-cloud",
			Owner: ownerName,
			Name:  "owner-only",
		},
		credential.CloudCredentialInfo{
			Label:    "owner-only",
			AuthType: "access-key",
			Attributes: map[string]string{
				"foo": "bar",
			},
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	var credUUID string
	err = db.QueryRowContext(c.Context(),
		`SELECT uuid FROM cloud_credential WHERE owner_uuid = ? AND name = ? AND cloud_uuid = ?`,
		ownerUUID, "owner-only", s.cloudUUID).Scan(&credUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(),
		`UPDATE model SET cloud_credential_uuid = ? WHERE uuid = ?`,
		credUUID, s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	info, err := st.GetControllerModelInfo(c.Context(), s.modelUUID.String(), nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	var foundOwner bool
	for _, u := range info.Users {
		if u.Name == ownerName.String() {
			foundOwner = true
			c.Check(u.Removed, tc.IsFalse)
			c.Check(u.External, tc.IsFalse)
		}
	}
	c.Check(foundOwner, tc.IsTrue, tc.Commentf("expected credential owner in users, got %#v", info.Users))
	c.Assert(info.ModelCredential, tc.NotNil)
	c.Check(info.ModelCredential.Owner, tc.Equals, ownerName.String())
}

// TestGetControllerModelInfoIncludesExternalCredentialOwner verifies the user
// profile set keeps the external flag for an external credential owner.
func (s *stateSuite) TestGetControllerModelInfoIncludesExternalCredentialOwner(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	db := s.DB()

	ownerName, err := user.NewName("credential-owner@external")
	c.Assert(err, tc.ErrorIsNil)
	ownerUUID := usertesting.GenUserUUID(c)
	accessState := accessstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	err = accessState.AddUser(
		c.Context(),
		ownerUUID,
		ownerName,
		ownerName.Name(),
		true,
		s.userUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	credSt := credentialstate.NewState(s.TxnRunnerFactory())
	err = credSt.UpsertCloudCredential(
		c.Context(), corecredential.Key{
			Cloud: "my-cloud",
			Owner: ownerName,
			Name:  "owner-only-external",
		},
		credential.CloudCredentialInfo{
			Label:    "owner-only-external",
			AuthType: "access-key",
			Attributes: map[string]string{
				"foo": "bar",
			},
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	var credUUID string
	err = db.QueryRowContext(c.Context(),
		`SELECT uuid FROM cloud_credential WHERE owner_uuid = ? AND name = ? AND cloud_uuid = ?`,
		ownerUUID, "owner-only-external", s.cloudUUID).Scan(&credUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(),
		`UPDATE model SET cloud_credential_uuid = ? WHERE uuid = ?`,
		credUUID, s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	info, err := st.GetControllerModelInfo(c.Context(), s.modelUUID.String(), nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	var foundOwner bool
	for _, u := range info.Users {
		if u.Name == ownerName.String() {
			foundOwner = true
			c.Check(u.Removed, tc.IsFalse)
			c.Check(u.External, tc.IsTrue)
		}
	}
	c.Check(foundOwner, tc.IsTrue, tc.Commentf("expected external credential owner in users, got %#v", info.Users))
}

// TestGetControllerModelInfoIncludesModelQualifierUser verifies the user
// profile set includes the model qualifier when it exists as a user.
func (s *stateSuite) TestGetControllerModelInfoIncludesModelQualifierUser(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	db := s.DB()

	ownerName := usertesting.GenNewName(c, "prod")
	ownerUUID := usertesting.GenUserUUID(c)
	accessState := accessstate.NewState(
		s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c),
	)
	err := accessState.AddUser(
		c.Context(),
		ownerUUID,
		ownerName,
		ownerName.Name(),
		false,
		s.userUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	info, err := st.GetControllerModelInfo(c.Context(), s.modelUUID.String(), nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	var foundQualifier bool
	for _, u := range info.Users {
		if u.Name == ownerName.String() {
			foundQualifier = true
		}
	}
	c.Check(foundQualifier, tc.IsTrue, tc.Commentf(
		"expected qualifier user in users, got %#v", info.Users,
	))

	_, err = db.ExecContext(c.Context(),
		`UPDATE user SET removed = TRUE WHERE uuid = ?`,
		ownerUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	info, err = st.GetControllerModelInfo(c.Context(), s.modelUUID.String(), nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	foundQualifier = false
	for _, u := range info.Users {
		if u.Name == ownerName.String() {
			foundQualifier = true
			c.Check(u.Removed, tc.IsTrue)
		}
	}
	c.Check(foundQualifier, tc.IsTrue, tc.Commentf(
		"expected removed qualifier user in users, got %#v", info.Users,
	))

	replacementUUID := usertesting.GenUserUUID(c)
	err = accessState.AddUser(
		c.Context(),
		replacementUUID,
		ownerName,
		"replacement",
		false,
		s.userUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	info, err = st.GetControllerModelInfo(c.Context(), s.modelUUID.String(), nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	var qualifierUsers []modelmigration.ModelUser
	for _, u := range info.Users {
		if u.Name == ownerName.String() {
			qualifierUsers = append(qualifierUsers, u)
		}
	}
	c.Assert(qualifierUsers, tc.HasLen, 2)
	var foundRemoved, foundReplacement bool
	for _, u := range qualifierUsers {
		if u.Removed {
			foundRemoved = true
			continue
		}
		if u.DisplayName == "replacement" {
			foundReplacement = true
		}
	}
	c.Check(foundRemoved, tc.IsTrue)
	c.Check(foundReplacement, tc.IsTrue)
}

// TestGetControllerModelInfoIncludesAuthorizedKeyOwner verifies the user
// profile set includes the owner of a model authorized key even when that user
// has no model or offer permission grant, so the target can resolve the key
// owner on import.
func (s *stateSuite) TestGetControllerModelInfoIncludesAuthorizedKeyOwner(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	db := s.DB()

	ownerName := usertesting.GenNewName(c, "key-owner")
	ownerUUID := usertesting.GenUserUUID(c)
	accessState := accessstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	err := accessState.AddUser(
		c.Context(),
		ownerUUID,
		ownerName,
		ownerName.Name(),
		false,
		s.userUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = db.ExecContext(c.Context(),
		`INSERT OR IGNORE INTO user_authentication (user_uuid, disabled) VALUES (?, FALSE)`,
		ownerUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	var keyID int64
	err = db.QueryRowContext(c.Context(),
		`INSERT INTO user_public_ssh_key (comment, fingerprint_hash_algorithm_id, fingerprint, public_key, user_uuid)
		 VALUES ('comment', 1, 'fp-owner', 'ssh-ed25519 AAAAownerkey', ?) RETURNING id`,
		ownerUUID.String()).Scan(&keyID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(),
		`INSERT INTO model_authorized_keys (model_uuid, user_public_ssh_key_id) VALUES (?, ?)`,
		s.modelUUID.String(), keyID)
	c.Assert(err, tc.ErrorIsNil)

	info, err := st.GetControllerModelInfo(c.Context(), s.modelUUID.String(), nil, nil)
	c.Assert(err, tc.ErrorIsNil)

	var foundOwner bool
	for _, u := range info.Users {
		if u.Name == ownerName.String() {
			foundOwner = true
			c.Check(u.Removed, tc.IsFalse)
			c.Check(u.External, tc.IsFalse)
		}
	}
	c.Check(foundOwner, tc.IsTrue, tc.Commentf("expected key owner in users, got %#v", info.Users))
}

// TestGetControllerModelInfoFullSet inserts a representative row for each
// remaining controller-scoped record and verifies they are all read back,
// including offer-scoped permissions and third-party external controllers
// selected from the caller-supplied inputs.
func (s *stateSuite) TestGetControllerModelInfoFullSet(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	db := s.DB()
	modelUUID := s.modelUUID.String()
	userUUID := s.userUUID.String()

	exec := func(query string, args ...any) {
		_, err := db.ExecContext(c.Context(), query, args...)
		c.Assert(err, tc.ErrorIsNil)
	}

	// Offer permission: consume on an offer hosted by this model.
	offerUUID := uuid.MustNewUUID().String()
	exec(`INSERT INTO permission (uuid, access_type_id, object_type_id, grant_on, grant_to)
	      VALUES (?, 2, 3, ?, ?)`, uuid.MustNewUUID().String(), offerUUID, userUUID)

	exec(`UPDATE cloud_credential SET invalid = TRUE, invalid_reason = 'expired'
	      WHERE uuid = ?`, s.credentialUUID.String())

	// Authorized key: requires user_authentication (for the view) + a public key.
	exec(`INSERT OR IGNORE INTO user_authentication (user_uuid, disabled) VALUES (?, FALSE)`, userUUID)
	var keyID int64
	err := db.QueryRowContext(c.Context(),
		`INSERT INTO user_public_ssh_key (comment, fingerprint_hash_algorithm_id, fingerprint, public_key, user_uuid)
		 VALUES ('comment', 1, 'fp', 'ssh-ed25519 AAAAkey', ?) RETURNING id`, userUUID).Scan(&keyID)
	c.Assert(err, tc.ErrorIsNil)
	exec(`INSERT INTO model_authorized_keys (model_uuid, user_public_ssh_key_id) VALUES (?, ?)`, modelUUID, keyID)

	// Leases: an application-leadership lease must surface as a leader; a
	// singular-controller lease and a lease pin are source-local runtime state
	// and must not travel.
	leaseUUID := uuid.MustNewUUID().String()
	start := time.Now().UTC().Truncate(time.Second)
	expiry := start.Add(time.Hour)
	exec(`INSERT INTO lease (uuid, lease_type_id, model_uuid, name, holder, start, expiry)
	      VALUES (?, 1, ?, 'app', 'app/0', ?, ?)`, leaseUUID, modelUUID, start, expiry)
	exec(`INSERT INTO lease_pin (uuid, lease_uuid, entity_id) VALUES (?, ?, 'machine-0')`,
		uuid.MustNewUUID().String(), leaseUUID)
	exec(`INSERT INTO lease (uuid, lease_type_id, model_uuid, name, holder, start, expiry)
	      VALUES (?, 0, ?, ?, 'controller-0', ?, ?)`,
		uuid.MustNewUUID().String(), modelUUID, modelUUID, start, expiry)

	// Secret backend reference for the model, against the internal backend.
	var backendUUID string
	err = db.QueryRowContext(c.Context(),
		`SELECT secret_backend_uuid FROM model_secret_backend WHERE model_uuid = ?`, modelUUID).Scan(&backendUUID)
	c.Assert(err, tc.ErrorIsNil)
	var backendName string
	err = db.QueryRowContext(c.Context(),
		`SELECT name FROM secret_backend WHERE uuid = ?`, backendUUID).Scan(&backendName)
	c.Assert(err, tc.ErrorIsNil)
	revUUID := uuid.MustNewUUID().String()
	secretID := "secret:" + uuid.MustNewUUID().String()
	exec(`INSERT INTO secret_backend_reference (secret_backend_uuid, model_uuid, secret_revision_uuid, secret_id)
	      VALUES (?, ?, ?, ?)`, backendUUID, modelUUID, revUUID, secretID)

	// Last login.
	loginTime := start.Add(-time.Hour)
	exec(`INSERT INTO model_last_login (model_uuid, user_uuid, time) VALUES (?, ?, ?)`,
		modelUUID, userUUID, loginTime)

	// Custom cloud image metadata is controller-global, but must be carried so
	// the target controller can recreate user-defined image selection data.
	rootStorageSize := uint64(8192)
	createdAt := start.Add(-2 * time.Hour)
	exec(`INSERT INTO cloud_image_metadata
	      (uuid, created_at, source, stream, region, version, architecture_id,
	       virt_type, root_storage_type, root_storage_size, priority, image_id)
	      VALUES (?, ?, ?, 'released', 'us-east-1', '22.04', 0, 'hvm', 'ebs',
	              ?, 10, 'ami-custom')`,
		uuid.MustNewUUID().String(), createdAt,
		cloudimagemetadata.CustomSource, rootStorageSize)
	exec(`INSERT INTO cloud_image_metadata
	      (uuid, created_at, source, stream, region, version, architecture_id,
	       virt_type, root_storage_type, priority, image_id)
	      VALUES (?, ?, 'simplestreams', 'released', 'us-east-1', '22.04', 0,
	              'hvm', 'ebs', 20, 'ami-cached')`,
		uuid.MustNewUUID().String(), createdAt)

	// Third-party external controller with an address and a consumed model.
	extCtrlUUID := uuid.MustNewUUID().String()
	consumedModelUUID := uuid.MustNewUUID().String()
	exec(`INSERT INTO external_controller (uuid, alias, ca_cert) VALUES (?, 'other-ctrl', 'CACERT')`, extCtrlUUID)
	exec(`INSERT INTO external_controller_address (uuid, controller_uuid, address) VALUES (?, ?, '1.2.3.4:17070')`,
		uuid.MustNewUUID().String(), extCtrlUUID)
	exec(`INSERT INTO external_model (uuid, controller_uuid) VALUES (?, ?)`,
		consumedModelUUID, extCtrlUUID)

	offererModels := []modelmigrationinternal.OffererModel{
		{ControllerUUID: extCtrlUUID, ModelUUID: consumedModelUUID},
	}

	info, err := st.GetControllerModelInfo(c.Context(), modelUUID, []string{offerUUID}, offererModels)
	c.Assert(err, tc.ErrorIsNil)

	// Offer permission present alongside the model admin permission.
	var foundOffer bool
	for _, p := range info.Permissions {
		if p.ObjectType == "offer" && p.GrantOn == offerUUID &&
			p.SubjectName == "test-user" && p.Access == "consume" {
			foundOffer = true
		}
	}
	c.Check(foundOffer, tc.IsTrue, tc.Commentf("expected offer permission, got %#v", info.Permissions))

	c.Check(info.AuthorizedKeys, tc.DeepEquals, []modelmigration.ModelAuthorizedKey{
		{Username: "test-user", PublicKey: "ssh-ed25519 AAAAkey"},
	})

	c.Check(info.Leaders, tc.DeepEquals, []modelmigration.ApplicationLeadership{
		{Application: "app", Leader: "app/0"},
	})

	c.Check(info.SecretBackendRefs, tc.DeepEquals, []modelmigration.SecretBackendReference{
		{BackendName: backendName, SecretRevisionUUID: revUUID, SecretID: secretID},
	})

	c.Assert(info.ModelCredential, tc.NotNil)
	c.Check(info.ModelCredential.Invalid, tc.IsTrue)
	c.Check(info.ModelCredential.InvalidReason, tc.Equals, "expired")

	var lastLogin *time.Time
	for _, u := range info.Users {
		if u.Name == "test-user" {
			lastLogin = u.LastLogin
			c.Check(u.Removed, tc.IsFalse)
			c.Check(u.External, tc.IsFalse)
		}
	}
	c.Assert(lastLogin, tc.NotNil)
	c.Check(lastLogin.Equal(loginTime), tc.IsTrue, tc.Commentf(
		"expected last login %v, got %v", loginTime, lastLogin,
	))

	c.Assert(info.CloudImageMetadata, tc.HasLen, 1)
	c.Check(info.CloudImageMetadata[0], tc.DeepEquals, modelmigration.CloudImageMetadata{
		Stream:          "released",
		Region:          "us-east-1",
		Version:         "22.04",
		Arch:            "amd64",
		VirtType:        "hvm",
		RootStorageType: "ebs",
		RootStorageSize: &rootStorageSize,
		Source:          cloudimagemetadata.CustomSource,
		Priority:        10,
		ImageID:         "ami-custom",
		CreatedAt:       createdAt,
	})

	c.Assert(info.ExternalControllers, tc.HasLen, 1)
	ec := info.ExternalControllers[0]
	c.Check(ec.UUID, tc.Equals, extCtrlUUID)
	c.Check(ec.Alias, tc.Equals, "other-ctrl")
	c.Check(ec.CACert, tc.Equals, "CACERT")
	c.Check(ec.Addresses, tc.DeepEquals, []string{"1.2.3.4:17070"})
	c.Check(ec.ConsumedModels, tc.DeepEquals, []string{consumedModelUUID})
}

// TestGetControllerModelInfoExternalModelMissing verifies model DB offerer
// selectors must be backed by source controller DB external_model rows.
func (s *stateSuite) TestGetControllerModelInfoExternalModelMissing(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	db := s.DB()

	extCtrlUUID := uuid.MustNewUUID().String()
	consumedModelUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		`INSERT INTO external_controller (uuid, alias, ca_cert) VALUES (?, 'other-ctrl', 'CACERT')`,
		extCtrlUUID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.GetControllerModelInfo(
		c.Context(),
		s.modelUUID.String(),
		nil,
		[]modelmigrationinternal.OffererModel{{
			ControllerUUID: extCtrlUUID,
			ModelUUID:      consumedModelUUID,
		}},
	)
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf(
		`external model %q for controller %q not found`, consumedModelUUID, extCtrlUUID,
	))
}

// TestGetControllerModelInfoExternalControllerMissing verifies model DB offerer
// selectors must be backed by source controller DB external_controller rows.
func (s *stateSuite) TestGetControllerModelInfoExternalControllerMissing(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	extCtrlUUID := uuid.MustNewUUID().String()
	consumedModelUUID := uuid.MustNewUUID().String()
	_, err := st.GetControllerModelInfo(
		c.Context(),
		s.modelUUID.String(),
		nil,
		[]modelmigrationinternal.OffererModel{{
			ControllerUUID: extCtrlUUID,
			ModelUUID:      consumedModelUUID,
		}},
	)
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf(
		`external controller %q for offerer model %q not found`, extCtrlUUID, consumedModelUUID,
	))
}

// TestGetControllerModelInfoModelNotFound verifies a clear error for an unknown
// model UUID.
func (s *stateSuite) TestGetControllerModelInfoModelNotFound(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	_, err := st.GetControllerModelInfo(c.Context(), uuid.MustNewUUID().String(), nil, nil)
	c.Assert(err, tc.ErrorMatches, `.*not found.*`)
}

// createControllerModel creates a the database for use in tests.
func (s *stateSuite) createControllerModel(c *tc.C, controllerModelUUID coremodel.UUID, userUUID user.UUID) uuid.UUID {
	// Before we can create the model, we need to create a controller model.
	// This ensures that we
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		err := controller.Create(c.Context(), preparer{}, tx, controllerModelUUID, coremodel.IAAS, model.GlobalModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
				Name:  "foobar",
			},
			Name:          "controller",
			Qualifier:     "prod",
			AdminUsers:    []user.UUID{userUUID},
			SecretBackend: juju.BackendName,
		})
		if err != nil {
			return err
		}

		activator := controller.GetActivator()
		return activator(ctx, preparer{}, tx, controllerModelUUID)
	})
	c.Assert(err, tc.ErrorIsNil)

	u, err := uuid.UUIDFromString(s.SeedControllerTable(c, controllerModelUUID))
	c.Assert(err, tc.ErrorIsNil)
	return u
}

// createModel creates a model in the database for use in tests.
func (s *stateSuite) createModel(c *tc.C, modelUUID coremodel.UUID, userUUID user.UUID) {
	s.createModelWithoutActivation(c, "my-test-model", modelUUID, userUUID)

	err := s.modelState.Activate(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) createModelWithoutActivation(
	c *tc.C, name string, modelUUID coremodel.UUID, creatorUUID user.UUID,
) {
	err := s.modelState.Create(
		c.Context(),
		modelUUID,
		coremodel.IAAS,
		model.GlobalModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
				Name:  "foobar",
			},
			Name:          name,
			Qualifier:     "prod",
			AdminUsers:    []user.UUID{creatorUUID},
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, tc.ErrorIsNil)
}

type preparer struct{}

func (p preparer) Prepare(query string, args ...any) (*sqlair.Statement, error) {
	return sqlair.Prepare(query, args...)
}
