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
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	"github.com/juju/juju/internal/errors"
)

// abortFinalizeWait bounds how long waitAbortFinalized blocks waiting for the
// model database to be dropped and the import claim released.
type abortFinalizeWait struct {
	// Delay is the interval between fallback finalization re-checks. The wait is
	// primarily driven by the model-database-deletion watcher; this re-check
	// backs it up in case an event is coalesced.
	Delay time.Duration

	// MaxDuration is the total time budget across all attempts.
	MaxDuration time.Duration
}

var defaultAbortFinalizeWait = abortFinalizeWait{
	Delay:       500 * time.Millisecond,
	MaxDuration: 20 * time.Second,
}

// abortFinalizer is the subset of the modelmigration import service needed to
// finalize an aborted claim after its model database is dropped.
type abortFinalizer interface {
	FinalizeAbortedImport(ctx context.Context, modelUUID coremodel.UUID) error
	WatchModelDatabaseDeletion(ctx context.Context, modelUUID coremodel.UUID) (watcher.NotifyWatcher, error)
}

type importAbortService interface {
	// GetImportClaim returns the durable import claim for the model, or
	// [modelmigrationerrors.ErrImportNotFound] when no claim exists.
	GetImportClaim(context.Context, coremodel.UUID) (modelmigration.ImportClaim, error)

	// SetImportPhaseAborting transitions the model's import claim from
	// importing to aborting. It returns [modelmigrationerrors.ErrAbortActivating]
	// when the claim has crossed the activation point of no return.
	SetImportPhaseAborting(context.Context, coremodel.UUID) error

	// IsModelNotAlive reports whether the model has left the alive state, i.e.
	// the generic removal undertaker has taken over its teardown.
	IsModelNotAlive(context.Context, coremodel.UUID) (bool, error)

	// IsImportNamespaceRegistered reports whether the model's dqlite namespace
	// is still registered, i.e. whether its model database may still need
	// dropping.
	IsImportNamespaceRegistered(context.Context, coremodel.UUID) (bool, error)

	// StageAbortedModelDatabaseDeletion removes the model's namespace
	// registration and stages the model database for deletion by the
	// undertaker's model-database deleter.
	StageAbortedModelDatabaseDeletion(context.Context, coremodel.UUID) error
}

// abortModelImport cleans up a partially imported v8 model. It leaves the
// durable claim in the aborting phase until the model database is dropped.
func abortModelImport(ctx context.Context, deps deps, claim importAbortService, modelUUID coremodel.UUID) error {
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
		// Not a programming error, and not a race: Activate takes the claim past
		// the point of no return before it can fail, and the source cannot tell a
		// failed Activate from one whose reply it never received.
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
		deps.Logger.Debugf(ctx,
			"model %q import claim is already aborting; re-driving abort compensation", modelUUID)
	default:
		return errors.Errorf("model %q: unexpected import claim phase %q", modelUUID, c.Phase)
	}

	// Check model life after taking or observing the claim's abort lock. This
	// catches a legacy abort that won between the initial claim read and the
	// phase transition. A legacy abort can still begin after this check, but
	// both cleanup paths are idempotent and converge on the same deletion.
	if notAlive, err := claim.IsModelNotAlive(ctx, modelUUID); err != nil &&
		!errors.Is(err, modelerrors.NotFound) {
		return errors.Errorf("checking model life for %q: %w", modelUUID, err)
	} else if notAlive {
		return nil
	}

	// Undo the controller-database import writes in reverse order. This is
	// idempotent and envelope-free: it derives everything it removes from the
	// model UUID alone.
	args := ImportModelArgs{
		ControllerModelInfo: coremodelmigration.ControllerModelInfo{
			ModelInfo: coremodelmigration.ModelIdentityInfo{UUID: modelUUID.String()},
		},
	}
	if err := removeOnAbortImport(ctx, deps, args); err != nil {
		return errors.Errorf("removing partial import for model %q: %w", modelUUID, err)
	}

	// Stage the model database for deletion by the undertaker's model-database
	// deleter (running on every controller node, so this works from any node).
	// Staging removes the namespace registration, so a re-run after the drop
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

// waitAbortFinalized waits for the model database drop and releases the aborted
// import claim, bounded by the supplied duration.
func waitAbortFinalized(ctx context.Context, deps deps, claim abortFinalizer, modelUUID coremodel.UUID, wait abortFinalizeWait) error {
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
			return errors.Capture(ctx.Err())
		case <-timeout:
			return errors.Errorf(
				"timed out finalizing aborted import for model %q: %w",
				modelUUID, modelmigrationerrors.ErrAbortNotFinalizable,
			)
		case _, ok := <-w.Changes():
			if !ok {
				return errors.Errorf("model database deletion watcher for model %q closed", modelUUID)
			}
		case <-deps.Clock.After(wait.Delay):
		}
	}
}
