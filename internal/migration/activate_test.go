// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"context"
	"database/sql"

	"github.com/juju/clock"
	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/crossmodelrelation"
	crossmodelrelationservice "github.com/juju/juju/domain/crossmodelrelation/service"
	cmrmodelstate "github.com/juju/juju/domain/crossmodelrelation/state/model"
	"github.com/juju/juju/domain/export"
	modelservice "github.com/juju/juju/domain/model/service"
	modelstatecontroller "github.com/juju/juju/domain/model/state/controller"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	modelmigrationservice "github.com/juju/juju/domain/modelmigration/service"
	migrationclaimstate "github.com/juju/juju/domain/modelmigration/state/controller"
	migrationmodelstate "github.com/juju/juju/domain/modelmigration/state/model"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/uuid"
)

// importForActivation runs a v8 controller-data import for a fresh model and
// seeds the model DB with the target agent version and the import gate that
// activation clears, returning the model UUID and its deps.
func (s *controllerImportSuite) importForActivation(
	c *tc.C, modelAgentVersion string,
) (coremodel.UUID, migration.Deps) {
	modelUUID := tc.Must(c, coremodel.NewUUID)
	deps, _, _ := s.deps(c, modelUUID)

	info := s.baseControllerModelInfo(modelUUID)
	view := export.ProjectionView{AgentTargetVersion: jujuversion.Current}
	err := migration.ImportControllerModelInfo(
		c.Context(), deps, uuid.MustNewUUID().String(), info, view)
	c.Assert(err, tc.ErrorIsNil)

	// The model-DB content import (agent version, import gate) is a separate
	// task, so seed the minimum activation needs directly.
	runner := s.ModelTxnRunner(c, modelUUID.String())
	err = runner.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		// The model schema seeds a singleton agent_version row, so update it
		// rather than insert; fall back to insert if it is somehow absent.
		res, err := tx.ExecContext(ctx,
			"UPDATE agent_version SET target_version = ?, latest_version = ?",
			modelAgentVersion, modelAgentVersion)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO agent_version (stream_id, target_version, latest_version) VALUES (0, ?, ?)",
				modelAgentVersion, modelAgentVersion); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx,
			"INSERT OR IGNORE INTO model_migrating (uuid, model_uuid) VALUES (?, ?)",
			uuid.MustNewUUID().String(), modelUUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return modelUUID, deps
}

type activationDomainServicesGetter struct {
	deps migration.Deps
}

func (g activationDomainServicesGetter) ServicesForModel(
	_ context.Context, modelUUID coremodel.UUID,
) (services.DomainServices, error) {
	return activationDomainServices{
		modelMigration: modelmigrationservice.NewService(
			migrationclaimstate.New(g.deps.ControllerDB, g.deps.Clock),
			migrationmodelstate.New(g.deps.ModelDB, modelUUID),
			modelUUID.String(),
			nil,
			nil,
			nil,
			g.deps.Logger,
		),
		model: modelservice.NewWatchableService(
			modelstatecontroller.NewState(g.deps.ControllerDB),
			nil,
			nil,
			g.deps.Clock,
			g.deps.Logger,
		),
		cmr: crossmodelrelationservice.NewWatchableService(
			nil,
			cmrmodelstate.NewState(g.deps.ModelDB, modelUUID, g.deps.Clock, g.deps.Logger),
			nil,
			nil,
			g.deps.Clock,
			g.deps.Logger,
		),
	}, nil
}

type activationDomainServices struct {
	services.DomainServices

	modelMigration *modelmigrationservice.Service
	model          *modelservice.WatchableService
	cmr            *crossmodelrelationservice.WatchableService
}

func (s activationDomainServices) ModelMigration() *modelmigrationservice.Service {
	return s.modelMigration
}

func (s activationDomainServices) Model() *modelservice.WatchableService {
	return s.model
}

func (s activationDomainServices) CrossModelRelation() *crossmodelrelationservice.WatchableService {
	return s.cmr
}

func (*controllerImportSuite) activateModel(
	c *tc.C, deps migration.Deps, args migration.ActivateModelArgs,
) error {
	importer := migration.NewModelImporter(
		func(coremodel.UUID) coremodelmigration.Scope {
			return coremodelmigration.NewScope(
				deps.ControllerDB, deps.ModelDB, nil, nil, args.ModelUUID,
			)
		},
		activationDomainServicesGetter{deps: deps},
		"",
		deps.Logger,
		deps.Clock,
	)
	return importer.ActivateModel(c.Context(), args)
}

func (s *controllerImportSuite) modelActivated(c *tc.C, modelUUID coremodel.UUID) bool {
	var activated bool
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT activated FROM model WHERE uuid = ?", modelUUID.String()).Scan(&activated)
	})
	c.Assert(err, tc.ErrorIsNil)
	return activated
}

func (s *controllerImportSuite) modelGateExists(c *tc.C, modelUUID coremodel.UUID) bool {
	var count int
	runner := s.ModelTxnRunner(c, modelUUID.String())
	err := runner.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM model_migrating WHERE model_uuid = ?", modelUUID.String()).Scan(&count)
	})
	c.Assert(err, tc.ErrorIsNil)
	return count > 0
}

func (s *controllerImportSuite) modelAgentVersion(c *tc.C, modelUUID coremodel.UUID) string {
	var v string
	runner := s.ModelTxnRunner(c, modelUUID.String())
	err := runner.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT target_version FROM agent_version").Scan(&v)
	})
	c.Assert(err, tc.ErrorIsNil)
	return v
}

func (s *controllerImportSuite) importClaimUUID(c *tc.C, modelUUID coremodel.UUID) string {
	var claimUUID string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT uuid FROM model_migration_import WHERE model_uuid = ?",
			modelUUID.String()).Scan(&claimUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	return claimUUID
}

func (s *controllerImportSuite) addActivationOffererForModel(
	c *tc.C, deps migration.Deps, modelUUID coremodel.UUID, appName, offererModelUUID string,
) {
	st := cmrmodelstate.NewState(deps.ModelDB, modelUUID, deps.Clock, deps.Logger)
	err := st.AddRemoteApplicationOfferer(c.Context(), appName, crossmodelrelation.AddRemoteApplicationOffererArgs{
		ApplicationUUID:       uuid.MustNewUUID().String(),
		CharmUUID:             uuid.MustNewUUID().String(),
		RemoteApplicationUUID: uuid.MustNewUUID().String(),
		OfferUUID:             uuid.MustNewUUID().String(),
		OffererModelUUID:      offererModelUUID,
		Charm: charm.Charm{
			ReferenceName: appName,
			Source:        charm.CMRSource,
			Metadata: charm.Metadata{
				Name:        appName,
				Description: "remote offerer application",
				Provides:    map[string]charm.Relation{},
				Requires:    map[string]charm.Relation{},
				Peers:       map[string]charm.Relation{},
			},
		},
		EncodedMacaroon: []byte("m"),
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *controllerImportSuite) activationOffererControllerUUID(
	c *tc.C, modelUUID coremodel.UUID, offererModelUUID string,
) sql.NullString {
	var got sql.NullString
	runner := s.ModelTxnRunner(c, modelUUID.String())
	err := runner.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT offerer_controller_uuid FROM application_remote_offerer WHERE offerer_model_uuid = ?",
			offererModelUUID).Scan(&got)
	})
	c.Assert(err, tc.ErrorIsNil)
	return got
}

// TestActivateModelHappyPath verifies the v8 activation state machine: the
// claim is deleted, the import gate is cleared, the model row is activated and
// the model agent version is aligned with the controller target.
func (s *controllerImportSuite) TestActivateModelHappyPath(c *tc.C) {
	modelUUID, deps := s.importForActivation(c, "1.0.0")

	err := s.activateModel(c, deps, migration.ActivateModelArgs{
		ModelUUID: modelUUID,
	})
	c.Assert(err, tc.ErrorIsNil)

	// The claim is gone.
	claimSt := migrationclaimstate.New(s.TxnRunnerFactory(), clock.WallClock)
	_, err = claimSt.GetImportClaim(c.Context(), modelUUID.String())
	c.Check(err, tc.ErrorIs, modelmigrationerrors.ErrImportNotFound)

	// The model row is activated and the gate cleared.
	c.Check(s.modelActivated(c, modelUUID), tc.IsTrue)
	c.Check(s.modelGateExists(c, modelUUID), tc.IsFalse)

	// The agent version was aligned with the controller target.
	c.Check(s.modelAgentVersion(c, modelUUID), tc.Equals, jujuversion.Current.String())
}

// TestActivateModelIdempotent verifies a second activation after a completed
// one is a no-op success: the model stays activated and no claim reappears.
func (s *controllerImportSuite) TestActivateModelIdempotent(c *tc.C) {
	modelUUID, deps := s.importForActivation(c, "1.0.0")

	err := s.activateModel(c, deps, migration.ActivateModelArgs{ModelUUID: modelUUID})
	c.Assert(err, tc.ErrorIsNil)

	err = s.activateModel(c, deps, migration.ActivateModelArgs{ModelUUID: modelUUID})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.modelActivated(c, modelUUID), tc.IsTrue)
	claimSt := migrationclaimstate.New(s.TxnRunnerFactory(), clock.WallClock)
	_, err = claimSt.GetImportClaim(c.Context(), modelUUID.String())
	c.Check(err, tc.ErrorIs, modelmigrationerrors.ErrImportNotFound)
}

// TestActivateModelRetryFromActivating verifies that a crash after the claim
// reached the activating phase resumes to completion on a re-run.
func (s *controllerImportSuite) TestActivateModelRetryFromActivating(c *tc.C) {
	modelUUID, deps := s.importForActivation(c, "1.0.0")

	claimSt := migrationclaimstate.New(s.TxnRunnerFactory(), clock.WallClock)
	err := claimSt.SetImportPhaseActivating(c.Context(), modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	err = s.activateModel(c, deps, migration.ActivateModelArgs{ModelUUID: modelUUID})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.modelActivated(c, modelUUID), tc.IsTrue)
	_, err = claimSt.GetImportClaim(c.Context(), modelUUID.String())
	c.Check(err, tc.ErrorIs, modelmigrationerrors.ErrImportNotFound)
}

// TestActivateModelUnexpectedImportPhase verifies the defensive switch guard:
// if a future phase reaches activation before the driver knows how to handle
// it, activation stops instead of falling through.
func (s *controllerImportSuite) TestActivateModelUnexpectedImportPhase(c *tc.C) {
	modelUUID, deps := s.importForActivation(c, "1.0.0")

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO model_migration_import_phase_type (id, type) VALUES (?, ?)",
			99, "paused")
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx,
			"UPDATE model_migration_import SET phase_type_id = ? WHERE model_uuid = ?",
			99, modelUUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.activateModel(c, deps, migration.ActivateModelArgs{ModelUUID: modelUUID})
	c.Assert(err, tc.ErrorMatches, `model ".+": unexpected import claim phase "paused"`)
	c.Check(s.modelActivated(c, modelUUID), tc.IsFalse)
	c.Check(s.modelGateExists(c, modelUUID), tc.IsTrue)
}

// TestActivateModelReconcileFailureLeavesClaimImporting verifies the core
// wedge-fix property: a genuine (permanent) reconciliation failure now happens
// before the point-of-no-return flip, so the claim stays in the importing phase
// and the source can safely abort — instead of wedging in activating.
func (s *controllerImportSuite) TestActivateModelReconcileFailureLeavesClaimImporting(c *tc.C) {
	modelUUID, deps := s.importForActivation(c, "1.0.0")

	sourceControllerUUID := uuid.MustNewUUID().String()
	sourceOffererModelUUID := uuid.MustNewUUID().String()
	s.addActivationOffererForModel(c, deps, modelUUID, "source-offerer", sourceOffererModelUUID)

	// Pre-seed the source controller with a different CA cert so activation's
	// reconcileOffererControllers (EnsureSourceControllerExists) fails with a
	// genuine external-controller mismatch.
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO external_controller (uuid, alias, ca_cert) VALUES (?, ?, ?)",
			sourceControllerUUID, "source", "a-different-ca-cert")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.activateModel(c, deps, migration.ActivateModelArgs{
		ModelUUID:             modelUUID,
		SourceControllerUUID:  sourceControllerUUID,
		SourceControllerAlias: "source",
		SourceCACert:          "source-ca-cert", // differs from the seeded cert
		SourceAPIAddrs:        []string{"10.0.0.1:17070"},
		CrossModelUUIDs:       []string{sourceOffererModelUUID},
	})
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrExternalControllerMismatch)

	// The claim is still importing (abortable), and the model is neither
	// activated nor un-gated.
	claimSt := migrationclaimstate.New(s.TxnRunnerFactory(), clock.WallClock)
	claim, err := claimSt.GetImportClaim(c.Context(), modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(claim.Phase, tc.Equals, modelmigration.ImportPhaseImporting)
	c.Check(s.modelActivated(c, modelUUID), tc.IsFalse)
	c.Check(s.modelGateExists(c, modelUUID), tc.IsTrue)
}

// TestActivateModelActivatingSkipsReconcile verifies that resuming an
// already-activating claim runs only the idempotent finalization and skips the
// fallible reconciliation: even with a seeded external-controller mismatch that
// would fail reconciliation, activation completes.
func (s *controllerImportSuite) TestActivateModelActivatingSkipsReconcile(c *tc.C) {
	modelUUID, deps := s.importForActivation(c, "1.0.0")

	sourceControllerUUID := uuid.MustNewUUID().String()
	sourceOffererModelUUID := uuid.MustNewUUID().String()
	s.addActivationOffererForModel(c, deps, modelUUID, "source-offerer", sourceOffererModelUUID)

	// A mismatch that reconciliation would trip over if it ran.
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO external_controller (uuid, alias, ca_cert) VALUES (?, ?, ?)",
			sourceControllerUUID, "source", "a-different-ca-cert")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	// Move the claim past the point of no return.
	claimSt := migrationclaimstate.New(s.TxnRunnerFactory(), clock.WallClock)
	c.Assert(claimSt.SetImportPhaseActivating(c.Context(), modelUUID.String()), tc.ErrorIsNil)

	err = s.activateModel(c, deps, migration.ActivateModelArgs{
		ModelUUID:             modelUUID,
		SourceControllerUUID:  sourceControllerUUID,
		SourceControllerAlias: "source",
		SourceCACert:          "source-ca-cert",
		SourceAPIAddrs:        []string{"10.0.0.1:17070"},
		CrossModelUUIDs:       []string{sourceOffererModelUUID},
	})
	// Reconciliation was skipped, so the mismatch is never hit and activation
	// completes.
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.modelActivated(c, modelUUID), tc.IsTrue)
	_, err = claimSt.GetImportClaim(c.Context(), modelUUID.String())
	c.Check(err, tc.ErrorIs, modelmigrationerrors.ErrImportNotFound)
}

// TestActivateModelReconcilesOffererControllers verifies activation's CMR
// reconciliation branch end to end for both source-hosted and third-party
// offerer models.
func (s *controllerImportSuite) TestActivateModelReconcilesOffererControllers(c *tc.C) {
	modelUUID, deps := s.importForActivation(c, "1.0.0")

	sourceControllerUUID := uuid.MustNewUUID().String()
	sourceOffererModelUUID := uuid.MustNewUUID().String()
	thirdPartyControllerUUID := uuid.MustNewUUID().String()
	thirdPartyOffererModelUUID := uuid.MustNewUUID().String()

	s.addActivationOffererForModel(
		c, deps, modelUUID, "source-offerer", sourceOffererModelUUID)
	s.addActivationOffererForModel(
		c, deps, modelUUID, "third-party-offerer", thirdPartyOffererModelUUID)

	claimSt := migrationclaimstate.New(s.TxnRunnerFactory(), clock.WallClock)
	claimUUID := s.importClaimUUID(c, modelUUID)
	err := modelmigrationservice.NewImportService(claimSt, deps.Logger).ImportExternalControllers(
		c.Context(), modelUUID, claimUUID,
		[]coremodelmigration.ExternalController{{
			UUID:           thirdPartyControllerUUID,
			Alias:          "third-party",
			CACert:         "third-party-ca-cert",
			Addresses:      []string{"10.0.0.5:17070"},
			ConsumedModels: []string{thirdPartyOffererModelUUID},
		}},
	)
	c.Assert(err, tc.ErrorIsNil)

	err = s.activateModel(c, deps, migration.ActivateModelArgs{
		ModelUUID:             modelUUID,
		SourceControllerUUID:  sourceControllerUUID,
		SourceControllerAlias: "source",
		SourceCACert:          "source-ca-cert",
		SourceAPIAddrs:        []string{"10.0.0.1:17070"},
		CrossModelUUIDs:       []string{sourceOffererModelUUID},
	})
	c.Assert(err, tc.ErrorIsNil)

	got := s.activationOffererControllerUUID(c, modelUUID, sourceOffererModelUUID)
	c.Assert(got.Valid, tc.IsTrue)
	c.Check(got.String, tc.Equals, sourceControllerUUID)

	got = s.activationOffererControllerUUID(c, modelUUID, thirdPartyOffererModelUUID)
	c.Assert(got.Valid, tc.IsTrue)
	c.Check(got.String, tc.Equals, thirdPartyControllerUUID)
}

// TestActivateModelAborting verifies activation refuses to proceed when the
// claim has already moved to the aborting phase, leaving the model unactivated.
func (s *controllerImportSuite) TestActivateModelAborting(c *tc.C) {
	modelUUID, deps := s.importForActivation(c, "1.0.0")

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"UPDATE model_migration_import SET phase_type_id = 2 WHERE model_uuid = ?", modelUUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.activateModel(c, deps, migration.ActivateModelArgs{ModelUUID: modelUUID})
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrActivationAborting)

	c.Check(s.modelActivated(c, modelUUID), tc.IsFalse)
}

// TestActivateModelLegacyNoClaim verifies a legacy import (import gate set, no
// v8 claim) still activates: the gate is cleared and the model row activated.
func (s *controllerImportSuite) TestActivateModelLegacyNoClaim(c *tc.C) {
	modelUUID, deps := s.importForActivation(c, "1.0.0")

	// Simulate a legacy import by dropping the v8 claim entirely.
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"DELETE FROM model_migration_import WHERE model_uuid = ?", modelUUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.activateModel(c, deps, migration.ActivateModelArgs{ModelUUID: modelUUID})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.modelActivated(c, modelUUID), tc.IsTrue)
	c.Check(s.modelGateExists(c, modelUUID), tc.IsFalse)
	c.Check(s.modelAgentVersion(c, modelUUID), tc.Equals, jujuversion.Current.String())
}
