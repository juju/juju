// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/api/base"
	apicaasapplicationprovisioner "github.com/juju/juju/api/controller/caasapplicationprovisioner"
	caasunitprovisionerapi "github.com/juju/juju/api/controller/caasunitprovisioner"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/services"
)

// ManifoldConfig defines a CAAS operator provisioner's dependencies.
type ManifoldConfig struct {
	APICallerName      string
	DomainServicesName string
	BrokerName         string
	ClockName          string
	NewWorker          func(Config) (worker.Worker, error)
	Logger             logger.Logger
}

// Validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if config.BrokerName == "" {
		return errors.NotValidf("empty BrokerName")
	}
	if config.ClockName == "" {
		return errors.NotValidf("empty ClockName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

func (config ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var apiCaller base.APICaller
	if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}

	var domainServices services.ModelDomainServices
	if err := getter.Get(config.DomainServicesName, &domainServices); err != nil {
		return nil, errors.Trace(err)
	}

	var clock clock.Clock
	if err := getter.Get(config.ClockName, &clock); err != nil {
		return nil, errors.Trace(err)
	}

	var broker caas.Broker
	if err := getter.Get(config.BrokerName, &broker); err != nil {
		return nil, errors.Trace(err)
	}

	modelTag, ok := apiCaller.ModelTag()
	if !ok {
		return nil, errors.New("API connection is controller-only (should never happen)")
	}

	w, err := config.NewWorker(Config{
		ApplicationService: domainServices.Application(),
		StatusService:      domainServices.Status(),
		Facade:             apicaasapplicationprovisioner.NewClient(apiCaller),
		Broker:             broker,
		ModelTag:           modelTag,
		Clock:              clock,
		Logger:             config.Logger,
		NewAppWorker:       NewAppWorker,
		UnitFacade:         caasunitprovisionerapi.NewClient(apiCaller),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Manifold creates a manifold that runs a CAAS operator provisioner. See the
// ManifoldConfig type for discussion about how this can/should evolve.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
			config.DomainServicesName,
			config.BrokerName,
			config.ClockName,
			config.DomainServicesName,
		},
		Start: config.start,
	}
}
