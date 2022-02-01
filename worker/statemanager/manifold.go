// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statemanager

import (
	"database/sql"

	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/dbaccessor"
)

type DBAccessor interface {
	GetDB(modelUUID string) (*sql.DB, error)
	GetExistingDB(modelUUID string) (*sql.DB, error)
}

type StateManagerProvider interface {
	GetStateManager(namespace string) (Overlord, error)
}

// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...interface{})
}

// ManifoldConfig defines the names of the manifolds on which a Manifold will
// depend.
type ManifoldConfig struct {
	DBAccessorName string
	Logger         Logger
}

// Manifold returns a dependency manifold that runs the statemanager
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DBAccessorName,
		},
		Output: stateManagerOutput,
		Start: func(context dependency.Context) (worker.Worker, error) {
			var dbAccessor dbaccessor.DBGetter
			if err := context.Get(config.DBAccessorName, &dbAccessor); err != nil {
				return nil, err
			}

			cfg := WorkerConfig{
				DBAccessor: dbAccessor,
				Logger:     config.Logger,
			}

			w, err := NewWorker(cfg)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}

func stateManagerOutput(in worker.Worker, out interface{}) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	w, ok := in.(*stateManagerWorker)
	if !ok {
		return errors.Errorf("expected input of type dbWorker, got %T", in)
	}

	switch out := out.(type) {
	case *StateManagerProvider:
		var target StateManagerProvider = w
		*out = target
	default:
		return errors.Errorf("expected output of *statemanager.StateManagerProvider, got %T", out)
	}
	return nil
}
