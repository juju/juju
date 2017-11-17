// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasbroker

import (
	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig describes the resources used by a Tracker.
type ManifoldConfig struct {
	APICallerName          string
	NewContainerBrokerFunc caas.NewContainerBrokerFunc
}

// Manifold returns a Manifold that encapsulates a *Tracker and exposes it as
// an caas,Broker resource.
func Manifold(config ManifoldConfig) dependency.Manifold {
	manifold := dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
		},
		Output: manifoldOutput,
		Start: func(context dependency.Context) (worker.Worker, error) {
			var apiCaller base.APICaller
			if err := context.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}
			// TODO(caas) introduce a caasbroker API
			apiSt, err := agent.NewState(apiCaller)
			if err != nil {
				return nil, errors.Trace(err)
			}
			w, err := NewTracker(Config{
				Observer:               apiSt,
				NewContainerBrokerFunc: config.NewContainerBrokerFunc,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
	return manifold
}

// manifoldOutput extracts an caas.Broker resource from a *Tracker.
func manifoldOutput(in worker.Worker, out interface{}) error {
	inTracker, ok := in.(*Tracker)
	if !ok {
		return errors.Errorf("expected *broker.Tracker, got %T", in)
	}
	outBroker, ok := out.(*caas.Broker)
	if !ok {
		return errors.Errorf("expected *caas.Broker, got %T", out)
	}
	*outBroker = inTracker.Broker()
	return nil
}
