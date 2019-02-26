// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"github.com/juju/errors"
	names "gopkg.in/juju/names.v2"
	worker "gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/catacomb"
)

//go:generate mockgen -package mocks -destination mocks/instancebroker_mock.go github.com/juju/juju/worker/instancemutater InstanceBroker
//go:generate mockgen -package mocks -destination mocks/logger_mock.go github.com/juju/juju/worker/instancemutater Logger
//go:generate mockgen -package mocks -destination mocks/namestag_mock.go gopkg.in/juju/names.v2 Tag

type InstanceBroker interface {
}

// Logger represents the logging methods called.
type Logger interface {
	Warningf(message string, args ...interface{})
	Errorf(message string, args ...interface{})
}

// Config represents the configuration required to run a new instance mutater
// worker.
type Config struct {
	Broker InstanceBroker

	// Logger is the logger for this worker.
	Logger Logger

	// Tag is the current machine tag
	Tag names.Tag
}

// Validate checks for missing values from the configuration and checks that
// they conform to a given type.
func (config Config) Validate() error {
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.Broker == nil {
		return errors.NotValidf("nil InstanceBroker")
	}
	if config.Tag == nil {
		return errors.NotValidf("nil tag")
	}
	if _, ok := config.Tag.(names.MachineTag); !ok {
		return errors.NotValidf("tag")
	}
	return nil
}

// NewWorker returns a worker that keeps track of
// the machines/containers in the state and polls their instance
// for any changes.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &mutaterWorker{
		logger: config.Logger,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

type mutaterWorker struct {
	catacomb catacomb.Catacomb
	logger   Logger
}

func (w *mutaterWorker) loop() error {
	return nil
}

// Kill implements worker.Worker.Kill.
func (w *mutaterWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (w *mutaterWorker) Wait() error {
	return w.catacomb.Wait()
}

// Stop stops the upgradeseriesworker and returns any
// error it encountered when running.
func (w *mutaterWorker) Stop() error {
	w.Kill()
	return w.Wait()
}
