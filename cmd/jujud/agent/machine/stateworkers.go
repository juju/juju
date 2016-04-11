// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/errors"

	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	workerstate "github.com/juju/juju/worker/state"
)

// StateWorkersConfig provides the dependencies for the
// stateworkers manifold.
type StateWorkersConfig struct {
	StateName         string
	StartStateWorkers func(*state.State) (worker.Worker, error)
}

// StateWorkersManifold starts workers that rely on a *state.State
// using a function provided to it.
//
// This manifold exists to start State-using workers which have not
// yet been ported to work directly with the dependency engine. Once
// all state workers started by StartStateWorkers have been migrated
// to the dependency engine, this manifold can be removed.
func StateWorkersManifold(config StateWorkersConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.StateName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			if config.StartStateWorkers == nil {
				return nil, errors.New("StartStateWorkers not specified")
			}

			var stTracker workerstate.StateTracker
			if err := context.Get(config.StateName, &stTracker); err != nil {
				return nil, err
			}

			st, err := stTracker.Use()
			if err != nil {
				return nil, errors.Annotate(err, "acquiring state")
			}

			w, err := config.StartStateWorkers(st)
			if err != nil {
				stTracker.Done()
				return nil, errors.Annotate(err, "worker startup")
			}

			// When the state workers are done, indicate that we no
			// longer need the State.
			go func() {
				w.Wait()
				stTracker.Done()
			}()

			return w, nil
		},
	}
}
