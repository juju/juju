// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicefactory

import (
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/status"
	domainservicefactory "github.com/juju/juju/domain/servicefactory"
	"github.com/juju/juju/internal/servicefactory"
)

// Config is the configuration required for service factory worker.
type Config struct {
	// DBDeleter is used to delete databases.
	DBDeleter coredatabase.DBDeleter

	// DBGetter supplies WatchableDB implementations by namespace.
	DBGetter changestream.WatchableDBGetter

	// ProviderFactory is used to get provider instances.
	ProviderFactory providertracker.ProviderFactory

	// BrokerFactory is used to get broker instances.
	BrokerFactory providertracker.ProviderFactory

	// StatusHistoryFactory is used to get status history setter for a given
	// model.
	StatusHistoryFactory status.StatusHistoryFactory

	// Logger is used to log messages.
	Logger Logger

	NewServiceFactoryGetter     ServiceFactoryGetterFn
	NewControllerServiceFactory ControllerServiceFactoryFn
	NewModelServiceFactory      ModelServiceFactoryFn
}

// Validate validates the service factory configuration.
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
	if config.BrokerFactory == nil {
		return errors.NotValidf("nil BrokerFactory")
	}
	if config.StatusHistoryFactory == nil {
		return errors.NotValidf("nil StatusHistoryFactory")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.NewServiceFactoryGetter == nil {
		return errors.NotValidf("nil NewServiceFactoryGetter")
	}
	if config.NewControllerServiceFactory == nil {
		return errors.NotValidf("nil NewControllerServiceFactory")
	}
	if config.NewModelServiceFactory == nil {
		return errors.NotValidf("nil NewModelServiceFactory")
	}
	return nil
}

// NewWorker returns a new service factory worker, with the given configuration.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	ctrlFactory := config.NewControllerServiceFactory(
		config.DBGetter,
		config.DBDeleter,
		config.StatusHistoryFactory,
		config.Logger,
	)
	w := &serviceFactoryWorker{
		ctrlFactory: ctrlFactory,
		factoryGetter: config.NewServiceFactoryGetter(
			ctrlFactory,
			config.DBGetter,
			config.Logger,
			config.NewModelServiceFactory,
			config.ProviderFactory,
			config.BrokerFactory,
			config.StatusHistoryFactory,
		),
	}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return w.tomb.Err()
	})
	return w, nil
}

// serviceFactoryWorker is a worker that holds a reference to a service factory.
// This doesn't actually create them dynamically, it just hands them out
// when asked.
type serviceFactoryWorker struct {
	tomb tomb.Tomb

	ctrlFactory   servicefactory.ControllerServiceFactory
	factoryGetter servicefactory.ServiceFactoryGetter
}

// ControllerFactory returns the controller service factory.
func (w *serviceFactoryWorker) ControllerFactory() servicefactory.ControllerServiceFactory {
	// TODO (stickupkid): Add metrics to here to see how often this is called.
	return w.ctrlFactory
}

// FactoryGetter returns the service factory getter.
func (w *serviceFactoryWorker) FactoryGetter() servicefactory.ServiceFactoryGetter {
	// TODO (stickupkid): Add metrics to here to see how often this is called.
	return w.factoryGetter
}

// Kill kills the service factory worker.
func (w *serviceFactoryWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait waits for the service factory worker to stop.
func (w *serviceFactoryWorker) Wait() error {
	return w.tomb.Wait()
}

// serviceFactory is a service factory that combines the controller and model
// service factories as a composed union type.
// The service factory is a composition of the controller and model service
// factories. In most circumstances, the controller service and model services
// are required at the same time, so this is a convenient way to get both
// services at the same time.
type serviceFactory struct {
	servicefactory.ControllerServiceFactory
	servicefactory.ModelServiceFactory
}

// serviceFactoryGetter is a service factory getter that returns a service
// factory for the given model uuid. This is late binding, so the model
// service factory is created on demand.
type serviceFactoryGetter struct {
	ctrlFactory            servicefactory.ControllerServiceFactory
	dbGetter               changestream.WatchableDBGetter
	logger                 Logger
	newModelServiceFactory ModelServiceFactoryFn
	providerFactory        providertracker.ProviderFactory
	brokerFactory          providertracker.ProviderFactory
	statusHistoryFactory   status.StatusHistoryFactory
}

// FactoryForModel returns a service factory for the given model uuid.
// This will late bind the model service factory to the actual service factory.
func (s *serviceFactoryGetter) FactoryForModel(modelUUID string) servicefactory.ServiceFactory {
	return &serviceFactory{
		ControllerServiceFactory: s.ctrlFactory,
		ModelServiceFactory: s.newModelServiceFactory(
			coremodel.UUID(modelUUID), s.dbGetter,
			s.providerFactory, s.brokerFactory,
			s.statusHistoryFactory,
			s.logger,
		),
	}
}

// The following loggers are required because the Logger interfaces in the other
// locations have a Child method that returns the same Logger type. As this
// a self referential type, we need to wrap it in a new type to satisfy the
// interface. The solution to this is to return a concrete type, but that is
// not idea either, as it means we can't push the testing logger into the
// tests.

// loggoLogger is a loggo.Logger for the service factory.
type loggoLogger struct {
	loggo.Logger
}

// NewLogger returns a new logger for the service factory.
func NewLogger(ns string) Logger {
	return loggoLogger{
		Logger: loggo.GetLogger(ns),
	}
}

// Child returns a child logger that satisfies the Logger interface.
func (c loggoLogger) Child(name string) Logger {
	return c
}

// serviceFactoryLogger is a Logger for the service factory.
type serviceFactoryLogger struct {
	Logger
}

// Child returns a child logger that satisfies the domainservicefactory.Logger.
func (c serviceFactoryLogger) Child(name string) domainservicefactory.Logger {
	return c
}
