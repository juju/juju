// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"
	"time"

	"github.com/juju/worker/v5"

	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	migrationclaimservice "github.com/juju/juju/domain/modelmigration/service"
	"github.com/juju/juju/internal/errors"
)

// AbortFinalizeWait bounds how long [WaitAbortFinalized] blocks waiting for the
// model database to be dropped and the import claim released.
type AbortFinalizeWait struct {
	// Delay is the interval between fallback finalization re-checks. The wait is
	// primarily driven by the model-database-deletion watcher; this re-check
	// backs it up in case an event is coalesced.
	Delay time.Duration

	// MaxDuration is the total time budget across all attempts.
	MaxDuration time.Duration
}

// DefaultAbortFinalizeWait is the wait applied on the facade Abort path: wait
// for finalization for up to twenty seconds, re-checking every half second as a
// fallback to the watcher. The model database is dropped by the undertaker
// within milliseconds in the normal case, so finalization usually succeeds on
// the first attempt; the budget only covers a controller node under load.
var DefaultAbortFinalizeWait = AbortFinalizeWait{
	Delay:       500 * time.Millisecond,
	MaxDuration: 20 * time.Second,
}

// abortFinalizer is the subset of the modelmigration import service that
// [WaitAbortFinalized] needs: finalize the aborted claim once the database drop
// is proven, and watch the staged model-database deletion so the wait reacts to
// the drop completing instead of polling blindly.
type abortFinalizer interface {
	// FinalizeAbortedImport deletes the model's import claim once abort cleanup
	// is provably complete, returning
	// [modelmigrationerrors.ErrAbortNotFinalizable] while it is not.
	FinalizeAbortedImport(ctx context.Context, modelUUID coremodel.UUID) error

	// WatchModelDatabaseDeletion fires when the staged model-database deletion
	// for the model changes, including when the undertaker removes it after
	// dropping the database.
	WatchModelDatabaseDeletion(ctx context.Context, modelUUID coremodel.UUID) (watcher.NotifyWatcher, error)
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

	// The model is dying, so the generic removal undertaker already owns its
	// teardown (a v7/legacy abort marks the model dead and takes the claim's
	// abort lock). Re-driving v8 compensation over it would race the
	// undertaker's own DeleteModel/DeleteDB, so leave it alone. A v8 abort
	// never marks the model dead, so this only fires for the legacy path.
	if dying, err := claim.IsModelDying(ctx, modelUUID); err != nil {
		return errors.Errorf("checking model life for %q: %w", modelUUID, err)
	} else if dying {
		return nil
	}

	switch c.Phase {
	case modelmigration.ImportPhaseActivating:
		// Not a programming error, and not a race: Activate takes the claim past
		// the point of no return before it can fail, and the source cannot tell a
		// failed Activate from one whose reply it never received. Either way its
		// VALIDATION phase drives ABORT and lands here. Refuse: the model may in
		// fact be activated, so it must not be torn down.
		return errors.Errorf("model %q: %w", modelUUID, modelmigrationerrors.ErrAbortActivating)
	case modelmigration.ImportPhaseImporting:
		if err := claim.SetImportPhaseAborting(ctx, modelUUID); err != nil {
			// The claim read above is not part of the transition transaction, so
			// a concurrent activation may have won the race. SetImportPhaseAborting
			// re-reads the phase inside its own transaction and reports
			// ErrAbortActivating itself, so wrapping preserves that sentinel.
			return errors.Errorf(
				"transitioning import claim to aborting for model %q: %w", modelUUID, err)
		}
	case modelmigration.ImportPhaseAborting:
		// A previous abort was interrupted before it finalized the claim (or the
		// reconciler is retrying one); re-drive the idempotent compensation below.
		deps.Logger.Debugf(ctx,
			"model %q import claim is already aborting; re-driving abort compensation", modelUUID)
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
// claim deleted), or the wait timeout occurs. It is called on the facade
// Abort path so the model UUID is released before the RPC returns, matching the
// synchronous abort behaviour of earlier Juju releases.
//
// The database drop that unblocks finalization happens out of band in the
// undertaker's model-database deleter, which removes the model's staged
// deletion row when it is done. This function watches that row and re-attempts
// finalization when it changes, rather than polling blindly: FinalizeAbortedImport
// commits successfully every time and reports
// [modelmigrationerrors.ErrAbortNotFinalizable] as a normal result while the
// drop is not yet proven, so only the drop event (or the fallback re-check) can
// make the condition come true. A periodic re-check backs the watcher up in
// case an event is coalesced.
//
// On success the claim is gone. When the budget is exhausted it returns nil
// after logging: the claim stays in the aborting phase and the reconciler
// finalizes it later, so the abort is never lost. Any other error (a genuine
// finalization failure) is returned.
//
// The modelmigration import service is injected by the caller; deps supplies
// only the clock and logger for the bounded wait.
func WaitAbortFinalized(ctx context.Context, deps Deps, claim abortFinalizer, modelUUID coremodel.UUID, wait AbortFinalizeWait) error {
	// Subscribe before the first finalize attempt so the drop cannot slip
	// through between a check and the subscription.
	w, err := claim.WatchModelDatabaseDeletion(ctx, modelUUID)
	if err != nil {
		return errors.Errorf("watching model database deletion for model %q: %w", modelUUID, err)
	}
	defer func() { _ = worker.Stop(w) }()

	timeout := deps.Clock.After(wait.MaxDuration)
	for {
		err := claim.FinalizeAbortedImport(ctx, modelUUID)
		if err == nil {
			return nil
		}
		if !errors.Is(err, modelmigrationerrors.ErrAbortNotFinalizable) {
			return errors.Errorf("finalizing aborted import for model %q: %w", modelUUID, err)
		}

		select {
		case <-ctx.Done():
			// The client is gone, or we are shutting down. The reconciler
			// completes the abort later.
			deps.Logger.Warningf(ctx,
				"model %q abort finalization interrupted; the reconciler will complete it",
				modelUUID)
			return nil
		case <-timeout:
			// The undertaker has not dropped the database within the budget. The
			// claim is still aborting; the reconciler completes it later.
			deps.Logger.Warningf(ctx,
				"model %q abort accepted but claim finalization still pending; the reconciler will complete it",
				modelUUID)
			return nil
		case _, ok := <-w.Changes():
			if !ok {
				return errors.Errorf("model database deletion watcher for model %q closed", modelUUID)
			}
		case <-deps.Clock.After(wait.Delay):
		}
	}
}
