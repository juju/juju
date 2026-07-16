// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"
	"time"

	"github.com/juju/retry"

	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	migrationclaimservice "github.com/juju/juju/domain/modelmigration/service"
	"github.com/juju/juju/internal/errors"
)

// AbortFinalizeWait bounds how long [WaitAbortFinalized] blocks waiting for the
// model database to be dropped and the import claim released.
type AbortFinalizeWait struct {
	// Delay is the constant interval between finalization attempts.
	Delay time.Duration

	// MaxDuration is the total time budget across all attempts.
	MaxDuration time.Duration
}

// DefaultAbortFinalizeWait is the wait applied on the facade Abort path: poll
// finalization twice a second for up to twenty seconds. The model database is
// dropped by the undertaker within milliseconds in the normal case, so
// finalization usually succeeds on the first or second attempt; the budget only
// covers a controller node under load.
var DefaultAbortFinalizeWait = AbortFinalizeWait{
	Delay:       500 * time.Millisecond,
	MaxDuration: 20 * time.Second,
}

// AbortModelImport drives target-side cleanup of a partially imported v8 model.
// It is the single entry point for both the facade Abort call and the abort
// reconciler's retries, and is idempotent and safe to call repeatedly.
//
// It transitions the durable model_migration_import claim to the aborting phase
// (refusing the transition once activation has crossed the point of no return),
// undoes the controller-database import writes in reverse order via
// [RemoveOnAbortImport], then hands the model database off to the undertaker's
// model-database deleter by staging its deletion. It deliberately leaves the
// claim in the aborting phase: the claim is deleted only once the database drop
// is proven complete, by [WaitAbortFinalized] on the facade path or the abort
// reconciler otherwise. Until then the model UUID stays claimed and every
// concurrent Import keeps seeing a cleanup-in-progress AlreadyExists.
//
// deps.ModelDB is not required: the abort compensation writes only to the
// controller database, and passing a nil ModelDB makes that structural.
//
// The modelmigration import service is injected by the caller (the apiserver
// domain services on the facade path, or the reconciler's own controller-scoped
// service): this package never builds it from raw database handles.
//
// It returns [modelmigrationerrors.ErrAbortActivating] when the claim is
// activating (a non-retryable conflict: the model must not be torn down after
// activation has begun), and nil when no claim exists (nothing was imported, or
// cleanup already finalized).
//
// It also returns nil, doing nothing, when the model is already being torn down
// by the generic removal undertaker (a v7/legacy abort marked it dead and took
// the claim's abort lock): that path owns the teardown, and re-driving v8
// compensation over it would race the undertaker.
func AbortModelImport(ctx context.Context, deps Deps, claim *migrationclaimservice.Service, modelUUID coremodel.UUID) error {
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

	// Stand aside if the generic removal undertaker is already tearing this
	// model down. A v7/legacy abort (the migrationtarget API.Abort facade path)
	// marks the model dead and takes the claim's abort lock in one transaction
	// (domain/removal ... MarkMigratingModelAsDead); the undertaker then owns
	// teardown of the model, its database and - via removeBasicModelData - the
	// import claim. Re-driving v8 compensation here (the abort reconciler picks
	// up the aborting claim too) would stage a model-database deletion and
	// deregister the namespace mid-teardown, racing the undertaker's own
	// DeleteModel/DeleteDB. A v8 abort never marks the model dead, so this only
	// fires for the legacy path.
	if removing, err := claim.IsModelRemovalInProgress(ctx, modelUUID); err != nil {
		return errors.Errorf("checking removal state for model %q: %w", modelUUID, err)
	} else if removing {
		return nil
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

	// Stage the model database for deletion by the undertaker's model-database
	// deleter (running on every controller node, so this works from any node).
	// Staging removes the namespace registration, so a re-drive after the drop
	// completes sees no registration and skips this; it is idempotent regardless.
	// A claim that was concurrently finalized reports ErrImportNotFound, which is
	// success here.
	if registered, err := claim.IsImportNamespaceRegistered(ctx, modelUUID); err != nil {
		return errors.Errorf("checking namespace registration for model %q: %w", modelUUID, err)
	} else if registered {
		if err := claim.StageAbortedModelDatabaseDeletion(ctx, modelUUID); err != nil &&
			!errors.Is(err, modelmigrationerrors.ErrImportNotFound) {
			return errors.Errorf("staging model database deletion for model %q: %w", modelUUID, err)
		}
	}
	return nil
}

// WaitAbortFinalized blocks until the aborted model's import claim can be
// finalized (the model database has been dropped by the undertaker and the
// claim deleted), or the wait budget is exhausted. It is called on the facade
// Abort path so the model UUID is released before the RPC returns, matching the
// synchronous abort behaviour of earlier Juju releases.
//
// It polls [migrationclaimservice.Service.FinalizeAbortedImport] every
// wait.Delay for up to wait.MaxDuration, retrying only while finalization is not
// yet provable ([modelmigrationerrors.ErrAbortNotFinalizable]). On success the
// claim is gone. When the budget is exhausted it returns nil after logging: the
// claim stays in the aborting phase and the abort reconciler finalizes it later,
// so the abort is never lost. Any other error (a genuine finalization failure)
// is returned.
//
// The modelmigration import service is injected by the caller; deps supplies
// only the clock and logger for the bounded wait.
func WaitAbortFinalized(ctx context.Context, deps Deps, claim *migrationclaimservice.Service, modelUUID coremodel.UUID, wait AbortFinalizeWait) error {
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			return claim.FinalizeAbortedImport(ctx, modelUUID)
		},
		IsFatalError: func(err error) bool {
			return !errors.Is(err, modelmigrationerrors.ErrAbortNotFinalizable)
		},
		Clock:       deps.Clock,
		Delay:       wait.Delay,
		MaxDuration: wait.MaxDuration,
		Stop:        ctx.Done(),
	})
	switch {
	case err == nil:
		return nil
	case retry.IsDurationExceeded(err) || retry.IsAttemptsExceeded(err):
		// The undertaker has not dropped the database within the budget. The
		// claim is still aborting; the reconciler completes it later.
		deps.Logger.Warningf(ctx,
			"model %q abort accepted but claim finalization still pending; the reconciler will complete it",
			modelUUID)
		return nil
	case retry.IsRetryStopped(err):
		// The context was cancelled (client gone or shutdown). The reconciler
		// completes the abort later.
		deps.Logger.Warningf(ctx,
			"model %q abort finalization interrupted; the reconciler will complete it",
			modelUUID)
		return nil
	default:
		return errors.Errorf("finalizing aborted import for model %q: %w", modelUUID, retry.LastError(err))
	}
}
