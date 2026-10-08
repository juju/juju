// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoreguard

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/worker/fortress"
)

// DrainingService provides the object-store draining state and watcher.
type DrainingService interface {
	GetDrainingPhase(ctx context.Context) (objectstore.Phase, error)
	WatchDraining(ctx context.Context) (watcher.NotifyWatcher, error)
}

// Config holds the dependencies required by the object-store guard worker.
type Config struct {
	Guard           fortress.Guard
	DrainingService DrainingService
	Logger          logger.Logger
}

// Validate ensures the worker has all of its required dependencies.
func (c Config) Validate() error {
	if c.Guard == nil {
		return errors.NotValidf("nil Guard")
	}
	if c.DrainingService == nil {
		return errors.NotValidf("nil DrainingService")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
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
		return nil, errors.Trace(err)
	}
	w := &Worker{config: config}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "objectstoreguard",
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
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
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(drainingWatcher); err != nil {
		return errors.Trace(err)
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
				return errors.Annotate(err, "getting object-store draining phase")
			}
			if err := w.syncGuard(ctx, phase); err != nil {
				return errors.Trace(err)
			}

		}
	}
}

func (w *Worker) syncGuard(ctx context.Context, phase objectstore.Phase) error {
	switch phase {
	case objectstore.PhaseUnknown, objectstore.PhaseCompleted:
		w.config.Logger.Infof(ctx, "object store is not draining, unlocking local guard")
		if err := w.config.Guard.Unlock(ctx); err != nil {
			return errors.Annotate(err, "unlocking local object-store guard")
		}
		return nil
	case objectstore.PhaseDraining:
		w.config.Logger.Infof(ctx, "object store is draining, locking local guard")
		if err := w.config.Guard.Lockdown(ctx); err != nil {
			return errors.Annotate(err, "locking local object-store guard")
		}
		return nil
	case objectstore.PhaseError:
		w.config.Logger.Errorf(ctx, "object store draining is in an error state, keeping local guard locked")
		if err := w.config.Guard.Lockdown(ctx); err != nil {
			return errors.Annotate(err, "keeping local object-store guard locked")
		}
		return nil
	default:
		return errors.Errorf("unknown object-store draining phase %q", phase)
	}
}
