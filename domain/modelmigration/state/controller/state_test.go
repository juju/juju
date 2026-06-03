// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"database/sql"
	"testing"

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
	"github.com/juju/juju/domain/credential"
	credentialstate "github.com/juju/juju/domain/credential/state"
	"github.com/juju/juju/domain/model"
	"github.com/juju/juju/domain/model/state/controller"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
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
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid) VALUES (?, ?, 'source-migration-uuid')",
		migratingUUID, s.modelUUID)
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
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid) VALUES (?, ?, 'source-migration-uuid')",
		migratingUUID, s.modelUUID)
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
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid) VALUES (?, ?, 'source-migration-uuid')",
		migratingUUID, s.modelUUID)
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
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid) VALUES (?, ?, 'source-migration-uuid')",
		migratingUUID, s.modelUUID)
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
func (s *stateSuite) newMigrationSpec() modelmigration.MigrationSpec {
	return modelmigration.MigrationSpec{
		MigrationUUID: uuid.MustNewUUID().String(),
		ModelUUID:     s.modelUUID.String(),
		Target: migration.TargetInfo{
			ControllerUUID:  uuid.MustNewUUID().String(),
			ControllerAlias: "target-controller",
			Addrs:           []string{"10.0.0.1:17070", "10.0.0.2:17070"},
			CACert:          "ca-cert-data",
			User:            "admin",
			Token:           "super-token",
			SkipUserChecks:  false,
		},
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

	// Export row exists, in QUIESCE (phase id 1), not ended.
	var (
		modelUUID  string
		targetUUID string
		phaseID    int
		endTime    sql.NullString
	)
	err = db.QueryRowContext(c.Context(),
		"SELECT model_uuid, target_controller_uuid, current_phase_id, end_time FROM model_migration_export WHERE uuid = ?",
		spec.MigrationUUID).Scan(&modelUUID, &targetUUID, &phaseID, &endTime)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelUUID, tc.Equals, s.modelUUID.String())
	c.Check(targetUUID, tc.Equals, spec.Target.ControllerUUID)
	c.Check(phaseID, tc.Equals, 1)
	c.Check(endTime.Valid, tc.IsFalse)

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
		"SELECT ca_cert FROM external_controller WHERE uuid = ?", spec.Target.ControllerUUID).Scan(&caCert)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(caCert, tc.Equals, "ca-cert-data")

	var addrCount int
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM external_controller_address WHERE controller_uuid = ?", spec.Target.ControllerUUID).Scan(&addrCount)
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

// TestEnsureExternalControllerInsert asserts the helper inserts a controller and
// its addresses when none exists.
func (s *stateSuite) TestEnsureExternalControllerInsert(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	info := modelmigration.ExternalControllerInfo{
		UUID:      uuid.MustNewUUID().String(),
		Alias:     "ctrl",
		CACert:    "ca",
		Addresses: []string{"1.2.3.4:17070"},
	}
	err := st.EnsureExternalControllerMatchesOrInsert(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)

	var count int
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM external_controller WHERE uuid = ?", info.UUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)
}

// TestEnsureExternalControllerMatchNoOp asserts re-inserting an identical
// controller is a no-op rather than an error.
func (s *stateSuite) TestEnsureExternalControllerMatchNoOp(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	info := modelmigration.ExternalControllerInfo{
		UUID:      uuid.MustNewUUID().String(),
		Alias:     "ctrl",
		CACert:    "ca",
		Addresses: []string{"1.2.3.4:17070", "5.6.7.8:17070"},
	}
	err := st.EnsureExternalControllerMatchesOrInsert(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)

	// Same details, different address order: still a match.
	info.Addresses = []string{"5.6.7.8:17070", "1.2.3.4:17070"}
	err = st.EnsureExternalControllerMatchesOrInsert(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)
}

// TestEnsureExternalControllerCACertConflict asserts a UUID collision with a
// different CA certificate is rejected.
func (s *stateSuite) TestEnsureExternalControllerCACertConflict(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	info := modelmigration.ExternalControllerInfo{
		UUID:      uuid.MustNewUUID().String(),
		CACert:    "ca",
		Addresses: []string{"1.2.3.4:17070"},
	}
	err := st.EnsureExternalControllerMatchesOrInsert(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)

	info.CACert = "different-ca"
	err = st.EnsureExternalControllerMatchesOrInsert(c.Context(), info)
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrExternalControllerConflict)
}

// TestEnsureExternalControllerAddressConflict asserts a UUID collision with
// different addresses is rejected.
func (s *stateSuite) TestEnsureExternalControllerAddressConflict(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	info := modelmigration.ExternalControllerInfo{
		UUID:      uuid.MustNewUUID().String(),
		CACert:    "ca",
		Addresses: []string{"1.2.3.4:17070"},
	}
	err := st.EnsureExternalControllerMatchesOrInsert(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)

	info.Addresses = []string{"9.9.9.9:17070"}
	err = st.EnsureExternalControllerMatchesOrInsert(c.Context(), info)
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrExternalControllerConflict)
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
	c.Check(mig.Target.ControllerUUID, tc.Equals, spec.Target.ControllerUUID)
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
	var endTime sql.NullString
	err = db.QueryRowContext(c.Context(),
		"SELECT end_time FROM model_migration_export WHERE uuid = ?", spec.MigrationUUID).Scan(&endTime)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(endTime.Valid, tc.IsTrue)

	// No active export remains.
	_, err = st.GetActiveExport(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrMigrationNotFound)
}

// TestSetStatusMessage asserts a status message is appended to the history.
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
	c.Check(count, tc.Equals, 2)
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

	var (
		phaseID int
		endTime sql.NullString
	)
	err = db.QueryRowContext(c.Context(),
		"SELECT current_phase_id, end_time FROM model_migration_export WHERE uuid = ?", spec.MigrationUUID).Scan(&phaseID, &endTime)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(phaseID, tc.Equals, 10)
	c.Check(endTime.Valid, tc.IsTrue)

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
