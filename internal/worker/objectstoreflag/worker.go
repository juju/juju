// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoreflag

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher"
)

// ErrChanged indicates that a Worker has stopped because its
// Check result is no longer valid.
var ErrChanged = errors.New("objectstore flag value changed")

// Predicate defines a predicate.
type Predicate func(objectstore.Phase) bool

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
	ModelUUID          model.UUID
	ObjectStoreService ObjectStoreService
	Check              Predicate
}

// Validate returns an error if the config cannot be expected to
// drive a functional Worker.
func (config Config) Validate() error {
	if config.Check == nil {
		return errors.NotValidf("nil Check")
	}
	return nil
}

// NewWorker returns a Worker that tracks the result of the configured
// Check on the object store draining phase.
func NewWorker(ctx context.Context, config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	phase, err := config.ObjectStoreService.GetDrainingPhase(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{
		config: config,
		phase:  phase,
	}
	err = catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Worker implements worker.Worker and util.Flag, and exits
// with ErrChanged whenever the result of its configured Check of
// the Model's objectstore phase changes.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config
	phase    objectstore.Phase
}

// Kill is part of the worker.Worker interface.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

// Check is part of the util.Flag interface.
func (w *Worker) Check() bool {
	return w.config.Check(w.phase)
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
			if w.Check() != w.config.Check(phase) {
				return ErrChanged
			}
		}
	}
}

func (w *Worker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}
