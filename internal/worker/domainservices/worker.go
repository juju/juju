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
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/internal/services"
)

// Config is the configuration required for domain services worker.
type Config struct {
	// DBDeleter is used to delete databases.
	DBDeleter coredatabase.DBDeleter

	// DBGetter supplies WatchableDB implementations by namespace.
	DBGetter changestream.WatchableDBGetter

	// ProviderFactory is used to get provider instances.
	ProviderFactory providertracker.ProviderFactory

	// ObjectStoreGetter is used to get object store instances.
	ObjectStoreGetter objectstore.ObjectStoreGetter

	// Logger is used to log messages.
	Logger logger.Logger

	Clock clock.Clock

	NewDomainServicesGetter     DomainServicesGetterFn
	NewControllerDomainServices ControllerDomainServicesFn
	NewModelDomainServices      ModelDomainServicesFn
}

// Validate validates the domain services configuration.
func (config Config) Validate() error {
	if config.DBDeleter == nil {
		return errors.NotValidf("nil DBDeleter")
	}
	if config.DBGetter == nil {
		return errors.NotValidf("nil DBGetter")
	}
	if config.ProviderFactory == nil {
		return errors.NotValidf("nil ProviderFactory")
	}
	if config.ObjectStoreGetter == nil {
		return errors.NotValidf("nil ObjectStoreGetter")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
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
	return nil
}

// NewWorker returns a new domain services worker, with the given configuration.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	ctrlFactory := config.NewControllerDomainServices(config.DBGetter, config.DBDeleter, config.Logger)
	w := &domainServicesWorker{
		ctrlFactory: ctrlFactory,
		servicesGetter: config.NewDomainServicesGetter(
			ctrlFactory,
			config.DBGetter,
			config.Logger,
			config.NewModelDomainServices,
			config.ProviderFactory,
			config.ObjectStoreGetter,
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
	logger                 logger.Logger
	clock                  clock.Clock
	newModelDomainServices ModelDomainServicesFn
	providerFactory        providertracker.ProviderFactory
	objectStoreGetter      objectstore.ObjectStoreGetter
}

// ServicesForModel returns the domain services for the given model uuid.
// This will late bind the model domain services to the actual domain services.
func (s *domainServicesGetter) ServicesForModel(modelUUID coremodel.UUID) services.DomainServices {
	return &domainServices{
		ControllerDomainServices: s.ctrlFactory,
		ModelDomainServices: s.newModelDomainServices(
			modelUUID, s.dbGetter,
			s.providerFactory,
			modelObjectStoreGetter{
				modelUUID:         modelUUID,
				objectStoreGetter: s.objectStoreGetter,
			},
			s.logger,
			s.clock,
		),
	}
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
