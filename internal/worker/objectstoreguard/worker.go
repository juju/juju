// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoreguard

import (
	"context"

	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/worker/fortress"
)

// DrainingService provides the object-store draining state and watcher.
type DrainingService interface {
	GetDrainingPhase(ctx context.Context) (objectstore.Phase, error)
	IsObjectStoreUsable(ctx context.Context) (bool, error)
	WatchDraining(ctx context.Context) (watcher.NotifyWatcher, error)
}

// Config holds the dependencies required by the object-store guard worker.
type Config struct {
	Guard              fortress.Guard
	DrainingService    DrainingService
	ObjectStoreFlusher objectstore.ObjectStoreFlusher
	Logger             logger.Logger
}

// Validate ensures the worker has all of its required dependencies.
func (c Config) Validate() error {
	if c.Guard == nil {
		return errors.Errorf("nil Guard").Add(coreerrors.NotValid)
	}
	if c.DrainingService == nil {
		return errors.Errorf("nil DrainingService").Add(coreerrors.NotValid)
	}
	if c.ObjectStoreFlusher == nil {
		return errors.Errorf("nil ObjectStoreFlusher").Add(coreerrors.NotValid)
	}
	if c.Logger == nil {
		return errors.Errorf("nil Logger").Add(coreerrors.NotValid)
	}
	return nil
}

// Worker watches the draining phase and opens or closes the local object-store
// fortress. The primary controller's objectstoredrainer is responsible for
// copying data; this worker only synchronizes the guard on other controllers.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config
}

// NewWorker creates an object-store guard worker.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	w := &Worker{config: config}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "objectstoreguard",
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Capture(err)
	}
	return w, nil
}

// Kill stops the worker.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the worker to stop.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

func (w *Worker) loop() error {
	ctx := w.catacomb.Context(context.Background())

	drainingWatcher, err := w.config.DrainingService.WatchDraining(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	if err := w.catacomb.Add(drainingWatcher); err != nil {
		return errors.Capture(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case _, ok := <-drainingWatcher.Changes():
			if !ok {
				select {
				case <-w.catacomb.Dying():
					return w.catacomb.ErrDying()
				default:
				}
				return errors.New("object-store draining watcher closed")
			}
			phase, err := w.config.DrainingService.GetDrainingPhase(ctx)
			if err != nil {
				return errors.Errorf("getting object-store draining phase: %w", err)
			}
			if err := w.syncGuard(ctx, phase); err != nil {
				return errors.Capture(err)
			}

		}
	}
}

func (w *Worker) syncGuard(ctx context.Context, phase objectstore.Phase) error {
	if phase.IsNotStarted() || phase == objectstore.PhaseCompleted {
		usable, err := w.config.DrainingService.IsObjectStoreUsable(ctx)
		if err != nil {
			return errors.Errorf("checking object-store usability: %w", err)
		}
		if !usable {
			w.config.Logger.Warningf(ctx, "object store is not usable, keeping local guard locked")
			return w.lockGuard(ctx)
		}

		w.config.Logger.Infof(ctx, "object store is usable, refreshing local stores")
		if err := w.config.ObjectStoreFlusher.FlushWorkers(ctx); err != nil {
			return errors.Errorf("refreshing local object stores: %w", err)
		}

		w.config.Logger.Infof(ctx, "object store is not draining, unlocking local guard")
		if err := w.config.Guard.Unlock(ctx); err != nil {
			return errors.Errorf("unlocking local object-store guard: %w", err)
		}
		return nil
	}

	switch phase {
	case objectstore.PhaseDraining:
		w.config.Logger.Infof(ctx, "object store is draining, locking local guard")
		return w.lockGuard(ctx)
	case objectstore.PhaseError:
		w.config.Logger.Errorf(ctx, "object store draining is in an error state, keeping local guard locked")
		return w.lockGuard(ctx)
	default:
		return errors.Errorf("unknown object-store draining phase %q", phase)
	}
}

func (w *Worker) lockGuard(ctx context.Context) error {
	if err := w.config.Guard.Lockdown(ctx); err != nil {
		return errors.Errorf("locking local object-store guard: %w", err)
	}
	return nil
}
