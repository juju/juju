// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasbroker

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/api/agent/caasagent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/worker/caasadmission"
	"github.com/juju/juju/internal/worker/caasrbacmapper"
)

// ManifoldConfig describes the resources used by a Tracker.
type ManifoldConfig struct {
	APICallerName          string
	NewContainerBrokerFunc caas.NewContainerBrokerFunc
	Logger                 logger.Logger
}

// Manifold returns a Manifold that encapsulates a *Tracker and exposes it as
// a caas.Broker resource.
func Manifold(config ManifoldConfig) dependency.Manifold {
	manifold := dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
		},
		Output: manifoldOutput,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			var apiCaller base.APICaller
			if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
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
