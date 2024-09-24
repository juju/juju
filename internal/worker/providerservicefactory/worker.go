// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providerservicefactory

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/services"
)

// Config is the configuration required for domain services worker.
type Config struct {
	// DBGetter supplies WatchableDB implementations by namespace.
	DBGetter changestream.WatchableDBGetter

	Logger logger.Logger

	NewProviderServicesGetter ProviderServicesGetterFn
	NewProviderServices       ProviderServicesFn
}

// Validate validates the domain services configuration.
func (config Config) Validate() error {
	if config.DBGetter == nil {
		return errors.NotValidf("nil DBGetter")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.NewProviderServices == nil {
		return errors.NotValidf("nil NewProviderServices")
	}
	if config.NewProviderServicesGetter == nil {
		return errors.NotValidf("nil NewProviderServicesGetter")
	}
	return nil
}

// NewWorker returns a new domain services worker, with the given configuration.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &domainServicesWorker{
		servicesGetter: config.NewProviderServicesGetter(
			config.NewProviderServices,
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

// domainServicesWorker is a worker that holds a reference to a domain services.
// This doesn't actually create them dynamically, it just hands them out
// when asked.
type domainServicesWorker struct {
	tomb tomb.Tomb

	servicesGetter services.ProviderServicesGetter
}

// ServicesGetter returns the provider domain services getter.
func (w *domainServicesWorker) ServicesGetter() services.ProviderServicesGetter {
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

// domainServices is a provider domain services type.
type domainServices struct {
	services.ProviderServices
}

// domainServicesGetter is a provider domain services getter that returns a
// provider domain services for the given model uuid. This is late binding,
// so the provider domain services is created on demand.
type domainServicesGetter struct {
	newProviderServices ProviderServicesFn
	dbGetter            changestream.WatchableDBGetter
	logger              logger.Logger
}

// ServicesForModel returns a provider domain services for the given model uuid.
// This will late bind the provider domain services to the actual service
// factory.
func (s *domainServicesGetter) ServicesForModel(modelUUID string) services.ProviderServices {
	return &domainServices{
		ProviderServices: s.newProviderServices(
			coremodel.UUID(modelUUID), s.dbGetter, s.logger,
		),
	}
}
