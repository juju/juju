// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"

	coredatabase "github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/export"
	"github.com/juju/juju/domain/export/types/latest"
	v4_1_0 "github.com/juju/juju/domain/export/types/v4_1_0"
	modeltesting "github.com/juju/juju/domain/model/state/testing"
	migrationclaimstate "github.com/juju/juju/domain/modelmigration/state/controller"
	domainservicestesting "github.com/juju/juju/domain/services/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/internal/uuid"
)

// modelImporterSuite is a thin smoke test for ModelImporter.ImportModel, the
// public method the migrationtarget facade calls. The orchestration itself is
// covered in this package's direct ImportControllerModelInfo tests; this only
// proves the delegator resolves the migration scope for the model UUID and
// wires it through correctly, and that it activates the imported model so it is
// connectable during the source VALIDATION phase.
//
// It embeds DomainServicesSuite (rather than a bare schema suite) because
// ImportModel now resolves real domain services for the model to flip
// model.activated at the end of the import.
type modelImporterSuite struct {
	domainservicestesting.DomainServicesSuite
}

func TestModelImporterSuite(t *testing.T) {
	tc.Run(t, &modelImporterSuite{})
}

func (s *modelImporterSuite) SetUpTest(c *tc.C) {
	// DomainServicesSuite seeds the controller row, admin user, and
	// cloud+credential (as s.CloudName), and provisions the model databases.
	s.DomainServicesSuite.SetUpTest(c)
	modeltesting.CreateInternalSecretBackend(c, s.ControllerTxnRunner())
}

func (s *modelImporterSuite) TestImportModel(c *tc.C) {
	modelUUID := tc.Must(c, coremodel.NewUUID)
	controllerFactory := s.TxnRunnerFactory()
	modelRunner := s.ModelTxnRunner(c, modelUUID.String())
	modelFactory := func(context.Context) (coredatabase.TxnRunner, error) {
		return modelRunner, nil
	}

	scope := func(coremodel.UUID) coremodelmigration.Scope {
		return coremodelmigration.NewScope(controllerFactory, modelFactory, nil, nil, modelUUID)
	}
	importer := migration.NewModelImporter(scope, s.ModelDomainServicesGetter(c), nil, "controller-uuid", loggertesting.WrapCheckLog(c), clock.WallClock)

	importArgs := migration.ImportModelArgs{
		SourceMigrationUUID: uuid.MustNewUUID().String(),
		ControllerModelInfo: coremodelmigration.ControllerModelInfo{
			ModelInfo: coremodelmigration.ModelIdentityInfo{
				UUID:      modelUUID.String(),
				Name:      "imported-model",
				Qualifier: "prod",
				Type:      "iaas",
				Cloud:     s.CloudName,
				Life:      "alive",
			},
		},
	}
	view := export.ProjectionView{AgentTargetVersion: jujuversion.Current}

	err := importer.ImportModel(c.Context(), importArgs, view)
	c.Assert(err, tc.ErrorIsNil)

	// The model is activated by the import so the migrating model's agents can
	// connect to it during VALIDATION. CheckModelExists reads v_model, which
	// only lists activated models, so a true result proves activation.
	exists, err := s.ControllerDomainServices(c).Model().CheckModelExists(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)

	// The import claim is still present and importing: activation of the model
	// row must not have deleted the claim (that happens only in the target
	// Activate call). It landed against the same controller DB the scope
	// resolved to.
	claimSt := migrationclaimstate.New(controllerFactory, clock.WallClock)
	claim, err := claimSt.GetImportClaim(c.Context(), modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(claim.SourceMigrationUUID, tc.Equals, importArgs.SourceMigrationUUID)

	// A second call against the same scope is rejected as a duplicate claim,
	// proving the delegator re-resolves the scope per call rather than
	// caching stale state.
	err = importer.ImportModel(c.Context(), importArgs, view)
	c.Check(err, tc.ErrorIs, coreerrors.AlreadyExists)
}

func (s *modelImporterSuite) TestImportModelNoSecretBackendRewriteRows(c *tc.C) {
	modelUUID := tc.Must(c, coremodel.NewUUID)
	controllerFactory := s.TxnRunnerFactory()
	modelRunner := s.ModelTxnRunner(c, modelUUID.String())
	modelFactory := func(context.Context) (coredatabase.TxnRunner, error) {
		return modelRunner, nil
	}

	passwordHash := "some-hash"
	payload := &latest.ModelExport{
		ModelAgent: []v4_1_0.ModelAgent{
			{ModelUUID: modelUUID.String(), PasswordHash: &passwordHash},
		},
		Sequence: []v4_1_0.Sequence{{Namespace: "machine", Value: 7}},
	}

	scope := func(coremodel.UUID) coremodelmigration.Scope {
		return coremodelmigration.NewScope(controllerFactory, modelFactory, nil, nil, modelUUID)
	}
	importer := migration.NewModelImporter(scope, s.ModelDomainServicesGetter(c), nil, "controller-uuid", loggertesting.WrapCheckLog(c), clock.WallClock)

	importArgs := migration.ImportModelArgs{
		SourceMigrationUUID: uuid.MustNewUUID().String(),
		ControllerModelInfo: coremodelmigration.ControllerModelInfo{
			ModelInfo: coremodelmigration.ModelIdentityInfo{
				UUID:      modelUUID.String(),
				Name:      "imported-model",
				Qualifier: "prod",
				Type:      "iaas",
				Cloud:     s.CloudName,
				Life:      "alive",
			},
		},
		ModelDBPayload: payload,
	}
	view := export.ProjectionView{AgentTargetVersion: jujuversion.Current}

	err := importer.ImportModel(c.Context(), importArgs, view)
	c.Assert(err, tc.ErrorIsNil)

	var sequenceValue int64
	err = modelRunner.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			`SELECT value FROM sequence WHERE namespace = ?`, "machine",
		).Scan(&sequenceValue)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sequenceValue, tc.Equals, int64(7))
}

// createExternalSecretBackend inserts a named external secret backend on the
// controller DB and returns its target UUID.
func (s *modelImporterSuite) createExternalSecretBackend(c *tc.C, name string) string {
	backendUUID := uuid.MustNewUUID().String()
	controllerRunner := s.ControllerTxnRunner()
	err := controllerRunner.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO secret_backend (uuid, name, backend_type_id)
			VALUES (?, ?, 1)
			ON CONFLICT (name) DO NOTHING
		`, backendUUID, name)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return backendUUID
}

// TestImportModelSecretBackendRewrite verifies that secret_value_ref
// backend_uuid fields are rewritten from the source controller's backend UUIDs
// to the target's before the model-DB insert. Deleted value refs do not have
// controller secret backend references, so they do not require a mapping.
func (s *modelImporterSuite) TestImportModelSecretBackendRewrite(c *tc.C) {
	modelUUID := tc.Must(c, coremodel.NewUUID)
	controllerFactory := s.TxnRunnerFactory()
	modelRunner := s.ModelTxnRunner(c, modelUUID.String())
	modelFactory := func(context.Context) (coredatabase.TxnRunner, error) {
		return modelRunner, nil
	}

	// Create a named external backend on the target with a fresh UUID.
	targetBackendName := "vault-external"
	targetBackendUUID := s.createExternalSecretBackend(c, targetBackendName)

	// Source backend UUID — different from the target's.
	sourceBackendUUID := uuid.MustNewUUID().String()

	now := time.Now().UTC()
	secretID := uuid.MustNewUUID().String()
	revUUID1 := uuid.MustNewUUID().String()
	revUUID2 := uuid.MustNewUUID().String()
	passwordHash := "some-hash"

	payload := &latest.ModelExport{
		ModelAgent: []v4_1_0.ModelAgent{
			{ModelUUID: modelUUID.String(), PasswordHash: &passwordHash},
		},
		Secret: []v4_1_0.Secret{
			{ID: secretID},
		},
		SecretMetadata: []v4_1_0.SecretMetadata{
			{SecretID: secretID, Version: 1, RotatePolicyID: 0, CreateTime: now, UpdateTime: now},
		},
		SecretRevision: []v4_1_0.SecretRevision{
			{UUID: revUUID1, SecretID: secretID, Revision: 1, CreateTime: now},
		},
		SecretValueRef: []v4_1_0.SecretValueRef{
			{RevisionUUID: revUUID1, BackendUUID: sourceBackendUUID, RevisionID: "ext-rev-1"},
		},
		SecretDeletedValueRef: []v4_1_0.SecretDeletedValueRef{
			{RevisionUUID: revUUID2, BackendUUID: sourceBackendUUID, RevisionID: "ext-rev-2"},
		},
	}

	scope := func(coremodel.UUID) coremodelmigration.Scope {
		return coremodelmigration.NewScope(controllerFactory, modelFactory, nil, nil, modelUUID)
	}
	importer := migration.NewModelImporter(scope, s.ModelDomainServicesGetter(c), nil, "controller-uuid", loggertesting.WrapCheckLog(c), clock.WallClock)

	importArgs := migration.ImportModelArgs{
		SourceMigrationUUID: uuid.MustNewUUID().String(),
		ControllerModelInfo: coremodelmigration.ControllerModelInfo{
			ModelInfo: coremodelmigration.ModelIdentityInfo{
				UUID:      modelUUID.String(),
				Name:      "imported-model",
				Qualifier: "prod",
				Type:      "iaas",
				Cloud:     s.CloudName,
				Life:      "alive",
			},
			SecretBackendRefs: []coremodelmigration.SecretBackendReference{
				{BackendName: targetBackendName, SecretRevisionUUID: revUUID1, SecretID: secretID},
			},
		},
		ModelDBPayload: payload,
	}
	view := export.ProjectionView{AgentTargetVersion: jujuversion.Current}

	err := importer.ImportModel(c.Context(), importArgs, view)
	c.Assert(err, tc.ErrorIsNil)

	// Verify secret_value_ref row carries the target backend UUID.
	var insertedBackendUUID string
	var insertedRevisionID string
	err = modelRunner.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			`SELECT backend_uuid, revision_id FROM secret_value_ref WHERE revision_uuid = ?`, revUUID1,
		).Scan(&insertedBackendUUID, &insertedRevisionID)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(insertedBackendUUID, tc.Equals, targetBackendUUID,
		tc.Commentf("secret_value_ref should carry target backend UUID, got %q", insertedBackendUUID))
	c.Check(insertedRevisionID, tc.Equals, "ext-rev-1")

	// Verify secret_deleted_value_ref row did not require a backend mapping.
	err = modelRunner.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			`SELECT backend_uuid, revision_id FROM secret_deleted_value_ref WHERE revision_uuid = ?`, revUUID2,
		).Scan(&insertedBackendUUID, &insertedRevisionID)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(insertedBackendUUID, tc.Equals, sourceBackendUUID)
	c.Check(insertedRevisionID, tc.Equals, "ext-rev-2")
}

// TestImportModelSecretBackendRewriteMissingRef verifies that a
// secret_value_ref revision absent from SecretBackendRefs causes
// ImportModel to error before committing any model-DB rows.
func (s *modelImporterSuite) TestImportModelSecretBackendRewriteMissingRef(c *tc.C) {
	modelUUID := tc.Must(c, coremodel.NewUUID)
	controllerFactory := s.TxnRunnerFactory()
	modelRunner := s.ModelTxnRunner(c, modelUUID.String())
	modelFactory := func(context.Context) (coredatabase.TxnRunner, error) {
		return modelRunner, nil
	}

	targetBackendName := "vault-external"
	s.createExternalSecretBackend(c, targetBackendName)

	sourceBackendUUID := uuid.MustNewUUID().String()
	now := time.Now().UTC()
	secretID := uuid.MustNewUUID().String()
	revUUID := uuid.MustNewUUID().String()
	passwordHash := "some-hash"

	payload := &latest.ModelExport{
		ModelAgent: []v4_1_0.ModelAgent{
			{ModelUUID: modelUUID.String(), PasswordHash: &passwordHash},
		},
		Secret: []v4_1_0.Secret{
			{ID: secretID},
		},
		SecretMetadata: []v4_1_0.SecretMetadata{
			{SecretID: secretID, Version: 1, RotatePolicyID: 0, CreateTime: now, UpdateTime: now},
		},
		SecretRevision: []v4_1_0.SecretRevision{
			{UUID: revUUID, SecretID: secretID, Revision: 1, CreateTime: now},
		},
		SecretValueRef: []v4_1_0.SecretValueRef{
			{RevisionUUID: revUUID, BackendUUID: sourceBackendUUID, RevisionID: "ext-rev-1"},
		},
	}

	scope := func(coremodel.UUID) coremodelmigration.Scope {
		return coremodelmigration.NewScope(controllerFactory, modelFactory, nil, nil, modelUUID)
	}
	importer := migration.NewModelImporter(scope, s.ModelDomainServicesGetter(c), nil, "controller-uuid", loggertesting.WrapCheckLog(c), clock.WallClock)

	importArgs := migration.ImportModelArgs{
		SourceMigrationUUID: uuid.MustNewUUID().String(),
		ControllerModelInfo: coremodelmigration.ControllerModelInfo{
			ModelInfo: coremodelmigration.ModelIdentityInfo{
				UUID:      modelUUID.String(),
				Name:      "imported-model",
				Qualifier: "prod",
				Type:      "iaas",
				Cloud:     s.CloudName,
				Life:      "alive",
			},
			// Deliberately omit SecretBackendRefs — the revision has no mapping.
		},
		ModelDBPayload: payload,
	}
	view := export.ProjectionView{AgentTargetVersion: jujuversion.Current}

	err := importer.ImportModel(c.Context(), importArgs, view)
	c.Assert(err, tc.ErrorMatches, `.*no target secret backend for secret revision.*`)

	// Verify no secret_value_ref rows were committed.
	var count int
	err = modelRunner.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM secret_value_ref WHERE revision_uuid = ?`, revUUID,
		).Scan(&count)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

func (s *modelImporterSuite) TestImportModelSecretBackendRewriteMissingBackend(c *tc.C) {
	modelUUID := tc.Must(c, coremodel.NewUUID)
	controllerFactory := s.TxnRunnerFactory()
	modelRunner := s.ModelTxnRunner(c, modelUUID.String())
	modelFactory := func(context.Context) (coredatabase.TxnRunner, error) {
		return modelRunner, nil
	}

	sourceBackendUUID := uuid.MustNewUUID().String()
	now := time.Now().UTC()
	secretID := uuid.MustNewUUID().String()
	revUUID := uuid.MustNewUUID().String()
	passwordHash := "some-hash"

	payload := &latest.ModelExport{
		ModelAgent: []v4_1_0.ModelAgent{
			{ModelUUID: modelUUID.String(), PasswordHash: &passwordHash},
		},
		Secret: []v4_1_0.Secret{
			{ID: secretID},
		},
		SecretMetadata: []v4_1_0.SecretMetadata{
			{SecretID: secretID, Version: 1, RotatePolicyID: 0, CreateTime: now, UpdateTime: now},
		},
		SecretRevision: []v4_1_0.SecretRevision{
			{UUID: revUUID, SecretID: secretID, Revision: 1, CreateTime: now},
		},
		SecretValueRef: []v4_1_0.SecretValueRef{
			{RevisionUUID: revUUID, BackendUUID: sourceBackendUUID, RevisionID: "ext-rev-1"},
		},
	}

	scope := func(coremodel.UUID) coremodelmigration.Scope {
		return coremodelmigration.NewScope(controllerFactory, modelFactory, nil, nil, modelUUID)
	}
	importer := migration.NewModelImporter(scope, s.ModelDomainServicesGetter(c), nil, "controller-uuid", loggertesting.WrapCheckLog(c), clock.WallClock)

	importArgs := migration.ImportModelArgs{
		SourceMigrationUUID: uuid.MustNewUUID().String(),
		ControllerModelInfo: coremodelmigration.ControllerModelInfo{
			ModelInfo: coremodelmigration.ModelIdentityInfo{
				UUID:      modelUUID.String(),
				Name:      "imported-model",
				Qualifier: "prod",
				Type:      "iaas",
				Cloud:     s.CloudName,
				Life:      "alive",
			},
			SecretBackendRefs: []coremodelmigration.SecretBackendReference{{
				BackendName:        "nonexistent",
				SecretRevisionUUID: revUUID,
				SecretID:           secretID,
			}},
		},
		ModelDBPayload: payload,
	}
	view := export.ProjectionView{AgentTargetVersion: jujuversion.Current}

	err := importer.ImportModel(c.Context(), importArgs, view)
	c.Assert(err, tc.ErrorMatches, `.*looking up secret backend "nonexistent".*`)

	var count int
	err = modelRunner.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM secret_value_ref WHERE revision_uuid = ?`, revUUID,
		).Scan(&count)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}
