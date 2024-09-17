// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoreservices

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/servicefactory"
)

// Config is the configuration required for services worker.
type Config struct {
	// DBGetter supplies WatchableDB implementations by namespace.
	DBGetter changestream.WatchableDBGetter

	Logger logger.Logger

	NewObjectStoreServicesGetter ObjectStoreServicesGetterFn
	NewObjectStoreServices       ObjectStoreServicesFn
}

// Validate validates the services configuration.
func (config Config) Validate() error {
	if config.DBGetter == nil {
		return errors.NotValidf("nil DBGetter")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.NewObjectStoreServices == nil {
		return errors.NotValidf("nil NewObjectStoreServices")
	}
	if config.NewObjectStoreServicesGetter == nil {
		return errors.NotValidf("nil NewObjectStoreServicesGetter")
	}
	return nil
}

// NewWorker returns a new services worker, with the given configuration.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &servicesWorker{
		servicesGetter: config.NewObjectStoreServicesGetter(
			config.NewObjectStoreServices,
			config.DBGetter,
			config.Logger,
		),
	}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return w.tomb.Err()
	})
	return w, nil
}

// servicesWorker is a worker that holds a reference to a object store services.
// This doesn't actually create them dynamically, it just hands them out
// when asked.
type servicesWorker struct {
	tomb tomb.Tomb

	servicesGetter servicefactory.ObjectStoreServicesGetter
}

// ServicesGetter returns the object store services getter.
func (w *servicesWorker) ServicesGetter() servicefactory.ObjectStoreServicesGetter {
	return w.servicesGetter
}

// Kill kills the services worker.
func (w *servicesWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait waits for the services worker to stop.
func (w *servicesWorker) Wait() error {
	return w.tomb.Wait()
}

// services is a object store services type.
type services struct {
	servicefactory.ObjectStoreServices
}

// serviceFactoryGetter is a object store services getter that returns a
// object store services for the given model uuid. This is late binding,
// so the object store services is created on demand.
type serviceFactoryGetter struct {
	newObjectStoreServices ObjectStoreServicesFn
	dbGetter               changestream.WatchableDBGetter
	logger                 logger.Logger
}

// FactoryForModel returns a object store services for the given model
// uuid. This will late bind the object store services to the actual
// services.
func (s *serviceFactoryGetter) FactoryForModel(modelUUID string) servicefactory.ObjectStoreServices {
	return &services{
		ObjectStoreServices: s.newObjectStoreServices(
			coremodel.UUID(modelUUID), s.dbGetter, s.logger,
		),
	}
}
