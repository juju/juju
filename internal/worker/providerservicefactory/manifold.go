// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providerservicefactory

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
	Infof(message string, args ...any)
	Warningf(message string, args ...any)
	Errorf(message string, args ...any)
}

// ManifoldConfig holds the information necessary to run a provider service
// factory worker in a dependency.Engine.
type ManifoldConfig struct {
	ChangeStreamName string
	Logger           Logger
	NewWorker        func(Config) (worker.Worker, error)

	// NewProviderServiceFactoryGetter returns a new provider service factory
	// getter, to select a provider service factory per model UUID.
	NewProviderServiceFactoryGetter ProviderServiceFactoryGetterFn

	// NewProviderServiceFactory returns a new provider service factory for
	// the given model UUID.
	NewProviderServiceFactory ProviderServiceFactoryFn
}

// ProviderServiceFactoryGetterFn is a function that returns a provider service
// factory getter.
type ProviderServiceFactoryGetterFn func(
	ProviderServiceFactoryFn,
	changestream.WatchableDBGetter,
	Logger,
) servicefactory.ProviderServiceFactoryGetter

// ProviderServiceFactoryFn is a function that returns a provider service
// factory.
type ProviderServiceFactoryFn func(
	coremodel.UUID,
	changestream.WatchableDBGetter,
	Logger,
) servicefactory.ProviderServiceFactory

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.ChangeStreamName == "" {
		return errors.NotValidf("empty ChangeStreamName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewProviderServiceFactoryGetter == nil {
		return errors.NotValidf("nil NewProviderServiceFactoryGetter")
	}
	if config.NewProviderServiceFactory == nil {
		return errors.NotValidf("nil NewProviderServiceFactory")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run an provider service.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.ChangeStreamName,
		},
		Start:  config.start,
		Output: config.output,
	}
}

func (config ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var dbGetter changestream.WatchableDBGetter
	if err := getter.Get(config.ChangeStreamName, &dbGetter); err != nil {
		return nil, errors.Trace(err)
	}

	return config.NewWorker(Config{
		DBGetter:                        dbGetter,
		Logger:                          config.Logger,
		NewProviderServiceFactoryGetter: config.NewProviderServiceFactoryGetter,
		NewProviderServiceFactory:       config.NewProviderServiceFactory,
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
	case *servicefactory.ProviderServiceFactoryGetter:
		*out = w.factoryGetter
	default:
		return errors.Errorf("unsupported output type %T", out)
	}
	return nil
}

// NewProviderServiceFactoryGetter returns a new service factory getter.
func NewProviderServiceFactoryGetter(
	newProviderServiceFactory ProviderServiceFactoryFn,
	dbGetter changestream.WatchableDBGetter,
	logger Logger,
) servicefactory.ProviderServiceFactoryGetter {
	return &serviceFactoryGetter{
		newProviderServiceFactory: newProviderServiceFactory,
		dbGetter:                  dbGetter,
		logger:                    logger,
	}
}

// NewProviderServiceFactory returns a new provider service factory.
func NewProviderServiceFactory(
	modelUUID coremodel.UUID,
	dbGetter changestream.WatchableDBGetter,
	logger Logger,
) servicefactory.ProviderServiceFactory {
	return domainservicefactory.NewProviderFactory(
		changestream.NewWatchableDBFactoryForNamespace(dbGetter.GetWatchableDB, coredatabase.ControllerNS),
		changestream.NewWatchableDBFactoryForNamespace(dbGetter.GetWatchableDB, modelUUID.String()),
		serviceFactoryLogger{
			Logger: logger,
		},
	)
}
