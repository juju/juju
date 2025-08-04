// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/services"
	internalworker "github.com/juju/juju/internal/worker"
)

// ManifoldConfig describes the resources used by the firewaller worker.
type ManifoldConfig struct {
	BrokerName         string
	DomainServicesName string

	ControllerUUID string
	ModelUUID      string

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
	if config.ControllerUUID == "" {
		return errors.NotValidf("empty ControllerUUID")
	}
	if config.ModelUUID == "" {
		return errors.NotValidf("empty ModelUUID")
	}
	if config.BrokerName == "" {
		return errors.NotValidf("empty BrokerName")
	}
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
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

	var broker caas.Broker
	if err := getter.Get(config.BrokerName, &broker); err != nil {
		return nil, errors.Trace(err)
	}

	var domainServices services.ModelDomainServices
	if err := getter.Get(config.DomainServicesName, &domainServices); err != nil {
		return nil, errors.Trace(err)
	}

	w, err := config.NewWorker(Config{
		ControllerUUID:     config.ControllerUUID,
		ModelUUID:          config.ModelUUID,
		PortService:        domainServices.Port(),
		ApplicationService: domainServices.Application(),
		Broker:             broker,
		Logger:             config.Logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}
