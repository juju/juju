// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicefactory

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/servicefactory"
	"github.com/juju/juju/worker/common"
)

// Logger represents the logging methods called.
type Logger interface {
	Debugf(message string, args ...any)
	Child(string) Logger
}

// ManifoldConfig holds the information necessary to run a service factory
// worker in a dependency.Engine.
type ManifoldConfig struct {
	DBAccessorName              string
	ChangeStreamName            string
	Logger                      Logger
	NewWorker                   func(Config) (worker.Worker, error)
	NewServiceFactoryGetter     ServiceFactoryGetterFn
	NewControllerServiceFactory ControllerServiceFactoryFn
	NewModelServiceFactory      ModelServiceFactoryFn
}

// ServiceFactoryGetterFn is a function that returns a service factory getter.
type ServiceFactoryGetterFn func(
	ControllerServiceFactory,
	changestream.WatchableDBGetter,
	Logger,
	ModelServiceFactoryFn,
) ServiceFactoryGetter

type ControllerServiceFactoryFn func(
	changestream.WatchableDBGetter,
	coredatabase.DBDeleter,
	Logger,
) ControllerServiceFactory

// ModelServiceFactoryFn is a function that returns a model service factory.
type ModelServiceFactoryFn func(
	changestream.WatchableDBGetter,
	string,
	Logger,
) ModelServiceFactory

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
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
			config.ChangeStreamName,
			config.DBAccessorName,
		},
		Start:  config.start,
		Output: config.output,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var dbGetter changestream.WatchableDBGetter
	if err := context.Get(config.ChangeStreamName, &dbGetter); err != nil {
		return nil, errors.Trace(err)
	}

	var dbDeleter coredatabase.DBDeleter
	if err := context.Get(config.DBAccessorName, &dbDeleter); err != nil {
		return nil, errors.Trace(err)
	}

	return config.NewWorker(Config{
		DBGetter:                    dbGetter,
		DBDeleter:                   dbDeleter,
		Logger:                      config.Logger,
		NewServiceFactoryGetter:     config.NewServiceFactoryGetter,
		NewControllerServiceFactory: config.NewControllerServiceFactory,
		NewModelServiceFactory:      config.NewModelServiceFactory,
	})
}

func (config ManifoldConfig) output(in worker.Worker, out any) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	w, ok := in.(*serviceFactoryWorker)
	if !ok {
		return errors.Errorf("expected input of type dbWorker, got %T", in)
	}

	switch out := out.(type) {
	case *ControllerServiceFactory:
		var target ControllerServiceFactory = w.ctrlFactory
		*out = target
	case *ServiceFactoryGetter:
		var target ServiceFactoryGetter = w.factoryGetter
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
) ControllerServiceFactory {
	return servicefactory.NewControllerFactory(
		changestream.NewWatchableDBFactoryForNamespace(dbGetter.GetWatchableDB, coredatabase.ControllerNS),
		dbDeleter,
		serviceFactoryLogger{
			Logger: logger,
		},
	)
}

// NewModelServiceFactory returns a new model service factory.
func NewModelServiceFactory(dbGetter changestream.WatchableDBGetter, modelUUID string, logger Logger) ModelServiceFactory {
	return servicefactory.NewModelFactory(
		changestream.NewWatchableDBFactoryForNamespace(dbGetter.GetWatchableDB, modelUUID),
		serviceFactoryLogger{
			Logger: logger,
		},
	)
}

// NewServiceFactoryGetter returns a new service factory getter.
func NewServiceFactoryGetter(
	ctrlFactory ControllerServiceFactory,
	dbGetter changestream.WatchableDBGetter,
	logger Logger,
	newModelServiceFactory ModelServiceFactoryFn,
) ServiceFactoryGetter {
	return &serviceFactoryGetter{
		ctrlFactory:            ctrlFactory,
		dbGetter:               dbGetter,
		logger:                 logger,
		newModelServiceFactory: newModelServiceFactory,
	}
}

type serviceFactoryGetter struct {
	ctrlFactory            ControllerServiceFactory
	dbGetter               changestream.WatchableDBGetter
	logger                 Logger
	newModelServiceFactory ModelServiceFactoryFn
}

// FactoryForModel returns a service factory for the given model uuid.
// This will late bind the model service factory to the actual service factory.
func (s *serviceFactoryGetter) FactoryForModel(modelUUID string) ServiceFactory {
	// At the moment the model service factory is not cached, and is created
	// on demand. We could cache it here, but then it's not clear when to clear
	// the cache. Given that the model service factory is cheap to create, we
	// can just create it on demand and then look into some sort of finalizer
	// to clear the cache at a later point.
	return &serviceFactory{
		ControllerServiceFactory: s.ctrlFactory,
		ModelServiceFactory:      s.newModelServiceFactory(s.dbGetter, modelUUID, s.logger),
	}
}

type serviceFactory struct {
	ControllerServiceFactory
	ModelServiceFactory
}
