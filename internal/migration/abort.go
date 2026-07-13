// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"

	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	migrationclaimservice "github.com/juju/juju/domain/modelmigration/service"
	migrationclaimstate "github.com/juju/juju/domain/modelmigration/state/controller"
	"github.com/juju/juju/internal/errors"
)

// AbortModelImport drives target-side cleanup of a partially imported v8 model.
// It is the single entry point for both the facade Abort call and the abort
// reconciler's retries, and is idempotent and safe to call repeatedly.
//
// It transitions the durable model_migration_import claim to the aborting phase
// (refusing the transition once activation has crossed the point of no return),
// then undoes the controller-database import writes in reverse order via
// [RemoveOnAbortImport]. It deliberately leaves the claim in the aborting phase:
// the abort reconciler drops the model database and finalizes (deletes the
// claim) only once cleanup is provably complete, so the model UUID stays claimed
// and every concurrent Import keeps seeing a cleanup-in-progress AlreadyExists
// until then.
//
// deps.ModelDB is not required: the abort compensation writes only to the
// controller database, and passing a nil ModelDB makes that structural.
//
// It returns [modelmigrationerrors.ErrAbortActivating] when the claim is
// activating (a non-retryable conflict: the model must not be torn down after
// activation has begun), and nil when no claim exists (nothing was imported, or
// cleanup already finalized).
func AbortModelImport(ctx context.Context, deps Deps, modelUUID coremodel.UUID) error {
	claim := migrationclaimservice.NewImportService(
		migrationclaimstate.New(deps.ControllerDB, deps.Clock), deps.Logger,
	)

	c, err := claim.GetImportClaim(ctx, modelUUID)
	switch {
	case errors.Is(err, modelmigrationerrors.ErrImportNotFound):
		// The claim is the first target-side write of an import, so a missing
		// claim means nothing was imported for this model, or a prior abort has
		// already finalized cleanup. Either way there is nothing to do.
		return nil
	case err != nil:
		return errors.Errorf("reading import claim for model %q: %w", modelUUID, err)
	}

	switch c.Phase {
	case modelmigration.ImportPhaseActivating:
		// Activation has crossed the point of no return; the imported model may
		// not be torn down. This is a non-retryable conflict.
		return errors.Errorf("model %q: %w", modelUUID, modelmigrationerrors.ErrAbortActivating)
	case modelmigration.ImportPhaseImporting:
		if err := claim.SetImportPhaseAborting(ctx, modelUUID); err != nil {
			// A concurrent activation may have won the race between the read
			// above and this compare-and-set; surface the conflict (as
			// ErrAbortActivating) or the transition error unchanged.
			return errors.Errorf(
				"transitioning import claim to aborting for model %q: %w", modelUUID, err)
		}
	case modelmigration.ImportPhaseAborting:
		// Already aborting: re-drive compensation below.
	default:
		return errors.Errorf("model %q: unexpected import claim phase %q", modelUUID, c.Phase)
	}

	// Undo the controller-database import writes in reverse order. This is
	// idempotent and envelope-free: it derives everything it removes from the
	// model UUID alone.
	args := ImportModelArgs{
		ControllerModelInfo: coremodelmigration.ControllerModelInfo{
			ModelInfo: coremodelmigration.ModelIdentityInfo{UUID: modelUUID.String()},
		},
	}
	if err := RemoveOnAbortImport(ctx, deps, args); err != nil {
		return errors.Errorf("removing partial import for model %q: %w", modelUUID, err)
	}
	return nil
}
