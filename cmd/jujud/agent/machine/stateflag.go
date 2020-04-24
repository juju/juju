// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/cmd/jujud/agent/engine"
)

// isControllerFlagManifold returns a dependency.Manifold which requires
// State, and returns a worker implementing engine.Flag, whose Check method
// always returns true. This is used for flagging that the machine is a
// controller/model manager.
func isControllerFlagManifold() dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{stateName},
		Output: engine.FlagOutput,
		Start: func(context dependency.Context) (worker.Worker, error) {
			if err := context.Get(stateName, nil); err != nil {
				return nil, err
			}
			return engine.NewStaticFlagWorker(true), nil
		},
	}
}
