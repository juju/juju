// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsinkservices

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

	// NewLogSinkServicesGetter returns a new provider domain services
	// getter, to select a provider domain services per model UUID.
	NewLogSinkServicesGetter LogSinkServicesGetterFn

	// NewLogSinkServices returns a new provider domain services for
	// the given model UUID.
	NewLogSinkServices LogSinkServicesFn
}

// LogSinkServicesGetterFn is a function that returns a provider service
// factory getter.
type LogSinkServicesGetterFn func(
	LogSinkServicesFn,
	changestream.WatchableDBGetter,
	logger.Logger,
) services.LogSinkServicesGetter

// LogSinkServicesFn is a function that returns a provider service
// factory.
type LogSinkServicesFn func(
	coremodel.UUID,
	changestream.WatchableDBGetter,
	logger.Logger,
) services.LogSinkServices

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.ChangeStreamName == "" {
		return errors.NotValidf("empty ChangeStreamName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewLogSinkServicesGetter == nil {
		return errors.NotValidf("nil NewLogSinkServicesGetter")
	}
	if config.NewLogSinkServices == nil {
		return errors.NotValidf("nil NewLogSinkServices")
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
		DBGetter:                 dbGetter,
		Logger:                   config.Logger,
		NewLogSinkServicesGetter: config.NewLogSinkServicesGetter,
		NewLogSinkServices:       config.NewLogSinkServices,
	})
}

func (config ManifoldConfig) output(in worker.Worker, out any) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	w, ok := in.(*servicesWorker)
	if !ok {
		return errors.Errorf("expected input of type dbWorker, got %T", in)
	}

	switch out := out.(type) {
	case *services.LogSinkServicesGetter:
		*out = w.servicesGetter
	case *services.ControllerLogSinkServices:
		*out = w.ControllerServices()
	default:
		return errors.Errorf("unsupported output type %T", out)
	}
	return nil
}

// NewLogSinkServicesGetter returns a new domain services getter.
func NewLogSinkServicesGetter(
	newLogSinkServices LogSinkServicesFn,
	dbGetter changestream.WatchableDBGetter,
	logger logger.Logger,
) services.LogSinkServicesGetter {
	return &domainServicesGetter{
		newLogSinkServices: newLogSinkServices,
		dbGetter:           dbGetter,
		logger:             logger,
	}
}

// NewLogSinkServices returns a new provider domain services.
func NewLogSinkServices(
	modelUUID coremodel.UUID,
	dbGetter changestream.WatchableDBGetter,
	logger logger.Logger,
) services.LogSinkServices {
	return domainservicefactory.NewLogSinkServices(
		changestream.NewWatchableDBFactoryForNamespace(dbGetter.GetWatchableDB, coredatabase.ControllerNS),
		changestream.NewWatchableDBFactoryForNamespace(dbGetter.GetWatchableDB, modelUUID.String()),
		logger,
	)
}
