// Copyright 2015-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongomaster

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/master"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the information necessary to run a FlagWorker in
// a dependency.Engine.
type ManifoldConfig struct {
	AgentName string
	ClockName string
	StateName string

	Duration time.Duration
}

// Manifold returns a dependency.Manifold that will run a FlagWorker and
// expose it to clients as an engine.Flag resource.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.ClockName,
			config.AgentName,
			config.StateName,
		},
		Start: config.start,
		Output: func(in worker.Worker, out interface{}) error {
			if w, ok := in.(wrappedWorker); ok {
				in = w.Worker
			}
			return engine.FlagOutput(in, out)
		},
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	var clock clock.Clock
	if err := context.Get(config.ClockName, &clock); err != nil {
		return nil, errors.Trace(err)
	}

	var agent agent.Agent
	if err := context.Get(config.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}
	agentTag := agent.CurrentConfig().Tag()
	machineTag, ok := agentTag.(names.MachineTag)
	if !ok {
		return nil, errors.New("singular flag expected a machine agent")
	}

	var stTracker workerstate.StateTracker
	if err := context.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}
	st, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}
	stMachine, err := st.Machine(machineTag.Id())
	if err != nil {
		stTracker.Done()
		return nil, errors.Trace(err)
	}
	conn := &Conn{st.MongoSession(), stMachine}

	flag, err := master.NewFlagWorker(master.FlagConfig{
		Clock:    clock,
		Conn:     conn,
		Duration: config.Duration,
	})
	if err != nil {
		stTracker.Done()
		return nil, errors.Trace(err)
	}

	go func() {
		flag.Wait()
		stTracker.Done()
	}()
	return wrappedWorker{flag}, nil
}

// wrappedWorker wraps a flag worker, translating ErrRefresh into
// dependency.ErrBounce.
type wrappedWorker struct {
	worker.Worker
}

// Wait is part of the worker.Worker interface.
func (w wrappedWorker) Wait() error {
	err := w.Worker.Wait()
	if errors.Cause(err) == master.ErrRefresh {
		err = dependency.ErrBounce
	}
	return err
}
