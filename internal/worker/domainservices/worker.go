// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domainservices

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/storage"
	domainservices "github.com/juju/juju/domain/services"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
	internalstorage "github.com/juju/juju/internal/storage"
)

// Config is the configuration required for domain services worker.
type Config struct {
	// DBGetter supplies WatchableDB implementations by namespace.
	DBGetter changestream.WatchableDBGetter

	// ProviderFactory is used to get provider instances.
	ProviderFactory providertracker.ProviderFactory

	// ObjectStoreGetter is used to get object store instances.
	ObjectStoreGetter objectstore.ObjectStoreGetter

	// StorageRegistryGetter is used to get storage registry instances.
	StorageRegistryGetter storage.StorageRegistryGetter

	// PublicKeyImporter is used to import public keys.
	PublicKeyImporter domainservices.PublicKeyImporter

	// LeaseManager is used to manage leases.
	LeaseManager lease.Manager

	// LoggerContextGetter is used to get the logger context per model.
	LoggerContextGetter logger.LoggerContextGetter

	// LogDir is the directory where logs are stored.
	LogDir string

	// Logger is used to log messages.
	Logger logger.Logger

	// Clock is used to provides a main Clock
	Clock clock.Clock

	// NewDomainServicesGetter is used to get domain services for a model.
	NewDomainServicesGetter DomainServicesGetterFn

	// NewControllerDomainServices is used to get controller domain services.
	NewControllerDomainServices ControllerDomainServicesFn

	// NewModelDomainServices is used to get model domain services.
	NewModelDomainServices ModelDomainServicesFn
}

// Validate validates the domain services configuration.
func (config Config) Validate() error {
	if config.DBGetter == nil {
		return errors.NotValidf("nil DBGetter")
	}
	if config.ProviderFactory == nil {
		return errors.NotValidf("nil ProviderServices")
	}
	if config.ObjectStoreGetter == nil {
		return errors.NotValidf("nil ObjectStoreGetter")
	}
	if config.StorageRegistryGetter == nil {
		return errors.NotValidf("nil StorageRegistryGetter")
	}
	if config.PublicKeyImporter == nil {
		return errors.NotValidf("nil PublicKeyImporter")
	}
	if config.LeaseManager == nil {
		return errors.NotValidf("nil LeaseManager")
	}
	if config.LoggerContextGetter == nil {
		return errors.NotValidf("nil LoggerContextGetter")
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

// NewWorker returns a new domain services worker, with the given configuration.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	controllerObjectStoreGetter := controllerObjectStoreGetter{
		objectStoreGetter: config.ObjectStoreGetter,
	}
	ctrlFactory := config.NewControllerDomainServices(
		config.DBGetter,
		controllerObjectStoreGetter,
		config.Clock,
		config.Logger,
	)
	w := &domainServicesWorker{
		ctrlFactory: ctrlFactory,
		servicesGetter: config.NewDomainServicesGetter(
			ctrlFactory,
			config.DBGetter,
			config.NewModelDomainServices,
			config.ProviderFactory,
			config.ObjectStoreGetter,
			config.StorageRegistryGetter,
			config.PublicKeyImporter,
			config.LeaseManager,
			config.LogDir,
			config.Clock,
			config.LoggerContextGetter,
		),
	}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return w.tomb.Err()
	})
	return w, nil
}

// domainServicesWorker is a worker that holds a reference to a domain services.
// This doesn't actually create them dynamically, it just hands them out
// when asked.
type domainServicesWorker struct {
	tomb tomb.Tomb

	ctrlFactory    services.ControllerDomainServices
	servicesGetter services.DomainServicesGetter
}

// ControllerServices returns the controller domain services.
func (w *domainServicesWorker) ControllerServices() services.ControllerDomainServices {
	// TODO (stickupkid): Add metrics to here to see how often this is called.
	return w.ctrlFactory
}

// ServicesGetter returns the domain services getter.
func (w *domainServicesWorker) ServicesGetter() services.DomainServicesGetter {
	// TODO (stickupkid): Add metrics to here to see how often this is called.
	return w.servicesGetter
}

// Kill kills the domain services worker.
func (w *domainServicesWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait waits for the domain services worker to stop.
func (w *domainServicesWorker) Wait() error {
	return w.tomb.Wait()
}

// domainServices represents that are the composition of the controller and
// model services as a union type. In most circumstances, the controller service
// and model services are required at the same time, so this is a convenient way
// to get both services at the same time.
type domainServices struct {
	services.ControllerDomainServices
	services.ModelDomainServices
}

// domainServicesGetter is a domain services getter that returns the services
// for a model using the given model uuid. This is late binding, so the model
// domain services is created on demand.
type domainServicesGetter struct {
	ctrlFactory            services.ControllerDomainServices
	dbGetter               changestream.WatchableDBGetter
	newModelDomainServices ModelDomainServicesFn
	providerFactory        providertracker.ProviderFactory
	objectStoreGetter      objectstore.ObjectStoreGetter
	storageRegistryGetter  storage.StorageRegistryGetter
	publicKeyImporter      domainservices.PublicKeyImporter
	leaseManager           lease.Manager
	logDir                 string
	clock                  clock.Clock
	loggerContextGetter    logger.LoggerContextGetter
}

// ServicesForModel returns the domain services for the given model uuid.
// This will late bind the model domain services to the actual domain services.
func (s *domainServicesGetter) ServicesForModel(ctx context.Context, modelUUID coremodel.UUID) (services.DomainServices, error) {
	loggerContext, err := s.loggerContextGetter.GetLoggerContext(ctx, modelUUID)
	if err != nil {
		return nil, internalerrors.Errorf("getting logger context: %w", err)
	}

	return &domainServices{
		ControllerDomainServices: s.ctrlFactory,
		ModelDomainServices: s.newModelDomainServices(
			modelUUID, s.dbGetter,
			s.providerFactory,
			modelObjectStoreGetter{
				modelUUID:         modelUUID,
				objectStoreGetter: s.objectStoreGetter,
			},
			modelStorageRegistryGetter{
				modelUUID:             modelUUID,
				storageRegistryGetter: s.storageRegistryGetter,
			},
			s.publicKeyImporter,
			modelApplicationLeaseManager{
				modelUUID: modelUUID,
				manager:   s.leaseManager,
			},
			s.logDir,
			s.clock,
			loggerContext.GetLogger("juju.services"),
		),
	}, nil
}

// modelObjectStoreGetter is an object store getter that returns a singular
// object store for the given model uuid. This is to ensure that service
// factories can't access object stores for other models.
type modelObjectStoreGetter struct {
	modelUUID         coremodel.UUID
	objectStoreGetter objectstore.ObjectStoreGetter
}

// GetObjectStore returns a singular object store for the given namespace.
func (s modelObjectStoreGetter) GetObjectStore(ctx context.Context) (objectstore.ObjectStore, error) {
	return s.objectStoreGetter.GetObjectStore(ctx, s.modelUUID.String())
}

// controllerObjectStoreGetter is an object store getter that returns a singular
// object store for the given controller namespace. This is to ensure that
// service factories can't access object stores for other models.
type controllerObjectStoreGetter struct {
	objectStoreGetter objectstore.ObjectStoreGetter
}

// GetObjectStore returns a singular object store for the given namespace.
func (s controllerObjectStoreGetter) GetObjectStore(ctx context.Context) (objectstore.ObjectStore, error) {
	return s.objectStoreGetter.GetObjectStore(ctx, coredatabase.ControllerNS)
}

// modelStorageRegistryGetter is a storage registry getter that returns a
// singular storage registry for the given model uuid. This is to ensure that
// service factories can't access storage registries for other models.
type modelStorageRegistryGetter struct {
	modelUUID             coremodel.UUID
	storageRegistryGetter storage.StorageRegistryGetter
}

// GetStorageRegistry returns a singular storage registry for the given
// namespace.
func (s modelStorageRegistryGetter) GetStorageRegistry(ctx context.Context) (internalstorage.ProviderRegistry, error) {
	return s.storageRegistryGetter.GetStorageRegistry(ctx, s.modelUUID.String())
}

// modelApplicationLeaseManager is a lease manager that is specific to
// an application scope.
type modelApplicationLeaseManager struct {
	modelUUID coremodel.UUID
	manager   lease.Manager
}

// GetLeaseManager returns a lease manager for the given model UUID.
func (s modelApplicationLeaseManager) GetLeaseManager() (lease.LeaseManager, error) {
	checker, err := s.manager.Checker(lease.ApplicationLeadershipNamespace, s.modelUUID.String())
	if err != nil {
		return nil, internalerrors.Errorf("getting checker lease manager: %w", err)
	}

	revoker, err := s.manager.Revoker(lease.ApplicationLeadershipNamespace, s.modelUUID.String())
	if err != nil {
		return nil, internalerrors.Errorf("getting revoker lease manager: %w", err)
	}

	return leaseManager{
		checker: checker,
		revoker: revoker,
	}, nil
}

type leaseManager struct {
	checker lease.Checker
	revoker lease.Revoker
}

// WaitUntilExpired returns nil when the named lease is no longer held.
func (s leaseManager) WaitUntilExpired(ctx context.Context, leaseName string, started chan<- struct{}) error {
	return s.checker.WaitUntilExpired(ctx, leaseName, started)
}

// Token returns a Token that can be interrogated at any time to discover
// whether the supplied lease is currently held by the supplied holder.
func (s leaseManager) Token(leaseName, holderName string) lease.Token {
	return s.checker.Token(leaseName, holderName)
}

// Revoke releases the named lease for the named holder.
func (s leaseManager) Revoke(leaseName, holderName string) error {
	return s.revoker.Revoke(leaseName, holderName)
}
