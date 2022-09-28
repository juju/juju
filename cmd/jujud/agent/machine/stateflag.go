// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/cmd/jujud/agent/engine"
)

// isControllerFlagManifold returns a dependency.Manifold that indicates
// the state config is present or not depending on the arg.
// It returns a worker implementing engine.Flag, whose Check method returns
// True in 2 cases:
// 1) state config is present on the machine and arg is True
// 2) state config is not present on the machine and arg is False.
func isControllerFlagManifold(yes bool) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{stateConfigWatcherName},
		Output: engine.FlagOutput,
		Start: func(context dependency.Context) (worker.Worker, error) {
			var haveStateConfig bool
			if err := context.Get(stateConfigWatcherName, &haveStateConfig); err != nil {
				return nil, err
			}
			return engine.NewStaticFlagWorker(haveStateConfig && yes || !haveStateConfig && !yes), nil
		},
	}
}
