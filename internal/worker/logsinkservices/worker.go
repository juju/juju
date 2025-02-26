// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsinkservices

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

	NewLogSinkServicesGetter LogSinkServicesGetterFn
	NewLogSinkServices       LogSinkServicesFn
}

// Validate validates the domain services configuration.
func (config Config) Validate() error {
	if config.DBGetter == nil {
		return errors.NotValidf("nil DBGetter")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.NewLogSinkServices == nil {
		return errors.NotValidf("nil NewLogSinkServices")
	}
	if config.NewLogSinkServicesGetter == nil {
		return errors.NotValidf("nil NewLogSinkServicesGetter")
	}
	return nil
}

// NewWorker returns a new domain services worker, with the given configuration.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &servicesWorker{
		servicesGetter: config.NewLogSinkServicesGetter(
			config.NewLogSinkServices,
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

// servicesWorker is a worker that holds a reference to a domain services.
// This doesn't actually create them dynamically, it just hands them out
// when asked.
type servicesWorker struct {
	tomb tomb.Tomb

	servicesGetter services.LogSinkServicesGetter
}

// ServicesGetter returns the log sink domain services getter.
func (w *servicesWorker) ServicesGetter() services.LogSinkServicesGetter {
	return w.servicesGetter
}

// ControllerServices returns the controller log sink services.
// Attempting to use anything other than the controller services will
// result in a panic.
func (w *servicesWorker) ControllerServices() services.ControllerLogSinkServices {
	return w.servicesGetter.ServicesForModel(coremodel.ControllerModelName)
}

// Kill kills the domain services worker.
func (w *servicesWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait waits for the domain services worker to stop.
func (w *servicesWorker) Wait() error {
	return w.tomb.Wait()
}

// domainServices is a log sink domain services type.
type domainServices struct {
	services.LogSinkServices
}

// domainServicesGetter is a log sink domain services getter that returns a
// log sink domain services for the given model uuid. This is late binding,
// so the log sink domain services is created on demand.
type domainServicesGetter struct {
	newLogSinkServices LogSinkServicesFn
	dbGetter           changestream.WatchableDBGetter
	logger             logger.Logger
}

// ServicesForModel returns a log sink domain services for the given model uuid.
// This will late bind the log sink domain services to the actual service
// factory.
func (s *domainServicesGetter) ServicesForModel(modelUUID coremodel.UUID) services.LogSinkServices {
	return &domainServices{
		LogSinkServices: s.newLogSinkServices(
			modelUUID, s.dbGetter, s.logger,
		),
	}
}
