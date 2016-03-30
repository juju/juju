// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environ

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig describes the resources used by a Tracker.
type ManifoldConfig struct {
	APICallerName      string
	NewEnvironFuncName string
}

// Manifold returns a Manifold that encapsulates a *Tracker and exposes it as
// an environs.Environ resource.
func Manifold(config ManifoldConfig) dependency.Manifold {
	manifold := dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
			config.NewEnvironFuncName,
		},
		Output: manifoldOutput,
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			var apiCaller base.APICaller
			if err := getResource(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}
			var newEnvironFunc NewEnvironFunc
			if err := getResource(config.NewEnvironFuncName, &newEnvironFunc); err != nil {
				return nil, errors.Trace(err)
			}
			w, err := NewTracker(Config{
				Observer:       agent.NewState(apiCaller),
				NewEnvironFunc: newEnvironFunc,
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
	outEnviron, ok := out.(*environs.Environ)
	if !ok {
		return errors.Errorf("expected *environs.Environ, got %T", out)
	}
	*outEnviron = inTracker.Environ()
	return nil
}
