// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providerservices

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	domainservicefactory "github.com/juju/juju/domain/services"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/common"
)

// ManifoldConfig holds the information necessary to run a provider service
// factory worker in a dependency.Engine.
type ManifoldConfig struct {
	ChangeStreamName string
	Logger           logger.Logger
	NewWorker        func(Config) (worker.Worker, error)

	// NewProviderServicesGetter returns a new provider domain services
	// getter, to select a provider domain services per model UUID.
	NewProviderServicesGetter ProviderServicesGetterFn

	// NewProviderServices returns a new provider domain services for
	// the given model UUID.
	NewProviderServices ProviderServicesFn
}

// ProviderServicesGetterFn is a function that returns a provider service
// factory getter.
type ProviderServicesGetterFn func(
	ProviderServicesFn,
	changestream.WatchableDBGetter,
	logger.Logger,
) services.ProviderServicesGetter

// ProviderServicesFn is a function that returns a provider service
// factory.
type ProviderServicesFn func(
	coremodel.UUID,
	changestream.WatchableDBGetter,
	logger.Logger,
) services.ProviderServices

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.ChangeStreamName == "" {
		return errors.NotValidf("empty ChangeStreamName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewProviderServicesGetter == nil {
		return errors.NotValidf("nil NewProviderServicesGetter")
	}
	if config.NewProviderServices == nil {
		return errors.NotValidf("nil NewProviderServices")
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
		DBGetter:                  dbGetter,
		Logger:                    config.Logger,
		NewProviderServicesGetter: config.NewProviderServicesGetter,
		NewProviderServices:       config.NewProviderServices,
	})
}

func (config ManifoldConfig) output(in worker.Worker, out any) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	w, ok := in.(*domainServicesWorker)
	if !ok {
		return errors.Errorf("expected input of type dbWorker, got %T", in)
	}

	switch out := out.(type) {
	case *services.ProviderServicesGetter:
		*out = w.servicesGetter
	default:
		return errors.Errorf("unsupported output type %T", out)
	}
	return nil
}

// NewProviderServicesGetter returns a new domain services getter.
func NewProviderServicesGetter(
	newProviderServices ProviderServicesFn,
	dbGetter changestream.WatchableDBGetter,
	logger logger.Logger,
) services.ProviderServicesGetter {
	return &domainServicesGetter{
		newProviderServices: newProviderServices,
		dbGetter:            dbGetter,
		logger:              logger,
	}
}

// NewProviderServices returns a new provider domain services.
func NewProviderServices(
	modelUUID coremodel.UUID,
	dbGetter changestream.WatchableDBGetter,
	logger logger.Logger,
) services.ProviderServices {
	return domainservicefactory.NewProviderServices(
		changestream.NewWatchableDBFactoryForNamespace(dbGetter.GetWatchableDB, coredatabase.ControllerNS),
		changestream.NewWatchableDBFactoryForNamespace(dbGetter.GetWatchableDB, modelUUID.String()),
		logger,
	)
}
