// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/api/agent/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/storage"
)

// Logger defines the methods used by the pruner worker for logging.
type Logger interface {
	Debugf(string, ...interface{})
	Warningf(string, ...interface{})
}

// ManifoldConfig describes the resources used by a Worker.
type ManifoldConfig struct {
	APICallerName              string
	ProviderServiceFactoryName string
	NewEnvironFunc             environs.NewEnvironFunc
	Logger                     Logger
}

// Manifold returns a Manifold that encapsulates a *Worker and exposes it as
// an environs.Environ resource.
func Manifold(config ManifoldConfig) dependency.Manifold {
	manifold := dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
			config.ProviderServiceFactoryName,
		},
		Output: manifoldOutput,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			var apiCaller base.APICaller
			if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}
			apiSt, err := agent.NewClient(apiCaller)
			if err != nil {
				return nil, errors.Trace(err)
			}
			w, err := NewWorker(ctx, Config{
				Observer:       apiSt,
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

// manifoldOutput extracts an environs.Environ resource from a *Worker.
func manifoldOutput(in worker.Worker, out interface{}) error {
	w, ok := in.(*Worker)
	if !ok {
		return errors.Errorf("expected *environ.Tracker, got %T", in)
	}
	switch result := out.(type) {
	case *environs.Environ:
		*result = w.Environ()
	case *environs.CloudDestroyer:
		*result = w.Environ()
	case *storage.ProviderRegistry:
		*result = w.Environ()
	default:
		return errors.Errorf("expected *environs.Environ, *storage.ProviderRegistry, or *environs.CloudDestroyer, got %T", out)
	}
	return nil
}
