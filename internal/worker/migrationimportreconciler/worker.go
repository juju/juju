// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package migrationimportreconciler provides a controller-scoped worker that
// completes interrupted target-side migration import aborts. When a v8 model
// import is aborted, the durable model_migration_import claim is moved to the
// aborting phase, the controller-database import writes are undone, and the
// model database is staged for deletion, but the claim is deliberately left in
// place until the drop is proven complete. On the facade Abort path that
// finalization is synchronous; this worker is the crash-recovery fallback for
// aborts whose caller did not finish (a source controller that went away, a
// process restart). It periodically scans for aborting claims, re-drives the
// abort compensation (idempotent), and finalizes the abort by deleting the
// claim once the undertaker's model-database deleter has dropped the database.
// It also warns about claims stuck in the importing or activating phase past a
// conservative age, which indicate a source controller that never completed or
// aborted a migration.
package migrationimportreconciler

import (
	"context"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/retry"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	"github.com/juju/juju/internal/errors"
)

const (
	// defaultReconcileInterval is the base interval between reconciler scans.
	// The interval is jittered so multiple controllers (should the singular
	// gate ever hand over) do not scan in lockstep.
	defaultReconcileInterval = time.Minute

	// staleClaimThreshold is how old an importing or activating claim must be
	// before the reconciler warns about it. It is deliberately conservative: a
	// long-running import must not be flagged, and there is no auto-recovery of
	// stale claims (that requires an operator-driven Abort or activation retry).
	staleClaimThreshold = 24 * time.Hour

	// staleWarnInterval rate-limits stale-claim warnings to at most once per
	// interval per model.
	staleWarnInterval = time.Hour

	// minAbortBackoff and maxAbortBackoff bound the per-model exponential
	// backoff applied when finalizing an aborting claim fails.
	minAbortBackoff = time.Minute
	maxAbortBackoff = 30 * time.Minute
)

// Service is the subset of the modelmigration import service the reconciler
// needs to scan and finalize aborting claims.
type Service interface {
	// GetAllImportClaims returns a snapshot of every outstanding import claim.
	GetAllImportClaims(ctx context.Context) ([]modelmigration.ImportClaimStatus, error)

	// FinalizeAbortedImport deletes the model's import claim and its companion
	// rows once abort cleanup is provably complete (the model database has been
	// dropped). It returns [modelmigrationerrors.ErrAbortNotFinalizable] when
	// cleanup is not yet provable.
	FinalizeAbortedImport(ctx context.Context, modelUUID coremodel.UUID) error
}

// AbortFunc re-drives target-side abort compensation for a model, transitioning
// the claim to aborting (if needed), undoing the controller-database import
// writes, and staging the model database for deletion. It is
// [github.com/juju/juju/internal/migration.AbortModelImport] bound to the
// controller database.
type AbortFunc func(ctx context.Context, modelUUID coremodel.UUID) error

// Config is the configuration for the migration import reconciler.
type Config struct {
	// Service scans and finalizes import claims.
	Service Service

	// Abort re-drives the controller-database abort compensation for a model.
	Abort AbortFunc

	// Clock provides the current time and timers.
	Clock clock.Clock

	// Logger logs reconciler activity.
	Logger logger.Logger
}

// Validate checks the configuration is complete.
func (c Config) Validate() error {
	if c.Service == nil {
		return errors.Errorf("nil Service").Add(coreerrors.NotValid)
	}
	if c.Abort == nil {
		return errors.Errorf("nil Abort").Add(coreerrors.NotValid)
	}
	if c.Clock == nil {
		return errors.Errorf("nil Clock").Add(coreerrors.NotValid)
	}
	if c.Logger == nil {
		return errors.Errorf("nil Logger").Add(coreerrors.NotValid)
	}
	return nil
}

// modelState holds the reconciler's per-model bookkeeping: the exponential
// backoff for a failing aborting claim, and the last time a stale claim was
// warned about. It is owned by the loop goroutine.
type modelState struct {
	nextRetry  time.Time
	backoff    time.Duration
	lastWarned time.Time
}

// Reconciler completes interrupted target-side migration import aborts.
type Reconciler struct {
	catacomb catacomb.Catacomb
	config   Config

	// models is owned by the loop goroutine and holds per-model backoff and
	// warning state. Entries are removed once a claim is finalized or gone.
	models map[string]*modelState

	mu          sync.Mutex
	lastRun     time.Time
	lastPending int
}

// NewWorker returns a new migration import reconciler.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	w := &Reconciler{
		config: config,
		models: make(map[string]*modelState),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "migration-import-reconciler",
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, errors.Capture(err)
}

// Kill is part of the worker.Worker interface.
func (w *Reconciler) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *Reconciler) Wait() error {
	return w.catacomb.Wait()
}

// Report shows up in the dependency engine report.
func (w *Reconciler) Report(ctx context.Context) map[string]any {
	w.mu.Lock()
	defer w.mu.Unlock()
	return map[string]any{
		"last-run":         w.lastRun,
		"pending-aborting": w.lastPending,
	}
}

func (w *Reconciler) loop() error {
	ctx := w.catacomb.Context(context.Background())

	timer := w.config.Clock.NewTimer(jitter(defaultReconcileInterval))
	defer timer.Stop()

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case <-timer.Chan():
			w.reconcile(ctx)
			timer.Reset(jitter(defaultReconcileInterval))
		}
	}
}

// reconcile scans all outstanding import claims once, finalizing aborting
// claims that are due and warning about stale importing/activating claims. A
// scan-level error is logged and swallowed so the worker keeps running.
func (w *Reconciler) reconcile(ctx context.Context) {
	claims, err := w.config.Service.GetAllImportClaims(ctx)
	if err != nil {
		w.config.Logger.Warningf(ctx, "listing migration import claims: %v", err)
		return
	}

	now := w.config.Clock.Now()
	seen := make(map[string]struct{}, len(claims))
	pending := 0
	for _, claim := range claims {
		seen[claim.ModelUUID] = struct{}{}
		switch claim.Phase {
		case modelmigration.ImportPhaseAborting:
			pending++
			w.reconcileAborting(ctx, claim, now)
		case modelmigration.ImportPhaseImporting, modelmigration.ImportPhaseActivating:
			w.warnIfStale(ctx, claim, now)
		}
	}

	// Drop per-model state for claims that no longer exist (finalized, or
	// activated away), so the map does not grow unbounded.
	for modelUUID := range w.models {
		if _, ok := seen[modelUUID]; !ok {
			delete(w.models, modelUUID)
		}
	}

	w.mu.Lock()
	w.lastRun = now
	w.lastPending = pending
	w.mu.Unlock()
}

// reconcileAborting finalizes a single aborting claim, honouring the per-model
// backoff on repeated failure.
func (w *Reconciler) reconcileAborting(ctx context.Context, claim modelmigration.ImportClaimStatus, now time.Time) {
	state := w.models[claim.ModelUUID]
	if state != nil && now.Before(state.nextRetry) {
		return
	}

	modelUUID := coremodel.UUID(claim.ModelUUID)
	if err := w.finalizeAbort(ctx, modelUUID); err != nil {
		w.recordFailure(claim, now, err)
		return
	}

	// Success: the claim is gone (or provably will be on the next scan). Clear
	// any backoff state.
	delete(w.models, claim.ModelUUID)
	w.config.Logger.Infof(ctx,
		"finalized aborted migration import for model %q (source migration %q)",
		claim.ModelUUID, claim.SourceMigrationUUID)
}

// finalizeAbort re-drives the abort compensation (which also stages the model
// database for deletion by the undertaker's model-database deleter), then
// finalizes the claim. Finalization only releases the claim once the database
// drop is proven complete (the staged deletion row is gone), so
// FinalizeAbortedImport returns ErrAbortNotFinalizable until then and this
// method retries on the next scan.
func (w *Reconciler) finalizeAbort(ctx context.Context, modelUUID coremodel.UUID) error {
	if err := w.config.Abort(ctx, modelUUID); err != nil {
		return errors.Errorf("re-driving abort compensation: %w", err)
	}

	if err := w.config.Service.FinalizeAbortedImport(ctx, modelUUID); err != nil {
		return errors.Errorf("finalizing aborted import: %w", err)
	}
	return nil
}

// recordFailure applies exponential backoff for a model whose abort
// finalization failed. A not-yet-finalizable result is expected and logged
// quietly; any other error is warned.
func (w *Reconciler) recordFailure(claim modelmigration.ImportClaimStatus, now time.Time, err error) {
	state := w.models[claim.ModelUUID]
	if state == nil {
		state = &modelState{}
		w.models[claim.ModelUUID] = state
	}
	if state.backoff == 0 {
		state.backoff = minAbortBackoff
	} else if state.backoff < maxAbortBackoff {
		state.backoff *= 2
		if state.backoff > maxAbortBackoff {
			state.backoff = maxAbortBackoff
		}
	}
	state.nextRetry = now.Add(state.backoff)

	ctx := w.catacomb.Context(context.Background())
	if errors.Is(err, modelmigrationerrors.ErrAbortNotFinalizable) {
		w.config.Logger.Debugf(ctx,
			"migration import abort for model %q not yet finalizable, retrying in %s: %v",
			claim.ModelUUID, state.backoff, err)
		return
	}
	w.config.Logger.Warningf(ctx,
		"finalizing migration import abort for model %q failed, retrying in %s: %v",
		claim.ModelUUID, state.backoff, err)
}

// warnIfStale emits a rate-limited warning for an importing or activating claim
// that has not changed phase for longer than staleClaimThreshold. It never
// mutates the claim: recovering a stale claim requires an operator-driven Abort
// (for importing) or an activation retry (for activating).
func (w *Reconciler) warnIfStale(ctx context.Context, claim modelmigration.ImportClaimStatus, now time.Time) {
	if now.Sub(claim.UpdatedAt) < staleClaimThreshold {
		return
	}
	state := w.models[claim.ModelUUID]
	if state != nil && !state.lastWarned.IsZero() && now.Sub(state.lastWarned) < staleWarnInterval {
		return
	}
	if state == nil {
		state = &modelState{}
		w.models[claim.ModelUUID] = state
	}
	state.lastWarned = now
	w.config.Logger.Warningf(ctx,
		"migration import claim for model %q has been in the %q phase since %s (source migration %q); "+
			"if the source controller is gone, an operator-driven abort (importing) or activation retry "+
			"(activating) is required to release the model UUID",
		claim.ModelUUID, claim.Phase, claim.UpdatedAt.Format(time.RFC3339), claim.SourceMigrationUUID)
}

// jitter returns a random duration between 0.5 and 1.5 times the given period.
func jitter(period time.Duration) time.Duration {
	half := period / 2
	return retry.ExpBackoff(half, period+half, 2, true)(0, 1)
}
