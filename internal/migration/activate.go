// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	cmrmodelstate "github.com/juju/juju/domain/crossmodelrelation/state/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modelstate "github.com/juju/juju/domain/model/state/controller"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	migrationclaimstate "github.com/juju/juju/domain/modelmigration/state/controller"
	migrationmodelstate "github.com/juju/juju/domain/modelmigration/state/model"
	"github.com/juju/juju/internal/errors"
)

// ActivateModelArgs carries the data needed to activate an imported model.
// It is built by the apiserver facade from the params.ActivateModelArgs RPC
// payload and resolved controller/model scope.
type ActivateModelArgs struct {
	// ModelUUID is the UUID of the model being activated on this controller.
	ModelUUID coremodel.UUID

	// SourceControllerUUID is the UUID of the source controller. It is used to
	// reconcile CMR offerer-controller references during activation.
	SourceControllerUUID string

	// SourceControllerAlias is the human-readable alias of the source
	// controller, recorded when the source controller is registered as an
	// external controller for CMR offerer reconciliation.
	SourceControllerAlias string

	// SourceCACert is the CA certificate of the source controller, recorded
	// when the source controller is registered as an external controller.
	SourceCACert string

	// SourceAPIAddrs are the API addresses of the source controller, recorded
	// when the source controller is registered as an external controller.
	SourceAPIAddrs []string

	// CrossModelUUIDs are the model UUIDs that have cross-model relations to
	// the source controller after migration. They drive CMR offerer-controller
	// reconciliation during activation.
	CrossModelUUIDs []string
}

// ActivateModel finalises the activation of a model imported via the v8 path.
// It runs a durable phase machine — importing → activating → claim deleted —
// so retrying after a crash at any step resumes safely; every step is
// idempotent.
//
// Legacy (3.6/4.0) imports set the model_migrating gate but create no
// model_migration_import claim. When no claim exists ActivateModel clears the
// gate and succeeds, preserving backward compatibility.
func ActivateModel(ctx context.Context, deps Deps, args ActivateModelArgs) error {
	modelUUID := args.ModelUUID
	modelUUIDStr := modelUUID.String()

	mmCtrl := migrationclaimstate.New(deps.ControllerDB, deps.Clock)
	mmModel := migrationmodelstate.New(deps.ModelDB, modelUUID)

	// Check for a v8 import claim. A missing claim means legacy import.
	claim, err := mmCtrl.GetImportClaim(ctx, modelUUIDStr)
	hasClaim := err == nil
	if err != nil && !errors.Is(err, modelmigrationerrors.ErrImportNotFound) {
		return errors.Errorf("checking import claim for model %q: %w", modelUUIDStr, err)
	}

	// 1. Transition to activating (v8 only) — point of no return.
	if hasClaim {
		switch claim.Phase {
		case modelmigration.ImportPhaseAborting:
			return errors.Errorf("model %q: %w", modelUUIDStr, modelmigrationerrors.ErrActivationAborting)
		case modelmigration.ImportPhaseImporting:
			if err := mmCtrl.SetImportPhaseActivating(ctx, modelUUIDStr); err != nil {
				return errors.Errorf(
					"transitioning import claim to activating for model %q: %w",
					modelUUIDStr, err,
				)
			}
		case modelmigration.ImportPhaseActivating:
			// Idempotent retry: already past the point of no return.
		}
	}

	// 2. CMR offerer-controller reconciliation: populate
	// application_remote_offerer.offerer_controller_uuid while the model is
	// gated. All updates are idempotent so a retry after a crash is safe.
	if err := reconcileOffererControllers(ctx, mmCtrl, deps, modelUUIDStr, args); err != nil {
		return errors.Errorf(
			"reconciling offerer controller UUIDs for model %q: %w", modelUUIDStr, err,
		)
	}

	// 3. Bump model agent version to match controller target, if needed.
	if err := bumpModelAgentVersion(ctx, mmCtrl, mmModel, modelUUIDStr); err != nil {
		return errors.Errorf(
			"bumping model agent version during activation of model %q: %w",
			modelUUIDStr, err,
		)
	}

	// 4. Clear the model-DB import gate.
	if err := mmModel.DeleteModelImportingStatus(ctx); err != nil {
		return errors.Errorf(
			"clearing import gate for model %q: %w", modelUUIDStr, err,
		)
	}

	// 4.5. Activate the model row itself in the controller DB. This is a
	// distinct flag from the migration claim: model_migration_import tracks
	// the migration's own phase machine, while model.activated is the
	// generic "model creation is fully complete" flag every model carries
	// (migrated or not) and is what v_model/CheckModelExists/GetModel gate
	// on. Without this, the model stays permanently invisible even after the
	// claim above is deleted. Idempotent: a retry that finds the row already
	// activated is a success, not an error.
	modelCtrl := modelstate.NewState(deps.ControllerDB)
	if err := modelCtrl.Activate(ctx, modelUUID); err != nil && !errors.Is(err, modelerrors.AlreadyActivated) {
		return errors.Errorf("activating model %q: %w", modelUUIDStr, err)
	}

	// 5. Delete the import claim last (v8 only): after the gate is gone, a
	// second call with no claim is an idempotent success.
	if hasClaim {
		if err := mmCtrl.DeleteActivatedImport(ctx, modelUUIDStr); err != nil {
			return errors.Errorf(
				"deleting activated import claim for model %q: %w", modelUUIDStr, err,
			)
		}
	}

	return nil
}

// reconcileOffererControllers populates
// application_remote_offerer.offerer_controller_uuid for all cross-model
// relations that cross a controller boundary, while the model gate is still
// held. It handles two cases:
//
//   - Source-hosted offerers: applications whose offering model UUID is in
//     args.CrossModelUUIDs. These lived on the source controller before
//     migration and now need their controller reference updated to point at the
//     source controller.
//
//   - Third-party offerers: applications whose offering model is hosted on a
//     controller other than the source, recorded in
//     model_migration_import_external_controller_model during import. Only
//     present on the v8 path (hasClaim).
//
// The source controller is registered via EnsureExternalControllerExists
// (compare-or-insert) rather than the legacy blind upsert. All CMR updates are
// idempotent.
func reconcileOffererControllers(
	ctx context.Context,
	mmCtrl *migrationclaimstate.State,
	deps Deps,
	modelUUIDStr string,
	args ActivateModelArgs,
) error {
	if args.SourceControllerUUID == "" {
		return nil
	}
	if len(args.CrossModelUUIDs) > 0 {
		// Register the source controller using compare-or-insert semantics.
		if err := mmCtrl.EnsureSourceControllerExists(
			ctx,
			args.SourceControllerUUID,
			args.SourceControllerAlias,
			args.SourceCACert,
			args.SourceAPIAddrs,
			args.CrossModelUUIDs,
		); err != nil {
			return errors.Errorf(
				"registering source controller %q: %w", args.SourceControllerUUID, err,
			)
		}

		// Populate offerer_controller_uuid for source-hosted offers.
		cmrState := cmrmodelstate.NewState(deps.ModelDB, coremodel.UUID(modelUUIDStr), deps.Clock, deps.Logger)
		for _, offererModelUUID := range args.CrossModelUUIDs {
			if err := cmrState.SetOffererControllerForOffererModel(
				ctx, offererModelUUID, args.SourceControllerUUID,
			); err != nil {
				return errors.Errorf(
					"setting offerer controller for source-hosted model %q: %w",
					offererModelUUID, err,
				)
			}
		}
	}

	// Third-party offerers (v8 only): mappings recorded during
	// ImportExternalControllers, read from the companion table.
	thirdParty, err := mmCtrl.ExternalControllerModelsForImport(ctx, modelUUIDStr)
	if err != nil {
		return errors.Errorf(
			"reading third-party offerer mappings for model %q: %w", modelUUIDStr, err,
		)
	}
	if len(thirdParty) > 0 {
		cmrState := cmrmodelstate.NewState(deps.ModelDB, coremodel.UUID(modelUUIDStr), deps.Clock, deps.Logger)
		for _, m := range thirdParty {
			if err := cmrState.SetOffererControllerForOffererModel(
				ctx, m.OffererModelUUID, m.ControllerUUID,
			); err != nil {
				return errors.Errorf(
					"setting offerer controller for third-party model %q: %w",
					m.OffererModelUUID, err,
				)
			}
		}
	}
	return nil
}

// bumpModelAgentVersion updates the model's target agent version to match the
// controller's target version when they differ.  It is idempotent: if the
// versions already match it is a no-op.
func bumpModelAgentVersion(
	ctx context.Context,
	ctrl *migrationclaimstate.State,
	model *migrationmodelstate.State,
	modelUUIDStr string,
) error {
	desiredStr, err := ctrl.GetControllerTargetVersion(ctx)
	if err != nil {
		return errors.Errorf("getting controller target version: %w", err)
	}
	if desiredStr == "" {
		return errors.New("controller target version is not set")
	}
	desired, err := semversion.Parse(desiredStr)
	if err != nil {
		return errors.Errorf("parsing controller target version %q: %w", desiredStr, err)
	}

	currentStr, err := model.GetModelTargetAgentVersion(ctx)
	if err != nil {
		return errors.Errorf(
			"getting model target agent version for model %q: %w", modelUUIDStr, err,
		)
	}
	current, err := semversion.Parse(currentStr)
	if err != nil {
		return errors.Errorf(
			"parsing model target agent version %q: %w", currentStr, err,
		)
	}

	if current == desired {
		return nil
	}
	return model.SetModelTargetAgentVersion(ctx, currentStr, desiredStr)
}
