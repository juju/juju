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
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig describes the resources used by a Tracker.
type ManifoldConfig util.ApiManifoldConfig

// Manifold returns a Manifold that encapsulates a *Tracker and exposes it as
// an environs.Environ resource.
func Manifold(config ManifoldConfig) dependency.Manifold {
	manifold := util.ApiManifold(
		util.ApiManifoldConfig(config),
		manifoldStart,
	)
	manifold.Output = manifoldOutput
	return manifold
}

// manifoldStart creates a *Tracker given a base.APICaller.
func manifoldStart(apiCaller base.APICaller) (worker.Worker, error) {
	w, err := NewTracker(Config{
		Observer: agent.NewState(apiCaller),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
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
