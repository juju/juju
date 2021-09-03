// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/api/base"
	api "github.com/juju/juju/api/caasmodelconfigmanager"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/docker"
)

// Logger represents the methods used by the worker to log details.
type Logger interface {
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
	Errorf(string, ...interface{})
	Warningf(string, ...interface{})
	Tracef(string, ...interface{})

	Child(string) loggo.Logger
}

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/facade_mock.go github.com/juju/juju/worker/caasmodelconfigmanager Facade
type Facade interface {
	ControllerConfig() (controller.Config, error)
}

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/broker_mock.go github.com/juju/juju/worker/caasmodelconfigmanager CAASBroker
type CAASBroker interface {
	EnsureImageRepoSecret(docker.ImageRepoDetails) error
}

// Config holds the configuration and dependencies for a worker.
type Config struct {
	ModelTag names.ModelTag

	Facade Facade
	Broker CAASBroker
	Logger Logger
}

// Validate returns an error if the config cannot be expected
// to drive a functional worker.
func (config Config) Validate() error {
	if config.ModelTag == (names.ModelTag{}) {
		return errors.NotValidf("ModelTag is missing")
	}
	if config.Facade == nil {
		return errors.NotValidf("Facade is missing")
	}
	if config.Broker == nil {
		return errors.NotValidf("Broker is missing")
	}
	if config.Logger == nil {
		return errors.NotValidf("Logger is missing")
	}
	return nil
}

type manager struct {
	catacomb catacomb.Catacomb

	name   string
	config Config
	logger Logger
}

// NewFacade returns a facade for caasapplicationprovisioner worker to use.
func NewFacade(caller base.APICaller) (Facade, error) {
	return api.NewClient(caller)
}

// NewWorker returns a worker that unlocks the model upgrade gate.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &manager{
		name:   config.ModelTag.Id(),
		config: config,
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

// Kill is part of the worker.Worker interface.
func (w *manager) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *manager) Wait() error {
	return w.catacomb.Wait()
}

func (w *manager) loop() error {
	controllerConfig, err := w.config.Facade.ControllerConfig()
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.config.Broker.EnsureImageRepoSecret(controllerConfig.CAASImageRepo()); err != nil {
		return errors.Trace(err)
	}
	return nil
}
