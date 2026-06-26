// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/docker/registry"
	"github.com/juju/juju/internal/services"
)

// ControllerConfigService provides access to controller configuration.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
	WatchControllerConfig(context.Context) (watcher.StringsWatcher, error)
}

// GetDomainServicesFunc is a helper function that gets the domain services
// from the manifold.
type GetDomainServicesFunc func(getter dependency.Getter, name string) (ControllerConfigService, error)

// GetDomainServices is a helper function that gets the controller config
// service from the manifold.
func GetDomainServices(getter dependency.Getter, name string) (ControllerConfigService, error) {
	return getDependencyByName(getter, name, func(s services.DomainServices) ControllerConfigService {
		return s.ControllerConfig()
	})
}

// getDependencyByName is a helper that extracts a dependency of type A from
// the getter and transforms it to type B using the provided function.
func getDependencyByName[A, B any](getter dependency.Getter, name string, fn func(A) B) (B, error) {
	var a A
	if err := getter.Get(name, &a); err != nil {
		var zero B
		return zero, err
	}
	return fn(a), nil
}

// ManifoldConfig describes how to configure and construct a Worker,
// and what registered resources it may depend upon.
type ManifoldConfig struct {
	DomainServicesName string
	BrokerName         string
	ModelUUID          string

	GetDomainServices GetDomainServicesFunc
	NewWorker         func(Config) (worker.Worker, error)

	Logger logger.Logger
	Clock  clock.Clock
}

// Validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if config.BrokerName == "" {
		return errors.NotValidf("empty BrokerName")
	}
	if config.ModelUUID == "" {
		return errors.NotValidf("empty ModelUUID")
	}
	if config.GetDomainServices == nil {
		return errors.NotValidf("nil GetDomainServices")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	return nil
}

func (config ManifoldConfig) start(_ context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var broker caas.Broker
	if err := getter.Get(config.BrokerName, &broker); err != nil {
		return nil, errors.Trace(err)
	}

	ctrlConfigSvc, err := config.GetDomainServices(getter, config.DomainServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	w, err := config.NewWorker(Config{
		ModelTag:     names.NewModelTag(config.ModelUUID),
		Facade:       ctrlConfigSvc,
		Broker:       broker,
		Logger:       config.Logger,
		RegistryFunc: registry.New,
		Clock:        config.Clock,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Manifold returns a dependency.Manifold that will run a Worker as
// configured.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DomainServicesName,
			config.BrokerName,
		},
		Start: config.start,
	}
}
