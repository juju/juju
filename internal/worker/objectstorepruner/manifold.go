// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstorepruner

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	coredependency "github.com/juju/juju/core/dependency"
	coreobjectstore "github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/domain/model"
	"github.com/juju/juju/internal/servicefactory"
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

// ModelManagerService is the interface that is used to get a list of models.
type ModelManagerService interface {
	// ModelList returns a list of all model UUIDs.
	// The list of models returned are the ones that are just present in the
	// model manager list. This means that the model is not deleted.
	ModelList(ctx context.Context) ([]model.UUID, error)
}

// GetModelManagerServiceFunc is a helper function that gets a service from
// the manifold.
type GetModelManagerServiceFunc func(getter dependency.Getter, name string) (ModelManagerService, error)

// ManifoldConfig defines the configuration for the trace manifold.
type ManifoldConfig struct {
	ObjectStoreName    string
	ServiceFactoryName string

	Clock  clock.Clock
	Logger Logger

	GetModelManagerService GetModelManagerServiceFunc
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
	if cfg.GetModelManagerService == nil {
		return errors.NotValidf("nil GetModelManagerService")
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

			modelManagerService, err := config.GetModelManagerService(getter, config.ServiceFactoryName)
			if err != nil {
				return nil, errors.Trace(err)
			}

			w, err := NewWorker(WorkerConfig{
				ModelManagerService: modelManagerService,
				Clock:               config.Clock,
				Logger:              config.Logger,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}

			return w, nil
		},
	}
}

// GetModelManagerService is a helper function that gets a service from the
// manifold.
func GetModelManagerService(getter dependency.Getter, name string) (ModelManagerService, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory servicefactory.ControllerServiceFactory) ModelManagerService {
		return factory.ModelManager()
	})
}
