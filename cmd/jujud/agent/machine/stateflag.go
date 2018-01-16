// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/worker/dependency"
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
