// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/api/base"
	api "github.com/juju/juju/api/controller/caasmodelconfigmanager"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/docker"
	"github.com/juju/juju/docker/imagerepo"
	"github.com/juju/juju/docker/registry"
)

// Logger represents the methods used by the worker to log details.
type Logger interface {
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
}

// ManifoldConfig describes how to configure and construct a Worker,
// and what registered resources it may depend upon.
type ManifoldConfig struct {
	APICallerName string
	BrokerName    string

	NewControllerConfigService func(base.APICaller) (ControllerConfigService, error)
	NewWorker                  func(Config) (worker.Worker, error)
	NewRegistry                RegistryFunc
	NewImageRepo               ImageRepoFunc

	Logger Logger
	Clock  clock.Clock
}

// Validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if config.BrokerName == "" {
		return errors.NotValidf("empty BrokerName")
	}
	if config.NewControllerConfigService == nil {
		return errors.NotValidf("nil NewControllerConfigService")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewRegistry == nil {
		return errors.NotValidf("nil NewRegistry")
	}
	if config.NewImageRepo == nil {
		return errors.NotValidf("nil NewImageRepo")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	return nil
}

func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var apiCaller base.APICaller
	if err := context.Get(config.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}

	var broker caas.Broker
	if err := context.Get(config.BrokerName, &broker); err != nil {
		return nil, errors.Trace(err)
	}

	modelTag, ok := apiCaller.ModelTag()
	if !ok {
		return nil, errors.New("API connection is controller-only (should never happen)")
	}

	controllerConfigService, err := config.NewControllerConfigService(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}
	worker, err := config.NewWorker(Config{
		ModelTag:                modelTag,
		ControllerConfigService: controllerConfigService,
		Broker:                  broker,
		Logger:                  config.Logger,
		RegistryFunc:            config.NewRegistry,
		ImageRepoFunc:           config.NewImageRepo,
		Clock:                   config.Clock,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}

// Manifold returns a dependency.Manifold that will run a Worker as
// configured.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
			config.BrokerName,
		},
		Start: config.start,
	}
}

// NewControllerConfigService returns a new ControllerConfig service from
// a given API caller.
func NewControllerConfigService(caller base.APICaller) (ControllerConfigService, error) {
	return api.NewClient(caller)
}

// NewRegistry returns a new Registry from a given image repo details.
func NewRegistry(details docker.ImageRepoDetails) (Registry, error) {
	return registry.New(details)
}

// NewImageRepo returns a new ImageRepo from a given path.
func NewImageRepo(path string) (ImageRepo, error) {
	return imagerepo.NewImageRepo(path)
}
