// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"

	corecontroller "github.com/juju/juju/core/controller"
	corelogger "github.com/juju/juju/core/logger"
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

// Migrating a model between two controllers is a commit across two
// controllers, and only the source can decide its outcome: it is the side that
// chooses between SUCCESS and ABORT. The target's job is to carry out that
// decision, never to guess it.
//
// The source's phase machine makes the decision legible. Any error activating
// the model sends it to ABORT, and it treats the target's cleanup as best
// effort. But once it durably records SUCCESS it can never reach ABORT again -
// it only rolls forward, retrying failures. So the target has exactly one
// reliable commit signal: the first message the source sends after recording
// SUCCESS, which is AdoptResources.
//
// Activation is therefore split in two, with no new RPC and no new error code,
// so an unmodified 3.6 or 4.0 source speaks the protocol unchanged:
//
//	prepareActivation  every fallible, reversible step. Called during
//	                   VALIDATION, while the source may still abort.
//	commitActivation   the irreversible transition. Called during SUCCESS, so
//	                   its arrival is the commit decision.

// prepareActivation performs every fallible, reversible step of activating an
// imported model, and nothing else.
//
// It is 3.6's Activate minus its final act of releasing the model. Each step is
// idempotent, and every failure leaves the model exactly as it was: claim still
// importing, gate still closed, workers still parked. That is the point - the
// source treats any failure here, or a reply it never receives, as a reason to
// abort, so nothing done here may prevent that abort from succeeding.
func prepareActivation(
	ctx context.Context,
	domainServices services.DomainServices,
	args ActivateModelArgs,
	logger corelogger.Logger,
) error {
	modelUUID := args.ModelUUID

	// Every import creates a claim, legacy ones included, so claim existence
	// says nothing about which path imported this model.
	claim, err := domainServices.ModelMigration().GetImportClaim(ctx, modelUUID)
	switch {
	case errors.Is(err, modelmigrationerrors.ErrImportNotFound):
		// No claim: either this model was never imported here, or it was
		// already committed and released. Only the latter is a success.
		if committed, err := committedPredicates(ctx, domainServices, modelUUID); err != nil {
			return errors.Capture(err)
		} else if committed {
			return nil
		}
		return errors.Errorf("model %q is not importing", modelUUID)
	case err != nil:
		return errors.Errorf("reading import claim for model %q: %w", modelUUID, err)
	}

	switch claim.Phase {
	case modelmigration.ImportPhaseAborting:
		// Cleanup has started; the source will abort anyway.
		return errors.Errorf("model %q: %w", modelUUID, modelmigrationerrors.ErrActivationAborting)
	case modelmigration.ImportPhaseActivating:
		// The commit already arrived, so there is nothing left to prepare.
		// Reporting success beats failing: a stale caller that failed here
		// would drive an abort the target must then refuse.
		return nil
	case modelmigration.ImportPhaseImporting:
	default:
		return errors.Errorf("model %q: unexpected import claim phase %q", modelUUID, claim.Phase)
	}

	// Validate the imported model before any write below. The checks are
	// read-only, so a validation failure leaves the model exactly as it was:
	// claim still importing, gate still closed, and the source free to abort.
	if err := domainServices.ModelMigration().ValidateImportedModel(ctx); err != nil {
		return errors.Errorf("validating imported model %q: %w", modelUUID, err)
	}

	if err := reconcileOffererControllers(ctx, domainServices, modelUUID, args); err != nil {
		return errors.Errorf(
			"reconciling offerer controller UUIDs for model %q: %w", modelUUID, err)
	}

	if err := reconcileModelAgentVersion(ctx, domainServices, modelUUID.String(), logger); err != nil {
		return errors.Errorf(
			"reconciling model agent version during activation of model %q: %w", modelUUID, err)
	}

	// Re-check the claim last. Preparation writes only shared controller rows,
	// using compare-or-insert, so an abort racing it has nothing to undo - but a
	// caller told preparation succeeded should not then meet a refused commit.
	if err := domainServices.ModelMigration().AssertImporting(ctx, modelUUID); err != nil {
		return errors.Errorf("model %q import claim changed during activation: %w", modelUUID, err)
	}
	return nil
}

// commitActivation performs the irreversible half of activation: it records
// that the source committed, releases the model, and adopts its cloud
// resources.
//
// It is driven by AdoptResources, which a source sends only after durably
// recording SUCCESS. Receipt is therefore proof that the source can never abort
// this migration, which is what makes the transition safe to treat as a point
// of no return.
//
// This is an ordered, replayable sequence rather than a transaction, and cannot
// be otherwise: the claim lives in the controller database, the gate in the
// model database, and resource adoption is a call out to the cloud provider.
// Each step is idempotent so that a commit interrupted anywhere is finished by
// repeating all of them - that replayability is what stands in for the
// atomicity a single database write would have given.
//
// The order matters. Releasing before adopting matches 3.6, which released the
// model in Activate and adopted afterwards, and it keeps model availability
// independent of the provider. An adoption failure therefore does not undo the
// release: the model is already this controller's, and the source retries
// AdoptResources until it succeeds.
func commitActivation(
	ctx context.Context,
	domainServices services.DomainServices,
	modelUUID coremodel.UUID,
	sourceControllerVersion semversion.Number,
) error {
	claim, err := domainServices.ModelMigration().GetImportClaim(ctx, modelUUID)
	switch {
	case errors.Is(err, modelmigrationerrors.ErrImportNotFound):
		// The claim is gone: either this commit already completed and its reply
		// was lost, or the model was never imported here. Only the former may
		// pass, and it has nothing left to do.
		committed, err := committedPredicates(ctx, domainServices, modelUUID)
		if err != nil {
			return errors.Capture(err)
		}
		if !committed {
			return errors.Errorf("model %q is not importing or activating", modelUUID)
		}
		// Already released by an earlier commit whose reply was lost. Nothing
		// left to do locally, but the adoption may not have run yet.
		return adoptResources(ctx, domainServices, modelUUID, sourceControllerVersion)
	case err != nil:
		return errors.Errorf("reading import claim for model %q: %w", modelUUID, err)
	}

	switch claim.Phase {
	case modelmigration.ImportPhaseAborting:
		// Unreachable from a correct source: a migration that reached SUCCESS
		// never drove an abort. Refuse loudly rather than resurrect a model
		// whose teardown is under way.
		return errors.Errorf("model %q: cannot commit activation, import is aborting", modelUUID)
	case modelmigration.ImportPhaseActivating:
		// An interrupted commit; everything below is idempotent, so resume.
	case modelmigration.ImportPhaseImporting:
		// The commit record itself.
		if err := domainServices.ModelMigration().SetImportPhaseActivating(ctx, modelUUID); err != nil {
			return errors.Errorf("recording activation commit for model %q: %w", modelUUID, err)
		}
	default:
		return errors.Errorf("model %q: unexpected import claim phase %q", modelUUID, claim.Phase)
	}

	if err := releaseModel(ctx, domainServices, modelUUID); err != nil {
		return errors.Capture(err)
	}

	return adoptResources(ctx, domainServices, modelUUID, sourceControllerVersion)
}

// adoptResources asks the cloud provider to re-tag the model's resources with
// this controller, so they are not destroyed when the source controller goes
// away.
//
// It runs last, after the model has already been released, and its error is
// returned to the source, which retries the whole call. A retry finds no claim,
// confirms the model was already released, and reaches here again to repeat the
// adoption alone.
func adoptResources(
	ctx context.Context,
	domainServices services.DomainServices,
	modelUUID coremodel.UUID,
	sourceControllerVersion semversion.Number,
) error {
	if err := domainServices.ModelMigration().AdoptResources(ctx, sourceControllerVersion); err != nil {
		return errors.Errorf("adopting cloud resources for model %q: %w", modelUUID, err)
	}
	return nil
}

// releaseModel makes a committed model usable and gives up the claim. Every
// step is idempotent, so a commit interrupted part-way is finished by repeating
// all of them.
//
// Claim deletion is last and is what actually releases the model: it is the
// change the migration flag watches, so the model's workers start at that
// moment and not before.
func releaseModel(
	ctx context.Context, domainServices services.DomainServices, modelUUID coremodel.UUID,
) error {
	if err := domainServices.ModelMigration().DeleteModelImportingStatus(ctx); err != nil {
		return errors.Errorf("clearing import gate for model %q: %w", modelUUID, err)
	}

	// model.activated is the generic "model creation is complete" flag every
	// model carries, distinct from the migration claim. A v8 import leaves it
	// false until here, so this is where the model becomes generally usable;
	// agents reach the model during validation without it, because the API
	// server serves connections for a model an import claim still covers (see
	// apiserver.modelConnectionFor). A legacy import activates the model as its
	// final import step, so for those this is a no-op.
	if err := domainServices.Model().ActivateModel(ctx, modelUUID); err != nil &&
		!errors.Is(err, modelerrors.AlreadyActivated) {
		return errors.Errorf("activating model %q: %w", modelUUID, err)
	}

	if err := domainServices.ModelMigration().DeleteActivatedImport(ctx, modelUUID); err != nil {
		return errors.Errorf("releasing import claim for model %q: %w", modelUUID, err)
	}
	return nil
}

// committedPredicates reports whether a model with no import claim shows every
// sign of having been released by a completed commit.
//
// A missing claim is ambiguous on its own - it equally describes a model that
// was never imported and one whose abort finished - so callers that treat it as
// success must confirm it rather than assume.
func committedPredicates(
	ctx context.Context, domainServices services.DomainServices, modelUUID coremodel.UUID,
) (bool, error) {
	// CheckModelExists is false for a model that is absent *or* not yet
	// activated, which is exactly the distinction wanted here.
	exists, err := domainServices.Model().CheckModelExists(ctx, modelUUID)
	if err != nil {
		return false, errors.Errorf("checking model %q exists: %w", modelUUID, err)
	}
	if !exists {
		return false, nil
	}

	gated, err := domainServices.ModelMigration().IsModelImporting(ctx)
	if err != nil {
		return false, errors.Errorf("checking import gate for model %q: %w", modelUUID, err)
	}
	return !gated, nil
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
//     model_migration_import_external_controller_model during import.
//
// The source controller is registered via EnsureSourceControllerExists
// (compare-or-insert) rather than the legacy blind upsert. All CMR updates are
// idempotent.
func reconcileOffererControllers(
	ctx context.Context,
	domainServices services.DomainServices,
	modelUUID coremodel.UUID,
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

	// Third-party offerers: mappings recorded during ImportExternalControllers,
	// read from the companion table. A legacy import records none, so this
	// simply returns nothing for it - there is no need to branch on the import
	// path, and no way to tell them apart by claim existence anyway.
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
//
// 3.6 never changed a migrated model's agent version, so a missing agent binary
// is never fatal here: if the target lacks binaries for a running architecture
// at the desired version, the model is left at its current version (whose
// binaries the source uploaded during import) and a warning is logged. The
// operator upgrades later via upgrade-model, which consults simplestreams.
// Activation is never blocked on binary availability.
func reconcileModelAgentVersion(
	ctx context.Context,
	domainServices services.DomainServices,
	modelUUIDStr string,
	logger corelogger.Logger,
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

	missing, err := domainServices.ModelMigration().MissingAgentBinaryArchitectures(ctx, desiredStr)
	if err != nil {
		return errors.Errorf(
			"checking agent binary availability for version %q: %w", desiredStr, err,
		)
	}
	if len(missing) > 0 {
		logger.Warningf(ctx,
			"not upgrading migrated model %q agent version from %q to %q: "+
				"no agent binaries for architecture(s) %q on this controller; "+
				"run 'juju upgrade-model' once binaries are available",
			modelUUIDStr, currentStr, desiredStr, missing,
		)
		return nil
	}

	return domainServices.ModelMigration().SetModelTargetAgentVersion(ctx, currentStr, desiredStr)
}
