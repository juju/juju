// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoreservices

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

// ManifoldConfig holds the information necessary to run a object store services
// worker in a dependency.Engine.
type ManifoldConfig struct {
	ChangeStreamName string
	Logger           logger.Logger
	NewWorker        func(Config) (worker.Worker, error)

	// NewObjectStoreServicesGetter returns a new object store services
	// getter, to select a object store services per model UUID.
	NewObjectStoreServicesGetter ObjectStoreServicesGetterFn

	// NewObjectStoreServices returns a new object store services for
	// the given model UUID.
	NewObjectStoreServices ObjectStoreServicesFn
}

// ObjectStoreServicesGetterFn is a function that returns a object store
// services getter.
type ObjectStoreServicesGetterFn func(
	ObjectStoreServicesFn,
	changestream.WatchableDBGetter,
	logger.Logger,
) services.ObjectStoreServicesGetter

// ObjectStoreServicesFn is a function that returns a object store services.
type ObjectStoreServicesFn func(
	coremodel.UUID,
	changestream.WatchableDBGetter,
	logger.Logger,
) services.ObjectStoreServices

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.ChangeStreamName == "" {
		return errors.NotValidf("empty ChangeStreamName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewObjectStoreServicesGetter == nil {
		return errors.NotValidf("nil NewObjectStoreServicesGetter")
	}
	if config.NewObjectStoreServices == nil {
		return errors.NotValidf("nil NewObjectStoreServices")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run an object store service.
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
		DBGetter:                     dbGetter,
		Logger:                       config.Logger,
		NewObjectStoreServicesGetter: config.NewObjectStoreServicesGetter,
		NewObjectStoreServices:       config.NewObjectStoreServices,
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
	case *services.ObjectStoreServicesGetter:
		*out = w.ServicesGetter()

	case *services.ControllerObjectStoreServices:
		*out = w.ControllerServices()

	default:
		return errors.Errorf("unsupported output type %T", out)
	}
	return nil
}

// NewObjectStoreServicesGetter returns a new object store services getter.
func NewObjectStoreServicesGetter(
	newObjectStoreServices ObjectStoreServicesFn,
	dbGetter changestream.WatchableDBGetter,
	logger logger.Logger,
) services.ObjectStoreServicesGetter {
	return &domainServicesGetter{
		newObjectStoreServices: newObjectStoreServices,
		dbGetter:               dbGetter,
		logger:                 logger,
	}
}

// NewObjectStoreServices returns a new object store services.
func NewObjectStoreServices(
	modelUUID coremodel.UUID,
	dbGetter changestream.WatchableDBGetter,
	logger logger.Logger,
) services.ObjectStoreServices {
	return domainservicefactory.NewObjectStoreServices(
		changestream.NewWatchableDBFactoryForNamespace(dbGetter.GetWatchableDB, coredatabase.ControllerNS),
		changestream.NewWatchableDBFactoryForNamespace(dbGetter.GetWatchableDB, modelUUID.String()),
		logger,
	)
}
