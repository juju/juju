// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"context"

	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/caas"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
	internalworker "github.com/juju/juju/internal/worker"
)

// ManifoldConfig describes the resources used by the firewaller worker.
type ManifoldConfig struct {
	BrokerName         string
	DomainServicesName string

	NewWorker func(Config) (worker.Worker, error)
	Logger    logger.Logger
}

// Manifold returns a Manifold that encapsulates the firewaller worker.
func Manifold(cfg ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			cfg.BrokerName,
			cfg.DomainServicesName,
		},
		Start:  cfg.start,
		Filter: internalworker.ShouldWorkerUninstall,
	}
}

// Validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.BrokerName == "" {
		return errors.New("not valid empty BrokerName").Add(coreerrors.NotValid)
	}
	if config.DomainServicesName == "" {
		return errors.New("not valid empty DomainServicesName").Add(
			coreerrors.NotValid,
		)
	}
	if config.NewWorker == nil {
		return errors.New("not valid nil NewWorker").Add(coreerrors.NotValid)
	}
	if config.Logger == nil {
		return errors.New("not valid nil Logger").Add(coreerrors.NotValid)
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (config ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Errorf("validating manifold configuration: %w", err)
	}

	var broker caas.Broker
	if err := getter.Get(config.BrokerName, &broker); err != nil {
		return nil, errors.Errorf(
			"getting caas broker for input name %q: %w", config.BrokerName, err,
		)
	}

	var domainServices services.ModelDomainServices
	if err := getter.Get(config.DomainServicesName, &domainServices); err != nil {
		return nil, errors.Errorf(
			"getting domain services for input name %q: %w",
			config.DomainServicesName, err,
		)
	}

	w, err := config.NewWorker(Config{
		PortService:        domainServices.Port(),
		ApplicationService: domainServices.Application(),
		Broker:             broker,
		Logger:             config.Logger,
	})
	if err != nil {
		return nil, errors.Errorf("starting new worker for manifold: %w", err)
	}
	return w, nil
}
