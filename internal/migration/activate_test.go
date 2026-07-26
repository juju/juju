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
	migrationdomain "github.com/juju/juju/domain/modelmigration"
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
		modelMigration: modelmigrationservice.NewWatchableService(
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

	modelMigration *modelmigrationservice.WatchableService
	model          *modelservice.WatchableService
	cmr            *crossmodelrelationservice.WatchableService
}

func (s activationDomainServices) ModelMigration() *modelmigrationservice.WatchableService {
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

// TestActivateModelLeavesModelAbortable is the central guarantee of the
// protocol: preparing a model must not commit anything.
//
// The source calls this during VALIDATION, where any error - or a reply it never
// receives - sends it to ABORT. If preparation released the model, that abort
// would arrive at a target that had already handed the model over, and the model
// would be live on both controllers.
func (s *controllerImportSuite) TestActivateModelLeavesModelAbortable(c *tc.C) {
	modelUUID, deps := s.importForActivation(c, "1.0.0")

	err := s.activateModel(c, deps, migration.ActivateModelArgs{
		ModelUUID: modelUUID,
	})
	c.Assert(err, tc.ErrorIsNil)

	// The claim is untouched and still importing, so an abort still works.
	claimSt := migrationclaimstate.New(s.TxnRunnerFactory(), clock.WallClock)
	claim, err := claimSt.GetImportClaim(c.Context(), modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(claim.Phase, tc.Equals, migrationdomain.ImportPhaseImporting)

	// Nothing was released: the gate is still set and the model not activated.
	c.Check(s.modelGateExists(c, modelUUID), tc.IsTrue)
	c.Check(s.modelActivated(c, modelUUID), tc.IsFalse)

	// The fallible work still happened - that is what preparation is for.
	c.Check(s.modelAgentVersion(c, modelUUID), tc.Equals, jujuversion.Current.String())
}

// TestCommitActivationReleasesModel verifies the commit does what preparation
// deliberately did not: clear the gate, activate the model row and give up the
// claim. Claim deletion is what actually releases the model, so it must be gone.
func (s *controllerImportSuite) TestCommitActivationReleasesModel(c *tc.C) {
	modelUUID, deps := s.importForActivation(c, "1.0.0")

	c.Assert(s.activateModel(c, deps, migration.ActivateModelArgs{ModelUUID: modelUUID}), tc.ErrorIsNil)
	c.Assert(s.commitActivation(c, deps, modelUUID), tc.ErrorIsNil)

	claimSt := migrationclaimstate.New(s.TxnRunnerFactory(), clock.WallClock)
	_, err := claimSt.GetImportClaim(c.Context(), modelUUID.String())
	c.Check(err, tc.ErrorIs, modelmigrationerrors.ErrImportNotFound)

	c.Check(s.modelActivated(c, modelUUID), tc.IsTrue)
	c.Check(s.modelGateExists(c, modelUUID), tc.IsFalse)
}

// TestCommitActivationIsIdempotentAfterRelease verifies a commit retry arriving
// after the claim is gone succeeds. The source cannot tell a lost reply from a
// failure, so it will send this call again.
func (s *controllerImportSuite) TestCommitActivationIsIdempotentAfterRelease(c *tc.C) {
	modelUUID, deps := s.importForActivation(c, "1.0.0")

	c.Assert(s.activateModel(c, deps, migration.ActivateModelArgs{ModelUUID: modelUUID}), tc.ErrorIsNil)
	c.Assert(s.commitActivation(c, deps, modelUUID), tc.ErrorIsNil)
	c.Assert(s.commitActivation(c, deps, modelUUID), tc.ErrorIsNil)

	c.Check(s.modelActivated(c, modelUUID), tc.IsTrue)
}

// TestCommitActivationWithoutImportFails verifies a commit for a model that was
// never imported here is refused, rather than a missing claim being read as
// "already done".
func (s *controllerImportSuite) TestCommitActivationWithoutImportFails(c *tc.C) {
	modelUUID, deps := s.importForActivation(c, "1.0.0")

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"DELETE FROM model_migration_import WHERE model_uuid = ?", modelUUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	// The model is neither activated nor ungated, so the predicates fail.
	err = s.commitActivation(c, deps, modelUUID)
	c.Check(err, tc.ErrorMatches, ".*is not importing or activating.*")
}

// TestCommitActivationRefusesAbortingClaim verifies a commit is refused while
// the model is being torn down. A correct source cannot produce this - a
// migration that reached SUCCESS never drove an abort - so it fails loudly
// rather than resurrecting a model whose cleanup is under way.
func (s *controllerImportSuite) TestCommitActivationRefusesAbortingClaim(c *tc.C) {
	modelUUID, deps := s.importForActivation(c, "1.0.0")
	s.setClaimPhase(c, modelUUID, "aborting")

	err := s.commitActivation(c, deps, modelUUID)
	c.Check(err, tc.ErrorMatches, ".*cannot commit activation, import is aborting.*")
	c.Check(s.modelActivated(c, modelUUID), tc.IsFalse)
}

// TestActivateModelIdempotent verifies preparation can be repeated, which a
// source restarting in VALIDATION will do, and that repeating it still commits
// nothing.
func (s *controllerImportSuite) TestActivateModelIdempotent(c *tc.C) {
	modelUUID, deps := s.importForActivation(c, "1.0.0")

	err := s.activateModel(c, deps, migration.ActivateModelArgs{ModelUUID: modelUUID})
	c.Assert(err, tc.ErrorIsNil)

	err = s.activateModel(c, deps, migration.ActivateModelArgs{ModelUUID: modelUUID})
	c.Assert(err, tc.ErrorIsNil)

	claimSt := migrationclaimstate.New(s.TxnRunnerFactory(), clock.WallClock)
	claim, err := claimSt.GetImportClaim(c.Context(), modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(claim.Phase, tc.Equals, migrationdomain.ImportPhaseImporting)
}

// TestCommitActivationResumesFromActivating verifies a commit interrupted after
// its transition is finished by a retry. The source retries AdoptResources until
// it succeeds, so this is the ordinary recovery path.
func (s *controllerImportSuite) TestCommitActivationResumesFromActivating(c *tc.C) {
	modelUUID, deps := s.importForActivation(c, "1.0.0")

	claimSt := migrationclaimstate.New(s.TxnRunnerFactory(), clock.WallClock)
	err := claimSt.SetImportPhaseActivating(c.Context(), modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	err = s.commitActivation(c, deps, modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.modelActivated(c, modelUUID), tc.IsTrue)
	_, err = claimSt.GetImportClaim(c.Context(), modelUUID.String())
	c.Check(err, tc.ErrorIs, modelmigrationerrors.ErrImportNotFound)
}

// TestActivateModelAfterCommitSucceeds verifies a stale preparation arriving
// after the commit is a no-op success rather than an error. Failing it would
// push a stale source to ABORT, which the target must then refuse.
func (s *controllerImportSuite) TestActivateModelAfterCommitSucceeds(c *tc.C) {
	modelUUID, deps := s.importForActivation(c, "1.0.0")
	s.setClaimPhase(c, modelUUID, "activating")

	err := s.activateModel(c, deps, migration.ActivateModelArgs{ModelUUID: modelUUID})
	c.Check(err, tc.ErrorIsNil)
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

// TestActivateAndCommitWithLegacyClaimShape verifies a legacy (v4-v7) import
// goes through the same prepare/commit protocol as a v8 one.
//
// A previous version of this test simulated "legacy" by deleting the import
// claim. That is not what a legacy import looks like: the legacy path creates a
// claim in the importing phase, exactly like v8. Claim existence is not a
// legacy/v8 discriminator, and treating it as one silently put 3.6 and 4.0
// sources on the wrong path.
func (s *controllerImportSuite) TestActivateAndCommitWithLegacyClaimShape(c *tc.C) {
	modelUUID, deps := s.importForActivation(c, "1.0.0")

	// The claim a legacy import leaves behind carries no source migration
	// identity of its own - it reuses its own UUID - but is otherwise identical.
	claimSt := migrationclaimstate.New(s.TxnRunnerFactory(), clock.WallClock)
	claim, err := claimSt.GetImportClaim(c.Context(), modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(claim.Phase, tc.Equals, migrationdomain.ImportPhaseImporting)

	// Preparation leaves it abortable, as for any other import.
	c.Assert(s.activateModel(c, deps, migration.ActivateModelArgs{ModelUUID: modelUUID}), tc.ErrorIsNil)
	c.Check(s.modelGateExists(c, modelUUID), tc.IsTrue)
	c.Check(s.modelActivated(c, modelUUID), tc.IsFalse)

	// The commit releases it.
	c.Assert(s.commitActivation(c, deps, modelUUID), tc.ErrorIsNil)
	c.Check(s.modelActivated(c, modelUUID), tc.IsTrue)
	c.Check(s.modelGateExists(c, modelUUID), tc.IsFalse)
	c.Check(s.modelAgentVersion(c, modelUUID), tc.Equals, jujuversion.Current.String())
}

// setClaimPhase forces the model's import claim into the named phase, standing
// in for a concurrent abort or an already-received commit.
func (s *controllerImportSuite) setClaimPhase(c *tc.C, modelUUID coremodel.UUID, phase string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE model_migration_import
SET    phase_type_id = (
           SELECT id FROM model_migration_import_phase_type WHERE type = ?)
WHERE  model_uuid = ?`, phase, modelUUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// commitActivation drives the commit half of activation, as the target's
// AdoptResources call does.
func (*controllerImportSuite) commitActivation(
	c *tc.C, deps migration.Deps, modelUUID coremodel.UUID,
) error {
	importer := migration.NewModelImporter(
		func(coremodel.UUID) coremodelmigration.Scope {
			return coremodelmigration.NewScope(deps.ControllerDB, deps.ModelDB, nil, nil, modelUUID)
		},
		activationDomainServicesGetter{deps: deps},
		"",
		deps.Logger,
		deps.Clock,
	)
	return importer.CommitActivation(c.Context(), modelUUID)
}
