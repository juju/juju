// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package removal

import (
	"context"
	"time"

	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/errors"
	internalworker "github.com/juju/juju/internal/worker"
)

// jobCheckMaxInterval is the maximum time between checks of the removal table,
// to see if any jobs need processing.
const jobCheckMaxInterval = 30 * time.Second

// Config holds configuration required to run the removal worker.
type Config struct {

	// RemovalService supplies the removal domain logic to the worker.
	RemovalService RemovalService

	// Clock is used by the worker to create timers.
	Clock Clock

	// Logger logs stuff.
	Logger logger.Logger
}

// Validate ensures that the configuration is
// correctly populated for worker operation.
func (config Config) Validate() error {
	if config.RemovalService == nil {
		return errors.New("nil RemovalService not valid").Add(coreerrors.NotValid)
	}
	if config.Clock == nil {
		return errors.New("nil Clock not valid").Add(coreerrors.NotValid)
	}
	if config.Logger == nil {
		return errors.New("nil Logger not valid").Add(coreerrors.NotValid)
	}
	return nil
}

type removalWorker struct {
	catacomb catacomb.Catacomb

	cfg    Config
	runner *worker.Runner
}

// NewWorker starts a new removal worker based
// on the input configuration and returns it.
func NewWorker(cfg Config) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	w := &removalWorker{
		cfg: cfg,
		// Scheduled removal jobs never restart and never
		// propagate their errors up to the worker.
		runner: worker.NewRunner(worker.RunnerParams{
			IsFatal:       func(error) bool { return false },
			ShouldRestart: func(error) bool { return false },
			Logger:        internalworker.WrapLogger(cfg.Logger),
		}),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{w.runner},
	}); err != nil {
		return nil, errors.Capture(err)
	}

	return w, nil
}

func (w *removalWorker) loop() (err error) {
	watch, err := w.cfg.RemovalService.WatchRemovals()
	if err != nil {
		return errors.Capture(err)
	}

	if err := w.catacomb.Add(watch); err != nil {
		return errors.Capture(err)
	}

	ctx := w.catacomb.Context(context.Background())

	timer := w.cfg.Clock.NewTimer(jobCheckMaxInterval)
	defer timer.Stop()

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case jobIDs := <-watch.Changes():
			w.cfg.Logger.Infof(ctx, "got removal job changes: %v", jobIDs)
			if err := w.processRemovalJobs(); err != nil {
				return errors.Capture(err)
			}
			timer.Reset(jobCheckMaxInterval)
		case <-timer.Chan():
			if err := w.processRemovalJobs(); err != nil {
				return errors.Capture(err)
			}
			timer.Reset(jobCheckMaxInterval)
		}
	}
}

// processRemovalJobs interrogates *all* current jobs scheduled for removal.
// For each one whose scheduled start time has passed, we check to see if there
// is an entry in our runner for it. If there is, it is already being processed
// and we ignore it. Otherwise, it is commenced in a new runner.
// This is safe due to the following conditions:
// - This is the only method interrogating/adding workers to the runner.
// - It is only invoked from cases in the main event loop, so is Goroutine safe.
func (w *removalWorker) processRemovalJobs() error {
	return nil
}

// Kill (worker.Worker) tells the worker to stop and return from its loop.
func (w *removalWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait (worker.Worker) waits for the worker to stop,
// and returns the error with which it exited.
func (w *removalWorker) Wait() error {
	return w.catacomb.Wait()
}
