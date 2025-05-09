// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoreflag

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/services"
)

// GetObjectStoreServiceServicesFunc is a function that retrieves the
// object store services from the dependency getter.
type GetObjectStoreServiceServicesFunc func(dependency.Getter, string) (ObjectStoreService, error)

// ManifoldConfig holds the dependencies and configuration for a
// Worker manifold.
type ManifoldConfig struct {
	ObjectStoreServicesName string
	Check                   Predicate

	GeObjectStoreServicesFn GetObjectStoreServiceServicesFunc
	NewWorker               func(context.Context, Config) (worker.Worker, error)
}

// validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.ObjectStoreServicesName == "" {
		return errors.NotValidf("empty ObjectStoreServicesName")
	}
	if config.Check == nil {
		return errors.NotValidf("nil Check")
	}
	if config.GeObjectStoreServicesFn == nil {
		return errors.NotValidf("nil GeObjectStoreServicesFn")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (config ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	objectStoreService, err := config.GeObjectStoreServicesFn(getter, config.ObjectStoreServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	worker, err := config.NewWorker(context, Config{
		ObjectStoreService: objectStoreService,
		Check:              config.Check,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}

// Manifold packages a Worker for use in a dependency.Engine.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.ObjectStoreServicesName,
		},
		Start:  config.start,
		Output: engine.FlagOutput,
		Filter: bounceErrChanged,
	}
}

// bounceErrChanged converts ErrChanged to dependency.ErrBounce.
func bounceErrChanged(err error) error {
	if errors.Cause(err) == ErrChanged {
		return dependency.ErrBounce
	}
	return err
}

// GetObjectStoreServices retrieves the ObjectStoreService using the given
// service.
func GeObjectStoreServices(getter dependency.Getter, name string) (ObjectStoreService, error) {
	var services services.ControllerObjectStoreServices
	if err := getter.Get(name, &services); err != nil {
		return nil, errors.Trace(err)
	}

	return services.AgentObjectStore(), nil
}

// IsTerminal checks if the phase is a terminal phase.
func IsTerminal(phase objectstore.Phase) bool {
	return phase.IsNotStarted() || phase.IsTerminal()
}
