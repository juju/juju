// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modellife

import (
	"context"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
)

// Config is the configuration for the model life worker.
type Config struct {
	ModelUUID    model.UUID
	ModelService ModelService
}

// Validate validates the configuration.
func (c Config) Validate() error {
	if c.ModelUUID == "" {
		return errors.NotValidf("empty ModelUUID")
	}
	if c.ModelService == nil {
		return errors.NotValidf("nil ModelService")
	}
	return nil
}

// Worker is a worker that watches the model life of a model
// and notifies when the model life changes.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config

	mutex sync.Mutex
	life  life.Value
}

// NewWorker creates a new model life worker.
func NewWorker(ctx context.Context, config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &Worker{
		config: config,
	}

	// Read it before the worker starts, so that we have a value
	// guaranteed before we return the worker. Because we read this
	// before we start the internal watcher, we'll need an additional
	// read triggered by the first change event; this will *probably*
	// be the same value, but we can't assume it.
	var err error
	w.life, err = config.ModelService.GetModelLife(ctx, config.ModelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	err = catacomb.Invoke(catacomb.Plan{
		Name: "model-life",
		Site: &w.catacomb,
		Work: w.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
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
	w.mutex.Lock()
	defer w.mutex.Unlock()

	return life.IsNotDead(w.life)
}

func (w *Worker) loop() error {
	ctx := w.catacomb.Context(context.Background())

	modelUUID := w.config.ModelUUID
	modelService := w.config.ModelService

	watcher, err := modelService.WatchModel(ctx, modelUUID)
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
			// Get the current life value to ensure we have the latest state.
			// If the life value has changed, we will bounce.
			currentCheck := w.Check()
			if err := w.lifeChanged(ctx, modelUUID, currentCheck); err != nil {
				return err
			}
		}
	}
}

func (w *Worker) lifeChanged(ctx context.Context, modelUUID model.UUID, current bool) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	modelService := w.config.ModelService

	var err error
	w.life, err = modelService.GetModelLife(ctx, modelUUID)
	if errors.Is(err, modelerrors.NotFound) ||
		errors.Is(err, database.ErrDBDead) ||
		errors.Is(err, database.ErrDBNotFound) {
		w.life = life.Dead
		return dependency.ErrBounce
	} else if err != nil {
		return errors.Trace(err)
	}

	if life.IsNotDead(w.life) != current {
		return dependency.ErrBounce
	}
	return nil
}
