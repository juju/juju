// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dqliteconfigupdater

import (
	"context"

	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	coredependency "github.com/juju/juju/core/dependency"
	coreerrors "github.com/juju/juju/core/errors"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/controllerruntimeconfig"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
)

// ManifoldConfig defines the configuration for the dqlite config updater
// manifold.
type ManifoldConfig struct {
	// DomainServicesName is the name of the domain-services dependency.
	// The updater resolves the controller config service, which watches
	// and holds the mutable Dqlite tuning values it synchronises into
	// the local runtime config, from the controller domain services
	// provided by that manifold.
	DomainServicesName string

	// RuntimeConfigPath is the path of this controller's local
	// runtime.conf delivery cache.
	RuntimeConfigPath string

	// Logger is the logger used by the worker.
	Logger corelogger.Logger
}

// Validate checks that all required configuration fields are set.
func (c ManifoldConfig) Validate() error {
	if c.DomainServicesName == "" {
		return errors.New("missing DomainServicesName not valid").Add(coreerrors.NotValid)
	}
	if c.RuntimeConfigPath == "" {
		return errors.New("missing RuntimeConfigPath not valid").Add(coreerrors.NotValid)
	}
	if c.Logger == nil {
		return errors.New("missing Logger not valid").Add(coreerrors.NotValid)
	}
	return nil
}

// Manifold returns a dependency manifold that runs the dqlite config
// updater, a jujud worker that synchronises the mutable Dqlite tuning
// values of the controller config into this controller's local
// runtime.conf and requests a controller restart so dbaccessor
// constructs Dqlite with the new values.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DomainServicesName,
		},
		Start: config.start,
	}
}

func (c ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := c.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	controllerConfigService, err := GetControllerConfigService(getter, c.DomainServicesName)
	if err != nil {
		return nil, errors.Errorf("getting controller config service: %w", err)
	}

	return NewWorker(Config{
		ControllerConfigService: controllerConfigService,
		ReadRuntimeConfig: func() (controllerruntimeconfig.ControllerRuntimeConfig, error) {
			return controllerruntimeconfig.ReadControllerRuntimeConfig(c.RuntimeConfigPath)
		},
		ChangeRuntimeConfig: func(mutate controllerruntimeconfig.Mutator) error {
			return controllerruntimeconfig.ChangeControllerRuntimeConfig(c.RuntimeConfigPath, mutate)
		},
		Logger: c.Logger,
	})
}

// GetControllerConfigService returns the controller config service from
// the domain-services dependency with the given name.
func GetControllerConfigService(getter dependency.Getter, name string) (ControllerConfigService, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory services.ControllerDomainServices) ControllerConfigService {
		return factory.ControllerConfig()
	})
}
