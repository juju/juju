// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicefactory

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/worker/v3"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/servicefactory"
)

// Config is the configuration required for service factory worker.
type Config struct {
	// DBDeleter is used to delete databases.
	DBDeleter coredatabase.DBDeleter

	// DBGetter supplies WatchableDB implementations by namespace.
	DBGetter changestream.WatchableDBGetter

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

	ctrlFactory := config.NewControllerServiceFactory(config.DBGetter, config.DBDeleter, config.Logger)
	w := &serviceFactoryWorker{
		ctrlFactory:   ctrlFactory,
		factoryGetter: config.NewServiceFactoryGetter(ctrlFactory, config.DBGetter, config.Logger, config.NewModelServiceFactory),
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

	ctrlFactory   ControllerServiceFactory
	factoryGetter ServiceFactoryGetter
}

// ControllerFactory returns the controller service factory.
func (w *serviceFactoryWorker) ControllerFactory() ControllerServiceFactory {
	// TODO (stickupkid): Add metrics to here to see how often this is called.
	return w.ctrlFactory
}

// FactoryGetter returns the service factory getter.
func (w *serviceFactoryWorker) FactoryGetter() ServiceFactoryGetter {
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

func (c loggoLogger) Child(name string) Logger {
	return c
}

// serviceFactoryLogger is a Logger for the service factory.
type serviceFactoryLogger struct {
	Logger
}

func (c serviceFactoryLogger) Child(name string) servicefactory.Logger {
	return c
}

type serviceFactoryGetter struct {
	ctrlFactory            ControllerServiceFactory
	dbGetter               changestream.WatchableDBGetter
	logger                 Logger
	newModelServiceFactory ModelServiceFactoryFn
}

// FactoryForModel returns a service factory for the given model uuid.
// This will late bind the model service factory to the actual service factory.
func (s *serviceFactoryGetter) FactoryForModel(modelUUID string) ServiceFactory {
	// At the moment the model service factory is not cached, and is created
	// on demand. We could cache it here, but then it's not clear when to clear
	// the cache. Given that the model service factory is cheap to create, we
	// can just create it on demand and then look into some sort of finalizer
	// to clear the cache at a later point.
	return &serviceFactory{
		ControllerServiceFactory: s.ctrlFactory,
		ModelServiceFactory:      s.newModelServiceFactory(s.dbGetter, modelUUID, s.logger),
	}
}

type serviceFactory struct {
	ControllerServiceFactory
	ModelServiceFactory
}
