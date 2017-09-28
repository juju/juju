// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environ

import (
	"github.com/juju/errors"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig describes the resources used by a Tracker.
type ManifoldConfig struct {
	APICallerName  string
	NewEnvironFunc environs.NewEnvironFunc
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
				Observer:       apiSt,
				NewEnvironFunc: config.NewEnvironFunc,
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
	outEnviron, ok := out.(*environs.IAASEnviron)
	if !ok {
		outEnviron, ok := out.(*environs.Environ)
		if !ok {
			return errors.Errorf("expected *environs.Environ or *environs.IAASEnviron, got %T", out)
		}
		*outEnviron = inTracker.Environ()
	}
	inEnv := inTracker.Environ()
	inIAASEnv, ok := inEnv.(environs.IAASEnviron)
	if !ok {
		return errors.Errorf("could not cast Tracker's environ into an IAASEnviron")
	}
	*outEnviron = inIAASEnv
	return nil
}
