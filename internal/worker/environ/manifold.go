// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environ

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/api/agent/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/storage"
)

// Logger defines the methods used by the pruner worker for logging.
type Logger interface {
	Debugf(string, ...interface{})
	Warningf(string, ...interface{})
}

// ManifoldConfig describes the resources used by a Tracker.
type ManifoldConfig struct {
	APICallerName  string
	NewEnvironFunc environs.NewEnvironFunc
	Logger         Logger
}

// Manifold returns a Manifold that encapsulates a *Tracker and exposes it as
// an environs.Environ resource.
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
			apiSt, err := agent.NewState(apiCaller)
			if err != nil {
				return nil, errors.Trace(err)
			}
			w, err := NewTracker(Config{
				ConfigAPI:      apiSt,
				NewEnvironFunc: config.NewEnvironFunc,
				Logger:         config.Logger,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
	return manifold
}

// manifoldOutput extracts an environs.Environ resource from a *Tracker.
func manifoldOutput(in worker.Worker, out interface{}) error {
	inTracker, ok := in.(*Tracker)
	if !ok {
		return errors.Errorf("expected *environ.Tracker, got %T", in)
	}
	switch result := out.(type) {
	case *environs.Environ:
		*result = inTracker.Environ()
	case *environs.CloudDestroyer:
		*result = inTracker.Environ()
	case *storage.ProviderRegistry:
		*result = inTracker.Environ()
	default:
		return errors.Errorf("expected *environs.Environ, *storage.ProviderRegistry, or *environs.CloudDestroyer, got %T", out)
	}
	return nil
}
