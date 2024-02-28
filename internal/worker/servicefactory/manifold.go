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
	coremodel "github.com/juju/juju/core/model"
	domainservicefactory "github.com/juju/juju/domain/servicefactory"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/internal/worker/common"
)

// Logger represents the logging methods called.
type Logger interface {
	Tracef(string, ...interface{})
	Debugf(message string, args ...any)
	Warningf(message string, args ...any)
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
	servicefactory.ControllerServiceFactory,
	changestream.WatchableDBGetter,
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
	coremodel.UUID,
	changestream.WatchableDBGetter,
	Logger,
) servicefactory.ModelServiceFactory

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
func (config ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var dbGetter changestream.WatchableDBGetter
	if err := getter.Get(config.ChangeStreamName, &dbGetter); err != nil {
		return nil, errors.Trace(err)
	}

	var dbDeleter coredatabase.DBDeleter
	if err := getter.Get(config.DBAccessorName, &dbDeleter); err != nil {
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
	modelUUID coremodel.UUID,
	dbGetter changestream.WatchableDBGetter,
	logger Logger,
) servicefactory.ModelServiceFactory {
	return domainservicefactory.NewModelFactory(
		changestream.NewWatchableDBFactoryForNamespace(dbGetter.GetWatchableDB, modelUUID.String()),
		serviceFactoryLogger{
			Logger: logger,
		},
	)
}

// NewServiceFactoryGetter returns a new service factory getter.
func NewServiceFactoryGetter(
	ctrlFactory servicefactory.ControllerServiceFactory,
	dbGetter changestream.WatchableDBGetter,
	logger Logger,
	newModelServiceFactory ModelServiceFactoryFn,
) servicefactory.ServiceFactoryGetter {
	return &serviceFactoryGetter{
		ctrlFactory:            ctrlFactory,
		dbGetter:               dbGetter,
		logger:                 logger,
		newModelServiceFactory: newModelServiceFactory,
	}
}
