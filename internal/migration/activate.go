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
//
// It runs the fallible, args-dependent reconciliation (CMR offerer controllers,
// model agent version) while the import claim is still in the importing phase,
// then flips the claim to activating — the point of no return — immediately
// before the idempotent, args-independent finalization (clear the model gate,
// activate the model row, delete the claim). This ordering is deliberate: any
// failure before the flip leaves the claim importing, so the source may safely
// abort; everything after the flip is convergent (only transient failures are
// possible) and its errors are wrapped with ErrActivationIncomplete so the
// source retries activation rather than aborting a possibly-live model. See
// ACTIVATE_ABORT_WEDGE.md.
//
// Every step is idempotent, so a retry after a crash resumes safely: an
// already-activating claim skips the reconciliation and re-drives finalization
// only.
//
// Legacy (3.6/4.0) imports set the model_migrating gate but create no
// model_migration_import claim. When no claim exists, activateModel reconciles
// and clears the gate and succeeds, preserving backward compatibility; a
// completed v8 activation (claim already deleted) short-circuits to success.
func activateModel(ctx context.Context, domainServices services.DomainServices, args ActivateModelArgs) error {
	modelUUID := args.ModelUUID
	modelUUIDStr := modelUUID.String()

	// Check for a v8 import claim. A missing claim means legacy import, or a
	// completed v8 activation.
	claim, err := domainServices.ModelMigration().GetImportClaim(ctx, modelUUID)
	hasClaim := err == nil
	if err != nil && !errors.Is(err, modelmigrationerrors.ErrImportNotFound) {
		return errors.Errorf("checking import claim for model %q: %w", modelUUIDStr, err)
	}

	if hasClaim {
		switch claim.Phase {
		case modelmigration.ImportPhaseAborting:
			// Abort cleanup has begun: refuse. This is a pre-point-of-no-return
			// decision (nothing is mutated), so the source may safely abort.
			return errors.Errorf("model %q: %w", modelUUIDStr, modelmigrationerrors.ErrActivationAborting)
		case modelmigration.ImportPhaseImporting, modelmigration.ImportPhaseActivating:
			// importing: run reconciliation, flip, finalize.
			// activating: resume — reconciliation provably completed before the
			// flip, so re-drive finalization only.
		default:
			return errors.Errorf(
				"model %q: unexpected import claim phase %q", modelUUIDStr, claim.Phase,
			)
		}
	} else if activated, err := modelAlreadyActivated(ctx, domainServices, modelUUID); err != nil {
		return errors.Errorf("checking activation state for model %q: %w", modelUUIDStr, err)
	} else if activated {
		// No claim and the model is already activated: a v8 activation that
		// completed and deleted its claim (or an already-activated legacy
		// model). Return success idempotently rather than re-running the
		// reconciliation below, which a lost-reply Activate retry would
		// otherwise hit and could misreport as a pre-point-of-no-return failure,
		// wrongly aborting a now-live model.
		return nil
	}

	// Fallible pre-point-of-no-return work. Runs for a fresh importing claim and
	// for a legacy (no-claim) import; skipped when resuming an already-activating
	// claim. Any error here leaves a v8 claim in the importing phase, so the
	// source may safely abort.
	if !hasClaim || claim.Phase == modelmigration.ImportPhaseImporting {
		// CMR offerer-controller reconciliation: populate
		// application_remote_offerer.offerer_controller_uuid while the model is
		// gated. Idempotent, but can fail permanently on a genuine
		// external-controller conflict (ErrExternalControllerMismatch) — which
		// must therefore surface before the point of no return.
		if err := reconcileOffererControllers(ctx, domainServices, modelUUID, hasClaim, args); err != nil {
			return errors.Errorf(
				"reconciling offerer controller UUIDs for model %q: %w", modelUUIDStr, err,
			)
		}
		// Reconcile the model agent version to match the controller target.
		if err := reconcileModelAgentVersion(ctx, domainServices, modelUUIDStr); err != nil {
			return errors.Errorf(
				"reconciling model agent version during activation of model %q: %w",
				modelUUIDStr, err,
			)
		}
	}

	// The point of no return: flip importing -> activating, immediately before
	// the idempotent finalization. From here the model may become live and must
	// never be torn down.
	if hasClaim && claim.Phase == modelmigration.ImportPhaseImporting {
		if err := domainServices.ModelMigration().SetImportPhaseActivating(ctx, modelUUID); err != nil {
			// The flip did not commit (or a concurrent abort won): still before
			// the point of no return, so the source may abort.
			return errors.Errorf(
				"transitioning import claim to activating for model %q: %w", modelUUIDStr, err,
			)
		}
	}

	// Finalize. For a v8 claim this runs past the point of no return, so wrap any
	// failure with ErrActivationIncomplete (a single coding point) so the source
	// retries activation instead of aborting. Legacy (no-claim) activations have
	// no claim to wedge and keep returning the raw error.
	if err := finalizeActivation(ctx, domainServices, modelUUID, hasClaim); err != nil {
		if hasClaim {
			return errors.Errorf("%w: %w", modelmigrationerrors.ErrActivationIncomplete, err)
		}
		return err
	}
	return nil
}

// CompleteActivation re-drives the idempotent finalization of a v8 import claim
// that is already past the point of no return (the activating phase). It exists
// for the abort reconciler to complete an activation the source worker did not
// finish - for example because the source controller was destroyed
// mid-activation. It needs only the model UUID: all args-dependent
// reconciliation provably completed before the claim crossed the point of no
// return, so finalization is args-independent and convergent.
func CompleteActivation(
	ctx context.Context,
	domainServicesGetter services.DomainServicesGetter,
	modelUUID coremodel.UUID,
) error {
	domainServices, err := domainServicesGetter.ServicesForModel(ctx, modelUUID)
	if err != nil {
		return errors.Errorf("retrieving domain services for model %q: %w", modelUUID, err)
	}
	return finalizeActivation(ctx, domainServices, modelUUID, true)
}

// finalizeActivation runs the idempotent, args-independent activation
// finalization: clear the model-DB import gate, activate the model row in the
// controller DB, and (for a v8 import) delete the import claim last. Every step
// is a delete/set on an existing row that always converges on retry, so it can
// be re-driven safely — by an Activate retry or by the abort reconciler, which
// needs only the model UUID.
//
// The gate (model DB) and the claim (controller DB) are in different databases
// and cannot share a transaction, but a crash between the steps leaves no
// half-visible model: visibility gates on model.activated, and the claim is
// deleted last, so a retry re-drives whatever remains.
func finalizeActivation(
	ctx context.Context,
	domainServices services.DomainServices,
	modelUUID coremodel.UUID,
	hasClaim bool,
) error {
	modelUUIDStr := modelUUID.String()

	if err := domainServices.ModelMigration().DeleteModelImportingStatus(ctx); err != nil {
		return errors.Errorf("clearing import gate for model %q: %w", modelUUIDStr, err)
	}

	if err := domainServices.Model().ActivateModel(ctx, modelUUID); err != nil &&
		!errors.Is(err, modelerrors.AlreadyActivated) {
		return errors.Errorf("activating model %q: %w", modelUUIDStr, err)
	}

	if hasClaim {
		if err := domainServices.ModelMigration().DeleteActivatedImport(ctx, modelUUID); err != nil {
			return errors.Errorf("deleting activated import claim for model %q: %w", modelUUIDStr, err)
		}
	}
	return nil
}

// modelAlreadyActivated reports whether the model row is already activated,
// using GetModelLife (which reports modelerrors.NotActivated for an unactivated
// model).
func modelAlreadyActivated(
	ctx context.Context, domainServices services.DomainServices, modelUUID coremodel.UUID,
) (bool, error) {
	_, err := domainServices.Model().GetModelLife(ctx, modelUUID)
	if errors.Is(err, modelerrors.NotActivated) {
		return false, nil
	}
	if err != nil {
		return false, errors.Capture(err)
	}
	return true, nil
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
