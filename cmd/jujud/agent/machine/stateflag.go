// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/v3/cmd/jujud/agent/engine"
)

// isControllerFlagManifold returns a dependency.Manifold that requires state
// config.
// It returns a worker implementing engine.Flag, whose Check method returns
// whether state config is present on the machine.
func isControllerFlagManifold() dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{stateConfigWatcherName},
		Output: engine.FlagOutput,
		Start: func(context dependency.Context) (worker.Worker, error) {
			var haveStateConfig bool
			if err := context.Get(stateConfigWatcherName, &haveStateConfig); err != nil {
				return nil, err
			}
			if !haveStateConfig {
				return nil, errors.Annotate(dependency.ErrMissing, "no state config detected")
			}
			return engine.NewStaticFlagWorker(true), nil
		},
	}
}
