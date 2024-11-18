// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/services"
)

// ManifoldConfig describes the resources used by the charmdownloader worker.
type ManifoldConfig struct {
	DomainServicesName string
	Logger             logger.Logger
}

// Manifold returns a Manifold that encapsulates the charmdownloader worker.
func Manifold(cfg ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			cfg.DomainServicesName,
		},
		Start: cfg.start,
	}
}

// Validate is called by start to check for bad configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (cfg ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var domainServices services.DomainServices
	if err := getter.Get(cfg.DomainServicesName, &domainServices); err != nil {
		return nil, errors.Trace(err)
	}

	w, err := NewWorker(Config{
		Logger: cfg.Logger,
		//ApplicationService: domainServices.Application(),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}
