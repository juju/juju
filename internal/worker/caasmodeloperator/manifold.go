// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller/caasmodeloperator"
	"github.com/juju/juju/caas"
)

// Logger is the interface this work requires for logging.
type Logger interface {
	Debugf(string, ...interface{})
}

// ManifoldConfig describes the resources used by the CAASModelOperatorWorker
type ManifoldConfig struct {
	// AgentName
	AgentName string
	// APICallerName is the name of the api caller dependency to fetch
	APICallerName string
	// BrokerName is the name of the api caller dependency to fetch
	BrokerName string
	// Logger to use in this worker
	Logger Logger
	// ModelUUID is the id of the model this worker is operating on

	ModelUUID string
}

// Manifold returns a Manifold that encapsulates a Kubernetes model operator.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
			config.BrokerName,
		},
		Output: nil,
		Start:  config.Start,
	}
}

// Start is used to start the manifold an extract a worker from the supplied
// configuration
func (m ManifoldConfig) Start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := m.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var agent agent.Agent
	if err := getter.Get(m.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}

	var apiCaller base.APICaller
	if err := getter.Get(m.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}

	var broker caas.Broker
	if err := getter.Get(m.BrokerName, &broker); err != nil {
		return nil, errors.Trace(err)
	}

	api := caasmodeloperator.NewClient(apiCaller)

	return NewModelOperatorManager(
		m.Logger, api, broker, m.ModelUUID, agent.CurrentConfig())
}

// Validate checks all the config fields are valid for the Manifold to start
func (m ManifoldConfig) Validate() error {
	if m.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if m.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if m.BrokerName == "" {
		return errors.NotValidf("empty BrokerName")
	}
	if m.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if m.ModelUUID == "" {
		return errors.NotValidf("empty ModelUUID")
	}
	return nil
}
