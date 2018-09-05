// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"fmt"

	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"

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
				// fmt.Printf("isControllerFlagManifold: stateName -> %q, err -> %#v, cause -> %#v\n", stateName, err, errors.Cause(err))
				return nil, err
			}
			fmt.Printf("isControllerFlagManifold: stateName  all good -> %q\n", stateName)
			return engine.NewStaticFlagWorker(true), nil
		},
	}
}
