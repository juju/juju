// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package util

import (
	"context"

	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent/engine"
)

// IsControllerFlagManifold returns a dependency.Manifold that indicates
// the state config is present or not depending on the arg.
// It returns a worker implementing engine.Flag, whose Check method returns
// True in 2 cases:
// 1) state config is present on the machine and arg is True
// 2) state config is not present on the machine and arg is False.
func IsControllerFlagManifold(inputName string, yes bool) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{inputName},
		Output: engine.FlagOutput,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			var haveStateConfig bool
			if err := getter.Get(inputName, &haveStateConfig); err != nil {
				return nil, err
			}
			return engine.NewStaticFlagWorker(haveStateConfig && yes || !haveStateConfig && !yes), nil
		},
	}
}
