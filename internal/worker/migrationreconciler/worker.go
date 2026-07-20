// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationreconciler

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/retry"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"
	"gopkg.in/tomb.v2"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	"github.com/juju/juju/internal/errors"
	internalworker "github.com/juju/juju/internal/worker"
)

const (
	// staleClaimThreshold is how old an importing or activating claim must be
	// before the reconciler warns about it. It is deliberately conservative: a
	// long-running import must not be flagged, and there is no auto-recovery of
	// stale claims (that requires an operator-driven Abort or activation retry).
	staleClaimThreshold = 24 * time.Hour

	// staleWarnInterval rate-limits stale-claim warnings to at most once per
	// interval per model.
	staleWarnInterval = time.Hour

	// staleClaimScanInterval is the base interval between scans used solely for
	// stale-claim warnings. Abort reconciliation is driven immediately by the
	// import-claim watcher.
	staleClaimScanInterval = staleWarnInterval

	// restartDelay is the delay the runner applies before restarting an abort
	// worker that exited with a non-fatal error (e.g. finalization not yet
	// provable because the undertaker has not dropped the database).
	restartDelay = time.Minute
)

// Service is the subset of the modelmigration import service the reconciler
// needs to scan and finalize aborting claims.
type Service interface {
	// WatchImportClaims emits the initial collection of model UUIDs with import
	// claims, followed by changed model UUIDs.
	WatchImportClaims(context.Context) (watcher.StringsWatcher, error)

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

// Config is the configuration for the migration reconciler.
type Config struct {
	// Service scans and finalizes import claims.
	Service Service

	// Abort re-drives the controller-database abort compensation for a model.
	Abort AbortFunc

	// Clock provides the current time and timers.
	Clock clock.Clock

	// Logger logs reconciler activity.
	Logger logger.Logger

	// NewWorker creates and returns a migration reconciler worker.
	NewWorker func(Config) (worker.Worker, error)
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

// reconciler completes interrupted target-side migration import aborts.
type reconciler struct {
	catacomb catacomb.Catacomb
	config   Config
	runner   *worker.Runner

	// staleWarnings tracks the last time a stale-claim warning was emitted for
	// a model, so warnings are rate-limited. It is owned by the loop goroutine.
	staleWarnings map[string]time.Time
}

// NewWorker returns a new migration reconciler.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name:          "migration-reconciler",
		IsFatal:       func(error) bool { return false },
		ShouldRestart: func(error) bool { return true },
		RestartDelay:  restartDelay,
		Clock:         config.Clock,
		Logger:        internalworker.WrapLogger(config.Logger),
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	w := &reconciler{
		config:        config,
		runner:        runner,
		staleWarnings: make(map[string]time.Time),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "migration-reconciler",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{w.runner},
	}); err != nil {
		return nil, errors.Capture(err)
	}
	return w, nil
}

// Kill is part of the worker.Worker interface.
func (w *reconciler) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *reconciler) Wait() error {
	return w.catacomb.Wait()
}

// Report shows up in the dependency engine report.
func (w *reconciler) Report(ctx context.Context) map[string]any {
	return w.runner.Report(ctx)
}

func (w *reconciler) loop() error {
	ctx := w.catacomb.Context(context.Background())

	watch, err := w.config.Service.WatchImportClaims(ctx)
	if err != nil {
		return errors.Errorf("watching migration import claims: %w", err)
	}
	if err := w.catacomb.Add(watch); err != nil {
		return errors.Errorf("adding migration import claims watcher: %w", err)
	}

	timer := w.config.Clock.NewTimer(jitter(staleClaimScanInterval))
	defer timer.Stop()

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case _, ok := <-watch.Changes():
			if !ok {
				return errors.New("migration import claims watcher closed")
			}
			// Events are hints only: they may be coalesced, so the full current
			// snapshot remains the source of truth for reconciliation.
			if err := w.reconcile(ctx); err != nil {
				return errors.Errorf("reconciling migration import claims: %w", err)
			}

		case <-timer.Chan():
			if err := w.reconcile(ctx); err != nil {
				return errors.Errorf("reconciling stale migration import claims: %w", err)
			}
			timer.Reset(jitter(staleClaimScanInterval))
		}
	}
}

// reconcile scans all outstanding import claims once, spawning per-model abort
// workers for aborting claims and warning about stale importing/activating
// claims. The caller decides what to do with a scan error.
func (w *reconciler) reconcile(ctx context.Context) error {
	claims, err := w.config.Service.GetAllImportClaims(ctx)
	if err != nil {
		return errors.Errorf("listing migration import claims: %w", err)
	}

	now := w.config.Clock.Now()
	seen := make(map[string]struct{}, len(claims))
	for _, claim := range claims {
		seen[claim.ModelUUID] = struct{}{}
		switch claim.Phase {
		case modelmigration.ImportPhaseAborting:
			// The only non-AlreadyExists error the runner returns here is
			// ErrDead, which means we are shutting down; abandon the scan.
			if err := w.startAbortWorker(ctx, claim); err != nil {
				return errors.Capture(err)
			}
		case modelmigration.ImportPhaseImporting, modelmigration.ImportPhaseActivating:
			w.warnIfStale(ctx, claim, now)
		}
	}

	// Drop stale-warning state for claims that no longer exist, so the map
	// does not grow unbounded.
	for modelUUID := range w.staleWarnings {
		if _, ok := seen[modelUUID]; !ok {
			delete(w.staleWarnings, modelUUID)
		}
	}
	return nil
}

// startAbortWorker starts a per-model abort worker for an aborting claim. The
// abort worker re-drives abort compensation and finalizes the claim; on failure
// it exits and the runner restarts it after restartDelay. Once finalization
// succeeds the abort worker exits cleanly and the runner forgets it (the claim
// is gone, so the next scan does not re-spawn it).
//
// The runner is the single source of truth for whether a model already has a
// worker: it reports AlreadyExists while one exists, whether it is running or
// waiting to be restarted. Asking it first (via WorkerNames) and starting
// second would be a check-then-act race.
func (w *reconciler) startAbortWorker(ctx context.Context, claim modelmigration.ImportClaimStatus) error {
	err := w.runner.StartWorker(ctx, claim.ModelUUID, newAbortWorker(
		w.config.Abort, w.config.Service, coremodel.UUID(claim.ModelUUID),
	))
	if errors.Is(err, coreerrors.AlreadyExists) {
		// This model is already being finalized; nothing to do.
		return nil
	} else if err != nil {
		return errors.Errorf("starting abort worker for model %q: %w", claim.ModelUUID, err)
	}
	w.config.Logger.Infof(ctx,
		"scheduling abort finalization for model %q (source migration %q)",
		claim.ModelUUID, claim.SourceMigrationUUID)
	return nil
}

// warnIfStale emits a rate-limited warning for an importing or activating claim
// that has not changed phase for longer than staleClaimThreshold.
//
// It never mutates the claim, because only the source controller knows whether
// the migration is still wanted: it drives the target's Abort (for importing) or
// re-drives Activate (for activating) via its migrationmaster. A claim only goes
// stale when the source stops driving it - typically because the source
// controller is gone - and in that case there is currently no target-side
// command an operator can run to release the model UUID. So this warning is
// deliberately just a signpost for support rather than a recovery instruction.
func (w *reconciler) warnIfStale(ctx context.Context, claim modelmigration.ImportClaimStatus, now time.Time) {
	if now.Sub(claim.UpdatedAt) < staleClaimThreshold {
		return
	}
	lastWarned, ok := w.staleWarnings[claim.ModelUUID]
	if ok && now.Sub(lastWarned) < staleWarnInterval {
		return
	}
	w.staleWarnings[claim.ModelUUID] = now
	w.config.Logger.Warningf(ctx,
		"migration import claim for model %q has been in the %q phase since %s (source migration %q); "+
			"the model UUID stays claimed until the source controller completes or aborts the migration",
		claim.ModelUUID, claim.Phase, claim.UpdatedAt.Format(time.RFC3339), claim.SourceMigrationUUID)
}

// jitter returns a randomised duration around the given period. ExpBackoff
// applies ±20% jitter, so the result lands in roughly 0.8–1.2 times the period
// (within the [0.5, 1.5] period clamp), enough to keep controllers from scanning
// in lockstep.
func jitter(period time.Duration) time.Duration {
	half := period / 2
	return retry.ExpBackoff(half, period+half, 2, true)(0, 1)
}

// abortWorker is a per-model worker that finalizes a single aborted import.
// It re-drives abort compensation (idempotent) and then attempts finalization.
// If finalization is not yet provable (the undertaker has not dropped the
// database), it returns a non-fatal error; the runner restarts it after
// restartDelay. If finalization succeeds, it returns nil and is not restarted
// (the claim is gone, so the next scan does not re-spawn it).
type abortWorker struct {
	tomb      tomb.Tomb
	abort     AbortFunc
	service   Service
	modelUUID coremodel.UUID
}

func newAbortWorker(
	abort AbortFunc, service Service, modelUUID coremodel.UUID,
) func(context.Context) (worker.Worker, error) {
	return func(ctx context.Context) (worker.Worker, error) {
		w := &abortWorker{
			abort:     abort,
			service:   service,
			modelUUID: modelUUID,
		}
		w.tomb.Go(w.run)
		return w, nil
	}
}

func (w *abortWorker) run() error {
	ctx := w.tomb.Context(context.Background())

	if err := w.abort(ctx, w.modelUUID); err != nil {
		return errors.Errorf("re-driving abort compensation: %w", err)
	}

	if err := w.service.FinalizeAbortedImport(ctx, w.modelUUID); err != nil {
		if errors.Is(err, modelmigrationerrors.ErrAbortNotFinalizable) {
			// Not yet provable: the undertaker has not dropped the database.
			// Return a non-fatal error so the runner restarts us after
			// restartDelay.
			return errors.Errorf(
				"abort finalization for model %q not yet provable: %w",
				w.modelUUID, err)
		}
		return errors.Errorf("finalizing aborted import: %w", err)
	}
	return nil
}

// Kill is part of the worker.Worker interface.
func (w *abortWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *abortWorker) Wait() error {
	return w.tomb.Wait()
}
