// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statemanager

import (
	"database/sql"

	"github.com/juju/errors"
	"github.com/juju/juju/worker/common"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"
)

type DBAccessor interface {
	GetDB(modelUUID string) (*sql.DB, error)
	GetExistingDB(modelUUID string) (*sql.DB, error)
}

type StateManager interface {
	GetStateManager(modelUUID string) error
}

// ManifoldConfig defines the names of the manifolds on which a Manifold will
// depend.
type ManifoldConfig struct {
	DBAccessorName string
}

// Manifold returns a dependency manifold that runs the statemanager
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DBAccessorName,
		},
		Output: dbAccessorOutput,
		Start: func(context dependency.Context) (worker.Worker, error) {
			var dbAccessor DBAccessor
			if err := context.Get(config.DBAccessorName, &dbAccessor); err != nil {
				return nil, err
			}

			cfg := WorkerConfig{}

			w, err := NewWorker(cfg)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}

func dbAccessorOutput(in worker.Worker, out interface{}) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	w, ok := in.(*stateManagerWorker)
	if !ok {
		return errors.Errorf("expected input of type dbWorker, got %T", in)
	}

	switch out := out.(type) {
	case *StateManager:
		var target StateManager = w
		*out = target
	default:
		return errors.Errorf("expected output of *statemanager.StateManager, got %T", out)
	}
	return nil
}
