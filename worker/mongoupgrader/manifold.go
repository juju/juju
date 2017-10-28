// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongoupgrader

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/worker/dependency"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the information necessary to run a mongo
// upgrader worker in a dependency.Engine.
type ManifoldConfig struct {
	StateName string
	Machine   names.MachineTag
	StopMongo StopMongo
}

func (config ManifoldConfig) Validate() error {
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.Machine == (names.MachineTag{}) {
		return errors.NotValidf("empty MachineTag")
	}
	if config.StopMongo == nil {
		return errors.NotValidf("nil StopMongo")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run a mongo
// upgrader worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.StateName,
		},
		Start: config.start,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var stTracker workerstate.StateTracker
	if err := context.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}
	st, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}

	worker := New(st, config.Machine.Id(), config.StopMongo)
	go func() {
		worker.Wait()
		stTracker.Done()
	}()
	return worker, nil
}
