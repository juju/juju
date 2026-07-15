// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationreconciler

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/retry"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"
	"gopkg.in/tomb.v2"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	"github.com/juju/juju/internal/errors"
	internalworker "github.com/juju/juju/internal/worker"
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

	// restartDelay is the delay the runner applies before restarting a model
	// job worker that exited with a non-fatal error (e.g. finalization not yet
	// provable because the undertaker has not dropped the database).
	restartDelay = time.Minute
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

// reconcile scans all outstanding import claims once, spawning per-model job
// workers for aborting claims and warning about stale importing/activating
// claims. A scan-level error is logged and swallowed so the worker keeps running.
func (w *reconciler) reconcile(ctx context.Context) {
	claims, err := w.config.Service.GetAllImportClaims(ctx)
	if err != nil {
		w.config.Logger.Warningf(ctx, "listing migration import claims: %v", err)
		return
	}

	now := w.config.Clock.Now()
	seen := make(map[string]struct{}, len(claims))
	for _, claim := range claims {
		seen[claim.ModelUUID] = struct{}{}
		switch claim.Phase {
		case modelmigration.ImportPhaseAborting:
			w.startAbortWorker(ctx, claim)
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
}

// startAbortWorker starts a per-model job worker for an aborting claim if one
// is not already running. The job worker re-drives abort compensation and
// finalizes the claim; on failure it exits and the runner restarts it after
// restartDelay. Once finalization succeeds the job worker exits cleanly and is
// not restarted (the claim is gone, so the next scan does not re-spawn it).
func (w *reconciler) startAbortWorker(ctx context.Context, claim modelmigration.ImportClaimStatus) {
	running := set.NewStrings(w.runner.WorkerNames()...)
	if running.Contains(claim.ModelUUID) {
		return
	}
	w.config.Logger.Infof(ctx,
		"scheduling abort finalization for model %q (source migration %q)",
		claim.ModelUUID, claim.SourceMigrationUUID)
	if err := w.runner.StartWorker(ctx, claim.ModelUUID, newAbortJobWorker(
		w.config.Abort, w.config.Service, coremodel.UUID(claim.ModelUUID),
	)); err != nil {
		w.config.Logger.Warningf(ctx, "starting abort worker for model %q: %v", claim.ModelUUID, err)
	}
}

// warnIfStale emits a rate-limited warning for an importing or activating claim
// that has not changed phase for longer than staleClaimThreshold. It never
// mutates the claim: recovering a stale claim requires an operator-driven Abort
// (for importing) or an activation retry (for activating).
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
			"if the source controller is gone, an operator-driven abort (importing) or activation retry "+
			"(activating) is required to release the model UUID",
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

// abortJobWorker is a per-model worker that finalizes a single aborted import.
// It re-drives abort compensation (idempotent) and then attempts finalization.
// If finalization is not yet provable (the undertaker has not dropped the
// database), it returns a non-fatal error; the runner restarts it after
// restartDelay. If finalization succeeds, it returns nil and is not restarted
// (the claim is gone, so the next scan does not re-spawn it).
type abortJobWorker struct {
	tomb      tomb.Tomb
	abort     AbortFunc
	service   Service
	modelUUID coremodel.UUID
}

func newAbortJobWorker(
	abort AbortFunc, service Service, modelUUID coremodel.UUID,
) func(context.Context) (worker.Worker, error) {
	return func(ctx context.Context) (worker.Worker, error) {
		w := &abortJobWorker{
			abort:     abort,
			service:   service,
			modelUUID: modelUUID,
		}
		w.tomb.Go(w.run)
		return w, nil
	}
}

func (w *abortJobWorker) run() error {
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
func (w *abortJobWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *abortJobWorker) Wait() error {
	return w.tomb.Wait()
}
