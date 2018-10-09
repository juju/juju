// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/caas"
)

// ManifoldConfig defines a CAAS unit provisioner's dependencies.
type ManifoldConfig struct {
	APICallerName string
	BrokerName    string

	NewClient func(base.APICaller) Client
	NewWorker func(Config) (worker.Worker, error)
}

// Validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if config.BrokerName == "" {
		return errors.NotValidf("empty BrokerName")
	}
	if config.NewClient == nil {
		return errors.NotValidf("nil NewClient")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
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

	client := config.NewClient(apiCaller)
	w, err := config.NewWorker(Config{
		ApplicationGetter:  client,
		ApplicationUpdater: client,

		ServiceBroker:   broker,
		ContainerBroker: broker,

		ProvisioningInfoGetter:   client,
		ProvisioningStatusSetter: client,
		LifeGetter:               client,
		UnitUpdater:              client,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Manifold creates a manifold that runs a CAAS unit provisioner. See the
// ManifoldConfig type for discussion about how this can/should evolve.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
			config.BrokerName,
		},
		Start: config.start,
	}
}
