// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstorepruner

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	coreobjectstore "github.com/juju/juju/core/objectstore"
)

// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...any)
	Warningf(message string, args ...any)
	Infof(message string, args ...any)
	Debugf(message string, args ...any)
	Tracef(message string, args ...any)

	IsTraceEnabled() bool
}

// ObjectStoreGetter is the interface that is used to get a object store.
type ObjectStoreGetter interface {
	// GetObjectStore returns a object store for the given namespace.
	GetObjectStore(context.Context, string) (coreobjectstore.ObjectStore, error)
}

// ManifoldConfig defines the configuration for the trace manifold.
type ManifoldConfig struct {
	ObjectStoreName    string
	ServiceFactoryName string

	Clock  clock.Clock
	Logger Logger
}

// Validate validates the manifold configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.ObjectStoreName == "" {
		return errors.NotValidf("empty ObjectStoreName")
	}
	if cfg.ServiceFactoryName == "" {
		return errors.NotValidf("empty ServiceFactoryName")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the trace worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.ObjectStoreName,
			config.ServiceFactoryName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			w, err := NewWorker(WorkerConfig{
				Clock:  config.Clock,
				Logger: config.Logger,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}

			return w, nil
		},
	}
}
