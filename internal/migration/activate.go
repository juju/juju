// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"

	corecontroller "github.com/juju/juju/core/controller"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
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

// activateModel finalises the activation of a model imported via the v8 path.
// It runs a durable phase machine: importing, activating, then claim deleted,
// so retrying after a crash at any step resumes safely; every step is
// idempotent.
//
// Legacy (3.6/4.0) imports set the model_migrating gate but create no
// model_migration_import claim. When no claim exists, activateModel clears the
// gate and succeeds, preserving backward compatibility.
func activateModel(ctx context.Context, domainServices services.DomainServices, args ActivateModelArgs) error {
	modelUUID := args.ModelUUID
	modelUUIDStr := modelUUID.String()

	// Check for a v8 import claim. A missing claim means legacy import.
	claim, err := domainServices.ModelMigration().GetImportClaim(ctx, modelUUID)
	hasClaim := err == nil
	if err != nil && !errors.Is(err, modelmigrationerrors.ErrImportNotFound) {
		return errors.Errorf("checking import claim for model %q: %w", modelUUIDStr, err)
	}

	// 1. Transition to activating (v8 only): point of no return.
	if hasClaim {
		switch claim.Phase {
		case modelmigration.ImportPhaseAborting:
			return errors.Errorf("model %q: %w", modelUUIDStr, modelmigrationerrors.ErrActivationAborting)
		case modelmigration.ImportPhaseImporting:
			if err := domainServices.ModelMigration().SetImportPhaseActivating(ctx, modelUUID); err != nil {
				return errors.Errorf(
					"transitioning import claim to activating for model %q: %w",
					modelUUIDStr, err,
				)
			}
		case modelmigration.ImportPhaseActivating:
			// Idempotent retry: already past the point of no return.
		default:
			return errors.Errorf(
				"model %q: unexpected import claim phase %q",
				modelUUIDStr, claim.Phase,
			)
		}
	}

	// 2. CMR offerer-controller reconciliation: populate
	// application_remote_offerer.offerer_controller_uuid while the model is
	// gated. All updates are idempotent so a retry after a crash is safe.
	if err := reconcileOffererControllers(ctx, domainServices, modelUUID, hasClaim, args); err != nil {
		return errors.Errorf(
			"reconciling offerer controller UUIDs for model %q: %w", modelUUIDStr, err,
		)
	}

	// 3. Reconcile the model agent version to match the controller target, if
	// needed.
	if err := reconcileModelAgentVersion(ctx, domainServices, modelUUIDStr); err != nil {
		return errors.Errorf(
			"reconciling model agent version during activation of model %q: %w",
			modelUUIDStr, err,
		)
	}

	// 4. Clear the model-DB import gate.
	// Steps 4 and 5 write to different databases and so cannot share a
	// transaction, but a crash between them leaves no half-visible model:
	// visibility gates on model.activated (set in step 5), so until step 5
	// lands the model stays invisible regardless of the gate. Both steps are
	// idempotent, so a retry resumes safely.
	if err := domainServices.ModelMigration().DeleteModelImportingStatus(ctx); err != nil {
		return errors.Errorf(
			"clearing import gate for model %q: %w", modelUUIDStr, err,
		)
	}

	// 5. Activate the model row itself in the controller DB. This is a
	// distinct flag from the migration claim: model_migration_import tracks
	// the migration's own phase machine, while model.activated is the
	// generic "model creation is fully complete" flag every model carries
	// (migrated or not) and is what v_model/CheckModelExists/GetModel gate
	// on. Without this, the model stays permanently invisible even after the
	// claim is later deleted. Idempotent: a retry that finds the row already
	// activated is a success, not an error.
	if err := domainServices.Model().ActivateModel(ctx, modelUUID); err != nil && !errors.Is(err, modelerrors.AlreadyActivated) {
		return errors.Errorf("activating model %q: %w", modelUUIDStr, err)
	}

	// 6. Delete the import claim last (v8 only): after the gate is gone, a
	// second call with no claim is an idempotent success.
	if hasClaim {
		if err := domainServices.ModelMigration().DeleteActivatedImport(ctx, modelUUID); err != nil {
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
// The source controller is registered via EnsureSourceControllerExists
// (compare-or-insert) rather than the legacy blind upsert. All CMR updates are
// idempotent.
func reconcileOffererControllers(
	ctx context.Context,
	domainServices services.DomainServices,
	modelUUID coremodel.UUID,
	hasClaim bool,
	args ActivateModelArgs,
) error {
	if args.SourceControllerUUID == "" {
		return nil
	}

	if len(args.CrossModelUUIDs) > 0 {
		crossModelUUIDs := make([]coremodel.UUID, len(args.CrossModelUUIDs))
		for i, u := range args.CrossModelUUIDs {
			crossModelUUIDs[i] = coremodel.UUID(u)
		}

		// Register the source controller using compare-or-insert semantics. The
		// service generates the external-controller address row UUIDs.
		if err := domainServices.ModelMigration().EnsureSourceControllerExists(
			ctx,
			corecontroller.UUID(args.SourceControllerUUID),
			args.SourceControllerAlias,
			args.SourceCACert,
			args.SourceAPIAddrs,
			crossModelUUIDs,
		); err != nil {
			return errors.Errorf(
				"registering source controller %q: %w", args.SourceControllerUUID, err,
			)
		}

		// Point all source-hosted offers at the source controller in a single
		// UPDATE.
		if err := domainServices.CrossModelRelation().SetOffererControllerForOffererModels(
			ctx, crossModelUUIDs, corecontroller.UUID(args.SourceControllerUUID),
		); err != nil {
			return errors.Errorf(
				"setting offerer controller for source-hosted models: %w", err,
			)
		}
	}

	// Third-party offerers (v8 only): mappings recorded during
	// ImportExternalControllers, read from the companion table. Legacy imports
	// have no claim and therefore no mappings, so skip the query entirely.
	if !hasClaim {
		return nil
	}
	// We first retrieve them (from the controller DB) to later be inserted into
	// the model DB.
	thirdParty, err := domainServices.ModelMigration().ExternalControllerModelsForImport(ctx, modelUUID)
	if err != nil {
		return errors.Errorf(
			"reading third-party offerer mappings for model %q: %w", modelUUID, err,
		)
	}

	// Group the offerer models by their controller so each distinct controller
	// is updated in a single batched call, shrinking the partial-failure window
	// on a crash. Each call is idempotent, so a retry is safe.
	modelsByController := make(map[corecontroller.UUID][]coremodel.UUID)
	for _, m := range thirdParty {
		controllerUUID := corecontroller.UUID(m.ControllerUUID)
		modelsByController[controllerUUID] = append(
			modelsByController[controllerUUID], coremodel.UUID(m.ModelUUID),
		)
	}
	for controllerUUID, modelUUIDs := range modelsByController {
		if err := domainServices.CrossModelRelation().SetOffererControllerForOffererModels(
			ctx, modelUUIDs, controllerUUID,
		); err != nil {
			return errors.Errorf(
				"setting offerer controller %q for third-party models: %w",
				controllerUUID, err,
			)
		}
	}
	return nil
}

// reconcileModelAgentVersion updates the model's target agent version to match
// the controller's target version when they differ.  It is idempotent: if the
// versions already match it is a no-op.
func reconcileModelAgentVersion(
	ctx context.Context,
	domainServices services.DomainServices,
	modelUUIDStr string,
) error {
	desiredStr, err := domainServices.ModelMigration().GetControllerTargetVersion(ctx)
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

	currentStr, err := domainServices.ModelMigration().GetModelTargetAgentVersion(ctx)
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
	return domainServices.ModelMigration().SetModelTargetAgentVersion(ctx, currentStr, desiredStr)
}
