// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoreguard

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/fortress"
)

// GetDrainingServiceFunc retrieves the controller object-store draining
// service from the manifold getter.
type GetDrainingServiceFunc func(dependency.Getter, string) (DrainingService, error)

// ManifoldConfig contains the dependencies required by the guard worker.
type ManifoldConfig struct {
	ObjectStoreServicesName string
	FortressName            string

	GetDrainingService GetDrainingServiceFunc
	NewWorker          func(Config) (worker.Worker, error)
	Logger             logger.Logger
}

// Validate checks the manifold configuration.
func (c ManifoldConfig) Validate() error {
	if c.ObjectStoreServicesName == "" {
		return errors.NotValidf("empty ObjectStoreServicesName")
	}
	if c.FortressName == "" {
		return errors.NotValidf("empty FortressName")
	}
	if c.GetDrainingService == nil {
		return errors.NotValidf("nil GetDrainingService")
	}
	if c.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Manifold returns a worker manifold for keeping a controller's local
// object-store guard synchronized with the draining phase.
func Manifold(c ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			c.ObjectStoreServicesName,
			c.FortressName,
		},
		Start: c.start,
	}
}

func (c ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := c.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	drainingService, err := c.GetDrainingService(getter, c.ObjectStoreServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var guard fortress.Guard
	if err := getter.Get(c.FortressName, &guard); err != nil {
		return nil, errors.Trace(err)
	}
	return c.NewWorker(Config{
		Guard:           guard,
		DrainingService: drainingService,
		Logger:          c.Logger,
	})
}

// GetDrainingService retrieves the controller's object-store draining service.
func GetDrainingService(getter dependency.Getter, name string) (DrainingService, error) {
	var objectStoreServices services.ControllerObjectStoreServices
	if err := getter.Get(name, &objectStoreServices); err != nil {
		return nil, errors.Trace(err)
	}
	var service DrainingService = objectStoreServices.AgentObjectStore()
	return service, nil
}
