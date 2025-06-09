// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoredrainer

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/fortress"
)

// ObjectStoreService provides access to the object store for draining
// operations.
type ObjectStoreService interface {
	// GetDrainingPhase returns the current active draining phase of the
	// object store.
	GetDrainingPhase(ctx context.Context) (objectstore.Phase, error)
	// WatchDraining returns a watcher that watches the draining phase of the
	// object store.
	WatchDraining(ctx context.Context) (watcher.Watcher[struct{}], error)
}

// Config holds the dependencies and configuration for a Worker.
type Config struct {
	Guard              fortress.Guard
	ObjectStoreService ObjectStoreService
	Clock              clock.Clock
	Logger             logger.Logger
}

// Validate returns an error if the config cannot be expected to
// drive a functional Worker.
func (config Config) Validate() error {
	if config.Guard == nil {
		return errors.NotValidf("nil Guard")
	}
	if config.ObjectStoreService == nil {
		return errors.NotValidf("nil ObjectStoreService")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// NewWorker returns a Worker that tracks the result of the configured.
func NewWorker(ctx context.Context, config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name: "objectstoredrainer",
		IsFatal: func(err error) bool {
			return false
		},
		Clock:  config.Clock,
		Logger: internalworker.WrapLogger(config.Logger),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{
		config: config,
		runner: runner,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "objectstoredrainer",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{
			runner,
		},
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Worker watches the object store service for changes to the draining
// phase. If the phase is draining, it locks the guard. If the phase is not
// draining, it unlocks the guard.
// The worker will manage the lifecycle of the watcher and will stop
// watching when the worker is killed or when the context is cancelled.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config

	runner *worker.Runner
}

// Kill kills the worker. It will cause the worker to stop if it is
// not already stopped. The worker will transition to the dying state.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the worker to finish. It will cause the worker to
// stop if it is not already stopped. It will return an error if the
// worker was killed with an error.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

func (w *Worker) loop() error {
	ctx := w.catacomb.Context(context.Background())

	service := w.config.ObjectStoreService
	watcher, err := service.WatchDraining(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-watcher.Changes():
			phase, err := service.GetDrainingPhase(ctx)
			if err != nil {
				return errors.Trace(err)
			}

			// We're not draining, so we can unlock the guard and wait
			// for the next change.
			if !phase.IsDraining() {
				w.config.Logger.Infof(ctx, "object store is not draining, unlocking guard")

				if err := w.config.Guard.Unlock(ctx); err != nil {
					return errors.Errorf("failed to update guard: %v", err)
				}
				continue
			}

			w.config.Logger.Infof(ctx, "object store is draining, locking guard")

			// TODO (stickupkid): Handle the draining phase.
			if err := w.config.Guard.Lockdown(ctx); err != nil {
				return errors.Errorf("failed to update guard: %v", err)
			}
		}
	}
}
