// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoredrainer

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/fortress"
)

// GetObjectStoreServiceServicesFunc is a function that retrieves the
// object store services from the dependency getter.
type GetObjectStoreServiceServicesFunc func(dependency.Getter, string) (ObjectStoreService, error)

// ManifoldConfig holds the dependencies and configuration for a
// Worker manifold.
type ManifoldConfig struct {
	ObjectStoreServicesName string
	FortressName            string

	GeObjectStoreServicesFn GetObjectStoreServiceServicesFunc
	NewWorker               func(context.Context, Config) (worker.Worker, error)

	Logger logger.Logger
}

// validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.FortressName == "" {
		return errors.NotValidf("empty FortressName")
	}
	if config.ObjectStoreServicesName == "" {
		return errors.NotValidf("empty ObjectStoreServicesName")
	}
	if config.GeObjectStoreServicesFn == nil {
		return errors.NotValidf("nil GeObjectStoreServicesFn")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
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

	var fortress fortress.Guard
	if err := getter.Get(config.FortressName, &fortress); err != nil {
		return nil, errors.Trace(err)
	}

	worker, err := config.NewWorker(context, Config{
		Guard:              fortress,
		ObjectStoreService: objectStoreService,
		Logger:             config.Logger,
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
			config.FortressName,
			config.ObjectStoreServicesName,
		},
		Start: config.start,
	}
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
