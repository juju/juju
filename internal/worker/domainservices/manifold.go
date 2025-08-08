// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domainservices

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	corehttp "github.com/juju/juju/core/http"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/storage"
	domainservices "github.com/juju/juju/domain/services"
	"github.com/juju/juju/internal/services"
	sshimporter "github.com/juju/juju/internal/ssh/importer"
	"github.com/juju/juju/internal/worker/common"
)

// ManifoldConfig holds the information necessary to run a domain services
// worker in a dependency.Engine.
type ManifoldConfig struct {
	ChangeStreamName            string
	ProviderFactoryName         string
	ObjectStoreName             string
	StorageRegistryName         string
	HTTPClientName              string
	LeaseManagerName            string
	LogSinkName                 string
	LogDir                      string
	Logger                      logger.Logger
	Clock                       clock.Clock
	NewWorker                   func(Config) (worker.Worker, error)
	NewDomainServicesGetter     DomainServicesGetterFn
	NewControllerDomainServices ControllerDomainServicesFn
	NewModelDomainServices      ModelDomainServicesFn
}

// DomainServicesGetterFn is a function that returns a domain services getter.
type DomainServicesGetterFn func(
	services.ControllerDomainServices,
	changestream.WatchableDBGetter,
	ModelDomainServicesFn,
	providertracker.ProviderFactory,
	objectstore.ObjectStoreGetter,
	storage.StorageRegistryGetter,
	domainservices.PublicKeyImporter,
	lease.Manager,
	string,
	clock.Clock,
	logger.LoggerContextGetter,
) services.DomainServicesGetter

// ControllerDomainServicesFn is a function that returns a controller service
// factory.
type ControllerDomainServicesFn func(
	changestream.WatchableDBGetter,
	objectstore.NamespacedObjectStoreGetter,
	clock.Clock,
	logger.Logger,
) services.ControllerDomainServices

// ModelDomainServicesFn is a function that returns a model domain services.
type ModelDomainServicesFn func(
	coremodel.UUID,
	changestream.WatchableDBGetter,
	providertracker.ProviderFactory,
	objectstore.ModelObjectStoreGetter,
	storage.ModelStorageRegistryGetter,
	domainservices.PublicKeyImporter,
	lease.ModelLeaseManagerGetter,
	string,
	clock.Clock,
	logger.Logger,
) services.ModelDomainServices

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.ChangeStreamName == "" {
		return errors.NotValidf("empty ChangeStreamName")
	}
	if config.ProviderFactoryName == "" {
		return errors.NotValidf("empty ProviderFactoryName")
	}
	if config.ObjectStoreName == "" {
		return errors.NotValidf("empty ObjectStoreName")
	}
	if config.StorageRegistryName == "" {
		return errors.NotValidf("empty StorageRegistryName")
	}
	if config.HTTPClientName == "" {
		return errors.NotValidf("empty HTTPClientName")
	}
	if config.LeaseManagerName == "" {
		return errors.NotValidf("empty LeaseManagerName")
	}
	if config.LogSinkName == "" {
		return errors.NotValidf("empty LogSinkName")
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
	if config.LogDir == "" {
		return errors.NotValidf("empty LogDir")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run a domain services
// worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.ChangeStreamName,
			config.ProviderFactoryName,
			config.ObjectStoreName,
			config.StorageRegistryName,
			config.HTTPClientName,
			config.LeaseManagerName,
			config.LogSinkName,
		},
		Start:  config.start,
		Output: config.output,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var dbGetter changestream.WatchableDBGetter
	if err := getter.Get(config.ChangeStreamName, &dbGetter); err != nil {
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

	var storageRegistryGetter storage.StorageRegistryGetter
	if err := getter.Get(config.StorageRegistryName, &storageRegistryGetter); err != nil {
		return nil, errors.Trace(err)
	}

	var httpClientGetter corehttp.HTTPClientGetter
	if err := getter.Get(config.HTTPClientName, &httpClientGetter); err != nil {
		return nil, errors.Trace(err)
	}

	sshImporterClient, err := httpClientGetter.GetHTTPClient(ctx, corehttp.SSHImporterPurpose)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var leaseManager lease.Manager
	if err := getter.Get(config.LeaseManagerName, &leaseManager); err != nil {
		return nil, errors.Trace(err)
	}

	var loggerContextGetter logger.LoggerContextGetter
	if err := getter.Get(config.LogSinkName, &loggerContextGetter); err != nil {
		return nil, errors.Trace(err)
	}

	return config.NewWorker(Config{
		DBGetter:                    dbGetter,
		ProviderFactory:             providerFactory,
		ObjectStoreGetter:           objectStoreGetter,
		StorageRegistryGetter:       storageRegistryGetter,
		PublicKeyImporter:           sshimporter.NewImporter(sshImporterClient),
		LeaseManager:                leaseManager,
		LoggerContextGetter:         loggerContextGetter,
		LogDir:                      config.LogDir,
		Logger:                      config.Logger,
		Clock:                       config.Clock,
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
	controllerObjectStoreGetter objectstore.NamespacedObjectStoreGetter,
	clock clock.Clock,
	logger logger.Logger,
) services.ControllerDomainServices {
	return domainservices.NewControllerServices(
		changestream.NewWatchableDBFactoryForNamespace(dbGetter.GetWatchableDB, coredatabase.ControllerNS),
		controllerObjectStoreGetter,
		clock,
		logger,
	)
}

// NewProviderTrackerModelDomainServices returns a new model domain services
// with a provider tracker.
func NewProviderTrackerModelDomainServices(
	modelUUID coremodel.UUID,
	dbGetter changestream.WatchableDBGetter,
	providerFactory providertracker.ProviderFactory,
	modelObjectStoreGetter objectstore.ModelObjectStoreGetter,
	storageRegistry storage.ModelStorageRegistryGetter,
	publicKeyImporter domainservices.PublicKeyImporter,
	leaseManager lease.ModelLeaseManagerGetter,
	logDir string,
	clock clock.Clock,
	logger logger.Logger,
) services.ModelDomainServices {
	return domainservices.NewModelServices(
		modelUUID,
		changestream.NewWatchableDBFactoryForNamespace(dbGetter.GetWatchableDB, coredatabase.ControllerNS),
		changestream.NewWatchableDBFactoryForNamespace(dbGetter.GetWatchableDB, modelUUID.String()),
		providerFactory,
		modelObjectStoreGetter,
		storageRegistry,
		publicKeyImporter,
		leaseManager,
		logDir,
		clock,
		logger,
	)
}

// NewDomainServicesGetter returns a new domain services getter.
func NewDomainServicesGetter(
	ctrlFactory services.ControllerDomainServices,
	dbGetter changestream.WatchableDBGetter,
	newModelDomainServices ModelDomainServicesFn,
	providerFactory providertracker.ProviderFactory,
	objectStoreGetter objectstore.ObjectStoreGetter,
	storageRegistryGetter storage.StorageRegistryGetter,
	publicKeyImporter domainservices.PublicKeyImporter,
	leaseManager lease.Manager,
	logDir string,
	clock clock.Clock,
	loggerContextGetter logger.LoggerContextGetter,
) services.DomainServicesGetter {
	return &domainServicesGetter{
		ctrlFactory:            ctrlFactory,
		dbGetter:               dbGetter,
		newModelDomainServices: newModelDomainServices,
		providerFactory:        providerFactory,
		objectStoreGetter:      objectStoreGetter,
		storageRegistryGetter:  storageRegistryGetter,
		publicKeyImporter:      publicKeyImporter,
		leaseManager:           leaseManager,
		logDir:                 logDir,
		clock:                  clock,
		loggerContextGetter:    loggerContextGetter,
	}
}

// NoopProviderFactory is a provider factory that returns not supported errors
// for all methods.
type NoopProviderFactory struct{}

// ProviderForModel returns a not supported error.
func (NoopProviderFactory) ProviderForModel(ctx context.Context, namespace string) (providertracker.Provider, error) {
	return nil, errors.NotSupportedf("provider")
}

// EphemeralProviderFromConfig returns a not supported error.
func (NoopProviderFactory) EphemeralProviderFromConfig(ctx context.Context, config providertracker.EphemeralProviderConfig) (providertracker.Provider, error) {
	return nil, errors.NotSupportedf("provider")
}
