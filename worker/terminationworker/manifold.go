// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package terminationworker

import (
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// Manifold returns a manifold whose worker returns ErrTerminateAgent
// if a termination signal is received by the process it's running in.
func Manifold() dependency.Manifold {
	return dependency.Manifold{
		Start: func(dependency.GetResourceFunc) (worker.Worker, error) {
			return NewWorker(), nil
		},
	}
}
