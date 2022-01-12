// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statemanager

import (
	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1/catacomb"
)

// WorkerConfig encapsulates the configuration options for the
// statemanager worker.
type WorkerConfig struct {
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	return nil
}

type stateManagerWorker struct {
	cfg      WorkerConfig
	catacomb catacomb.Catacomb
}

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

func (w *stateManagerWorker) GetStateManager(modelUUID string) error {
	return nil
}
