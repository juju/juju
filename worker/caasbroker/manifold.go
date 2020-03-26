// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasbroker

import (
	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/caasagent"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/worker/caasadmission"
	"github.com/juju/juju/worker/caasrbacmapper"
)

// Logger represents the methods used by the worker to log details.
type Logger interface {
	Debugf(string, ...interface{})
	Warningf(string, ...interface{})
}

// ManifoldConfig describes the resources used by a Tracker.
type ManifoldConfig struct {
	APICallerName          string
	NewContainerBrokerFunc caas.NewContainerBrokerFunc
	Logger                 Logger
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
			api, err := caasagent.NewClient(apiCaller)
			if err != nil {
				return nil, errors.Trace(err)
			}
			w, err := NewTracker(Config{
				ConfigAPI:              api,
				NewContainerBrokerFunc: config.NewContainerBrokerFunc,
				Logger:                 config.Logger,
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
	switch result := out.(type) {
	case *caas.Broker:
		*result = inTracker.Broker()
	case *caasadmission.K8sBroker:
		*result = inTracker.Broker().(caasadmission.K8sBroker)
	case *caasrbacmapper.K8sBroker:
		*result = inTracker.Broker().(caasrbacmapper.K8sBroker)
	case *environs.CloudDestroyer:
		*result = inTracker.Broker()
	case *storage.ProviderRegistry:
		*result = inTracker.Broker()
	default:
		return errors.Errorf("expected *caas.Broker, *storage.ProviderRegistry or *environs.CloudDestroyer, got %T", out)
	}
	return nil
}
