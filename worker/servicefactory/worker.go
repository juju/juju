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
	if config.NewControllerServiceFactory == nil {
		return errors.NotValidf("nil NewControllerServiceFactory")
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

type serviceFactoryWorker struct {
	tomb tomb.Tomb

	ctrlFactory   ControllerServiceFactory
	factoryGetter ServiceFactoryGetter
}

func (w *serviceFactoryWorker) Kill() {
	w.tomb.Kill(nil)
}

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
