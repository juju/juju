// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicefactory

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	coredependency "github.com/juju/juju/core/dependency"
	"github.com/juju/juju/domain/model"
	domainservicefactory "github.com/juju/juju/domain/servicefactory"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/internal/worker/common"
	workerstate "github.com/juju/juju/internal/worker/state"
)

// Logger represents the logging methods called.
type Logger interface {
	Tracef(string, ...interface{})
	Debugf(message string, args ...any)
	Warningf(message string, args ...any)
	Child(string) Logger
}

// EnvironConfigFunc is a function that returns an environ configuration.
type EnvironConfigFunc func(newEnvironFunc NewEnvironFunc, systemState SystemState) EnvironConfig

// GetSystemState is a helper function that gets a system state from the
// dependency engine.
type GetSystemStateFunc func(getter dependency.Getter, name string) (SystemState, error)

// ManifoldConfig holds the information necessary to run a service factory
// worker in a dependency.Engine.
type ManifoldConfig struct {
	StateName                   string
	DBAccessorName              string
	ChangeStreamName            string
	Logger                      Logger
	NewWorker                   func(Config) (worker.Worker, error)
	NewServiceFactoryGetter     ServiceFactoryGetterFn
	NewControllerServiceFactory ControllerServiceFactoryFn
	NewModelServiceFactory      ModelServiceFactoryFn
	NewEnvironConfig            EnvironConfigFunc
	NewEnviron                  NewEnvironFunc
	GetSystemState              GetSystemStateFunc
}

// ServiceFactoryGetterFn is a function that returns a service factory getter.
type ServiceFactoryGetterFn func(
	servicefactory.ControllerServiceFactory,
	changestream.WatchableDBGetter,
	domainservicefactory.EnvironFactory,
	Logger,
	ModelServiceFactoryFn,
) servicefactory.ServiceFactoryGetter

// ControllerServiceFactoryFn is a function that returns a controller service
// factory.
type ControllerServiceFactoryFn func(
	changestream.WatchableDBGetter,
	coredatabase.DBDeleter,
	Logger,
) servicefactory.ControllerServiceFactory

// ModelServiceFactoryFn is a function that returns a model service factory.
type ModelServiceFactoryFn func(
	model.UUID,
	changestream.WatchableDBGetter,
	domainservicefactory.EnvironFactory,
	Logger,
) servicefactory.ModelServiceFactory

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.DBAccessorName == "" {
		return errors.NotValidf("empty DBAccessorName")
	}
	if config.ChangeStreamName == "" {
		return errors.NotValidf("empty ChangeStreamName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewServiceFactoryGetter == nil {
		return errors.NotValidf("nil NewServiceFactoryGetter")
	}
	if config.NewControllerServiceFactory == nil {
		return errors.NotValidf("nil NewControllerServiceFactory")
	}
	if config.NewModelServiceFactory == nil {
		return errors.NotValidf("nil NewModelServiceFactory")
	}
	if config.NewEnvironConfig == nil {
		return errors.NotValidf("nil NewEnvironConfig")
	}
	if config.NewEnviron == nil {
		return errors.NotValidf("nil NewEnviron")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run an apiserver
// worker. The manifold outputs an *apiserverhttp.Mux, for other workers
// to register handlers against.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.StateName,
			config.ChangeStreamName,
			config.DBAccessorName,
		},
		Start:  config.start,
		Output: config.output,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	systemState, err := config.GetSystemState(getter, config.StateName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var dbGetter changestream.WatchableDBGetter
	if err := getter.Get(config.ChangeStreamName, &dbGetter); err != nil {
		_ = systemState.Release()
		return nil, errors.Trace(err)
	}

	var dbDeleter coredatabase.DBDeleter
	if err := getter.Get(config.DBAccessorName, &dbDeleter); err != nil {
		_ = systemState.Release()
		return nil, errors.Trace(err)
	}

	w, err := config.NewWorker(Config{
		DBGetter:  dbGetter,
		DBDeleter: dbDeleter,
		Logger:    config.Logger,

		NewServiceFactoryGetter:     config.NewServiceFactoryGetter,
		NewControllerServiceFactory: config.NewControllerServiceFactory,
		NewModelServiceFactory:      config.NewModelServiceFactory,

		// Create a new environ config that can be used to create the
		// environ once we have the controller service factory.
		EnvironConfig: config.NewEnvironConfig(config.NewEnviron, systemState),
	})
	if err != nil {
		_ = systemState.Release()
		return nil, errors.Trace(err)
	}

	return common.NewCleanupWorker(w, func() {
		_ = systemState.Release()
	}), nil
}

func (config ManifoldConfig) output(in worker.Worker, out any) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Unwrap()
	}
	w, ok := in.(*serviceFactoryWorker)
	if !ok {
		return errors.Errorf("expected input of type dbWorker, got %T", in)
	}

	switch out := out.(type) {
	case *servicefactory.ControllerServiceFactory:
		var target = w.ControllerFactory()
		*out = target
	case *servicefactory.ServiceFactoryGetter:
		var target = w.FactoryGetter()
		*out = target
	default:
		return errors.Errorf("unsupported output type %T", out)
	}
	return nil
}

// NewControllerServiceFactory returns a new controller service factory.
func NewControllerServiceFactory(
	dbGetter changestream.WatchableDBGetter,
	dbDeleter coredatabase.DBDeleter,
	logger Logger,
) servicefactory.ControllerServiceFactory {
	return domainservicefactory.NewControllerFactory(
		changestream.NewWatchableDBFactoryForNamespace(dbGetter.GetWatchableDB, coredatabase.ControllerNS),
		dbDeleter,
		serviceFactoryLogger{
			Logger: logger,
		},
	)
}

// NewModelServiceFactory returns a new model service factory.
func NewModelServiceFactory(
	modelUUID model.UUID,
	dbGetter changestream.WatchableDBGetter,
	environFactory domainservicefactory.EnvironFactory,
	logger Logger,
) servicefactory.ModelServiceFactory {
	return domainservicefactory.NewModelFactory(
		changestream.NewWatchableDBFactoryForNamespace(dbGetter.GetWatchableDB, string(modelUUID)),
		environFactory,
		serviceFactoryLogger{
			Logger: logger,
		},
	)
}

// NewServiceFactoryGetter returns a new service factory getter.
func NewServiceFactoryGetter(
	ctrlFactory servicefactory.ControllerServiceFactory,
	dbGetter changestream.WatchableDBGetter,
	environFactory domainservicefactory.EnvironFactory,
	logger Logger,
	newModelServiceFactory ModelServiceFactoryFn,
) servicefactory.ServiceFactoryGetter {
	return &serviceFactoryGetter{
		ctrlFactory:            ctrlFactory,
		dbGetter:               dbGetter,
		logger:                 logger,
		environFactory:         environFactory,
		newModelServiceFactory: newModelServiceFactory,
	}
}

// GetSystemState returns a system state from the dependency engine.
func GetSystemState(getter dependency.Getter, name string) (SystemState, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory servicefactory.ControllerServiceFactory) (SystemState, error) {
		var stTracker workerstate.StateTracker
		if err := getter.Get(name, &stTracker); err != nil {
			return nil, errors.Trace(err)
		}

		// Get the state pool after grabbing dependencies so we don't need
		// to remember to call Done on it if they're not running yet.
		statePool, _, err := stTracker.Use()
		if err != nil {
			return nil, errors.Trace(err)
		}

		systemState, err := statePool.SystemState()
		if err != nil {
			_ = stTracker.Done()
			return nil, errors.Trace(err)
		}
		return stateShim{
			State:    systemState,
			releaser: stTracker.Done,
		}, nil
	})
}
