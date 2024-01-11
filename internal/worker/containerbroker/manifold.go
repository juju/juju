// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerbroker

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/agent/provisioner"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/container/broker"
)

// ManifoldConfig describes the resources used by a Tracker.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string
	MachineLock   machinelock.Lock
	NewBrokerFunc func(broker.Config) (environs.InstanceBroker, error)
	NewTracker    func(Config) (worker.Worker, error)
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if config.MachineLock == nil {
		return errors.NotValidf("nil MachineLock")
	}
	if config.NewBrokerFunc == nil {
		return errors.NotValidf("nil NewBrokerFunc")
	}
	if config.NewTracker == nil {
		return errors.NotValidf("nil NewTracker")
	}
	return nil
}

// Manifold returns a Manifold that encapsulates a *Tracker and exposes it as
// an Broker resource.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
		},
		Output: manifoldOutput,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var agent agent.Agent
			if err := getter.Get(config.AgentName, &agent); err != nil {
				return nil, errors.Trace(err)
			}
			var apiCaller base.APICaller
			if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}
			w, err := config.NewTracker(Config{
				APICaller:     apiCaller,
				AgentConfig:   agent.CurrentConfig(),
				MachineLock:   config.MachineLock,
				NewBrokerFunc: config.NewBrokerFunc,
				NewStateFunc: func(apiCaller base.APICaller) State {
					return provisioner.NewClient(apiCaller)
				},
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}

// manifoldOutput extracts an environs.Environ resource from a *Tracker.
func manifoldOutput(in worker.Worker, out interface{}) error {
	inTracker, ok := in.(*Tracker)
	if !ok {
		return errors.Errorf("expected *broker.Tracker, got %T", in)
	}
	switch result := out.(type) {
	case *environs.InstanceBroker:
		*result = inTracker.Broker()
	default:
		return errors.Errorf("expected *environs.InstanceBroker got %T", out)
	}
	return nil
}
