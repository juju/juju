// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domainservices

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/providertracker"
	domainservicefactory "github.com/juju/juju/domain/services"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/common"
)

// ManifoldConfig holds the information necessary to run a domain services
// worker in a dependency.Engine.
type ManifoldConfig struct {
	DBAccessorName              string
	ChangeStreamName            string
	ProviderFactoryName         string
	ObjectStoreName             string
	Logger                      logger.Logger
	NewWorker                   func(Config) (worker.Worker, error)
	NewDomainServicesGetter     DomainServicesGetterFn
	NewControllerDomainServices ControllerDomainServicesFn
	NewModelDomainServices      ModelDomainServicesFn
}

// DomainServicesGetterFn is a function that returns a domain services getter.
type DomainServicesGetterFn func(
	services.ControllerDomainServices,
	changestream.WatchableDBGetter,
	logger.Logger,
	ModelDomainServicesFn,
	providertracker.ProviderFactory,
	objectstore.ObjectStoreGetter,
) services.DomainServicesGetter

// ControllerDomainServicesFn is a function that returns a controller service
// factory.
type ControllerDomainServicesFn func(
	changestream.WatchableDBGetter,
	coredatabase.DBDeleter,
	logger.Logger,
) services.ControllerDomainServices

// ModelDomainServicesFn is a function that returns a model domain services.
type ModelDomainServicesFn func(
	coremodel.UUID,
	changestream.WatchableDBGetter,
	providertracker.ProviderFactory,
	objectstore.ModelObjectStoreGetter,
	logger.Logger,
) services.ModelDomainServices

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.DBAccessorName == "" {
		return errors.NotValidf("empty DBAccessorName")
	}
	if config.ChangeStreamName == "" {
		return errors.NotValidf("empty ChangeStreamName")
	}
	if config.ProviderFactoryName == "" {
		return errors.NotValidf("empty ProviderFactoryName")
	}
	if config.ObjectStoreName == "" {
		return errors.NotValidf("empty ObjectStoreName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewDomainServicesGetter == nil {
		return errors.NotValidf("nil NewDomainServicesGetter")
	}
	if config.NewControllerDomainServices == nil {
		return errors.NotValidf("nil NewControllerDomainServices")
	}
	if config.NewModelDomainServices == nil {
		return errors.NotValidf("nil NewModelDomainServices")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run a domain services
// worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.ChangeStreamName,
			config.DBAccessorName,
			config.ProviderFactoryName,
			config.ObjectStoreName,
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

	var providerFactory providertracker.ProviderFactory
	if err := getter.Get(config.ProviderFactoryName, &providerFactory); err != nil {
		return nil, errors.Trace(err)
	}

	var objectStoreGetter objectstore.ObjectStoreGetter
	if err := getter.Get(config.ObjectStoreName, &objectStoreGetter); err != nil {
		return nil, errors.Trace(err)
	}

	return config.NewWorker(Config{
		DBGetter:                    dbGetter,
		DBDeleter:                   dbDeleter,
		ProviderFactory:             providerFactory,
		ObjectStoreGetter:           objectStoreGetter,
		Logger:                      config.Logger,
		NewDomainServicesGetter:     config.NewDomainServicesGetter,
		NewControllerDomainServices: config.NewControllerDomainServices,
		NewModelDomainServices:      config.NewModelDomainServices,
	})
}

func (config ManifoldConfig) output(in worker.Worker, out any) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	w, ok := in.(*domainServicesWorker)
	if !ok {
		return errors.Errorf("expected input of type domainServicesWorker, got %T", in)
	}

	switch out := out.(type) {
	case *services.ControllerDomainServices:
		var target = w.ControllerServices()
		*out = target
	case *services.DomainServicesGetter:
		var target = w.ServicesGetter()
		*out = target
	default:
		return errors.Errorf("unsupported output type %T", out)
	}
	return nil
}

// NewControllerDomainServices returns a new controller domain services.
func NewControllerDomainServices(
	dbGetter changestream.WatchableDBGetter,
	dbDeleter coredatabase.DBDeleter,
	logger logger.Logger,
) services.ControllerDomainServices {
	return domainservicefactory.NewControllerServices(
		changestream.NewWatchableDBFactoryForNamespace(dbGetter.GetWatchableDB, coredatabase.ControllerNS),
		dbDeleter,
		logger,
	)
}

// NewProviderTrackerModelDomainServices returns a new model domain services
// with a provider tracker.
func NewProviderTrackerModelDomainServices(
	modelUUID coremodel.UUID,
	dbGetter changestream.WatchableDBGetter,
	providerFactory providertracker.ProviderFactory,
	objectStore objectstore.ModelObjectStoreGetter,
	logger logger.Logger,
) services.ModelDomainServices {
	return domainservicefactory.NewModelFactory(
		modelUUID,
		changestream.NewWatchableDBFactoryForNamespace(dbGetter.GetWatchableDB, coredatabase.ControllerNS),
		changestream.NewWatchableDBFactoryForNamespace(dbGetter.GetWatchableDB, modelUUID.String()),
		providerFactory,
		objectStore,
		logger,
	)
}

// NewModelDomainServices returns a new model domain services.
// This creates a model domain services without a provider tracker. The provider
// tracker will return not supported errors for all methods.
func NewModelDomainServices(
	modelUUID coremodel.UUID,
	dbGetter changestream.WatchableDBGetter,
	objectStore objectstore.ModelObjectStoreGetter,
	logger logger.Logger,
) services.ModelDomainServices {
	return domainservicefactory.NewModelFactory(
		modelUUID,
		changestream.NewWatchableDBFactoryForNamespace(dbGetter.GetWatchableDB, coredatabase.ControllerNS),
		changestream.NewWatchableDBFactoryForNamespace(dbGetter.GetWatchableDB, modelUUID.String()),
		NoopProviderFactory{},
		objectStore,
		logger,
	)
}

// NewDomainServicesGetter returns a new domain services getter.
func NewDomainServicesGetter(
	ctrlFactory services.ControllerDomainServices,
	dbGetter changestream.WatchableDBGetter,
	logger logger.Logger,
	newModelDomainServices ModelDomainServicesFn,
	providerFactory providertracker.ProviderFactory,
	objectStoreGetter objectstore.ObjectStoreGetter,
) services.DomainServicesGetter {
	return &domainServicesGetter{
		ctrlFactory:            ctrlFactory,
		dbGetter:               dbGetter,
		logger:                 logger,
		newModelDomainServices: newModelDomainServices,
		providerFactory:        providerFactory,
		objectStoreGetter:      objectStoreGetter,
	}
}

type NoopProviderFactory struct{}

func (NoopProviderFactory) ProviderForModel(ctx context.Context, namespace string) (providertracker.Provider, error) {
	return nil, errors.NotSupportedf("provider")
}
