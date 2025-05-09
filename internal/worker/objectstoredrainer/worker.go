// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoredrainer

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher"
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
	return nil
}

// NewWorker returns a Worker that tracks the result of the configured.
func NewWorker(ctx context.Context, config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{
		config: config,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Worker implements
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config
}

// Kill is part of the worker.Worker interface.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

func (w *Worker) loop() error {
	ctx, cancel := w.scopedContext()
	defer cancel()

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
				if err := w.config.Guard.Unlock(); err != nil {
					return errors.Errorf("failed to update guard: %v", err)
				}
				continue
			}

			// TODO (stickupkid): Handle the draining phase.
			if err := w.config.Guard.Lockdown(ctx.Done()); err != nil {
				return errors.Errorf("failed to update guard: %v", err)
			}
		}
	}
}

func (w *Worker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}
