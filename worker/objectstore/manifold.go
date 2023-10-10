// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/state"
	"github.com/juju/mgo/v3"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	coreobjectstore "github.com/juju/juju/core/objectstore"
)

// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...any)
	Warningf(message string, args ...any)
	Infof(message string, args ...any)
	Debugf(message string, args ...any)
	Tracef(message string, args ...any)
}

// ObjectStoreGetter is the interface that is used to get a object store.
type ObjectStoreGetter interface {
	// GetObjectStore returns a object store for the given namespace.
	GetObjectStore(context.Context, string) (coreobjectstore.ObjectStore, error)
}

// MongoSession is the interface that is used to get a mongo session.
// Deprecated: is only here for backwards compatibility.
type MongoSession interface {
	MongoSession() *mgo.Session
}

// ObjectStoreWorkerFunc is the function signature for creating a new object
// store worker.
type ObjectStoreWorkerFunc func(context.Context, string, MongoSession, Logger) (TrackedObjectStore, error)

// ManifoldConfig defines the configuration for the trace manifold.
type ManifoldConfig struct {
	AgentName            string
	Clock                clock.Clock
	Logger               Logger
	NewObjectStoreWorker ObjectStoreWorkerFunc

	// StateName is only here for backwards compatibility. Once we have
	// the right abstractions in place, and we have a replacement, we can
	// remove this.
	StateName string
}

// Validate validates the manifold configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.NewObjectStoreWorker == nil {
		return errors.NotValidf("nil NewObjectStoreWorker")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the trace worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.StateName,
		},
		Output: output,
		Start: func(context dependency.Context) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var a agent.Agent
			if err := context.Get(config.AgentName, &a); err != nil {
				return nil, err
			}

			var stTracker state.StateTracker
			if err := context.Get(config.StateName, &stTracker); err != nil {
				return nil, errors.Trace(err)
			}

			// Get the state pool after grabbing dependencies so we don't need
			// to remember to call Done on it if they're not running yet.
			statePool, _, err := stTracker.Use()
			if err != nil {
				return nil, errors.Trace(err)
			}

			w, err := NewWorker(WorkerConfig{
				Clock:                config.Clock,
				Logger:               config.Logger,
				NewObjectStoreWorker: config.NewObjectStoreWorker,

				// StatePool is only here for backwards compatibility. Once we
				// have the right abstractions in place, and we have a
				// replacement, we can remove this.
				StatePool: statePool,
			})
			if err != nil {
				_ = stTracker.Done()
				return nil, errors.Trace(err)
			}

			return common.NewCleanupWorker(w, func() {
				// Ensure we clean up the state pool.
				_ = stTracker.Done()
			}), nil
		},
	}
}

func output(in worker.Worker, out any) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	w, ok := in.(*objectStoreWorker)
	if !ok {
		return errors.Errorf("expected input of objectStoreWorker, got %T", in)
	}

	switch out := out.(type) {
	case *ObjectStoreGetter:
		var target ObjectStoreGetter = w
		*out = target
	default:
		return errors.Errorf("expected output of Tracer, got %T", out)
	}
	return nil
}
