// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statemanager

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/overlord"
	"github.com/juju/juju/overlord/state"
)

// Overlord defines the various managers that are available for the whole
// state.
type Overlord interface {
	LogManager() overlord.LogManager
}

// WorkerConfig encapsulates the configuration options for the
// statemanager worker.
type WorkerConfig struct {
	DBAccessor DBAccessor
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.DBAccessor == nil {
		return errors.NotValidf("missing DBAccessor")
	}
	return nil
}

type stateManagerWorker struct {
	cfg      WorkerConfig
	catacomb catacomb.Catacomb
}

// NewWorker creates a new state manager worker.
func NewWorker(cfg WorkerConfig) (*stateManagerWorker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &stateManagerWorker{
		cfg: cfg,
	}

	if err = catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

func (w *stateManagerWorker) loop() (err error) {
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		}
	}
}

// Kill is part of the worker.Worker interface.
func (w *stateManagerWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *stateManagerWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *stateManagerWorker) GetStateManager(modelUUID string) (Overlord, error) {
	// TODO (stickupkid): Store the overlord, so that we can manage the managers
	// when the API server bounces.
	db, err := w.cfg.DBAccessor.GetDB(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return overlord.New(state.NewState(db))
}
