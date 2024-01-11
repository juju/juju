// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package terminationworker

import (
	"context"

	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
)

// Manifold returns a manifold whose worker returns ErrTerminateAgent
// if a termination signal is received by the process it's running in.
func Manifold() dependency.Manifold {
	return dependency.Manifold{
		Start: func(_ context.Context, getter dependency.Getter) (worker.Worker, error) {
			return NewWorker(), nil
		},
	}
}
