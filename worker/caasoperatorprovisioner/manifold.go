// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner

import (
	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/caasoperatorprovisioner"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig defines a CAAS operator provisioner's dependencies.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string
	BrokerName    string

	NewWorker func(Config) (worker.Worker, error)
}

// Validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if config.BrokerName == "" {
		return errors.NotValidf("empty BrokerName")
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

	var agent agent.Agent
	if err := context.Get(config.AgentName, &agent); err != nil {
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

	modelTag, ok := apiCaller.ModelTag()
	if !ok {
		return nil, errors.New("API connection is controller-only (should never happen)")
	}

	api := caasoperatorprovisioner.NewClient(apiCaller)
	agentConfig := agent.CurrentConfig()
	w, err := config.NewWorker(Config{
		Facade:      api,
		Broker:      broker,
		ModelTag:    modelTag,
		AgentConfig: agentConfig,
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
			config.AgentName,
			config.APICallerName,
			config.BrokerName,
		},
		Start: config.start,
	}
}
