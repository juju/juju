// Copyright 2015-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"
)

// ManifoldConfig holds the information necessary to run a FlagWorker in
// a dependency.Engine.
type ManifoldConfig struct {
	ClockName     string
	APICallerName string
	AgentName     string
	Duration      time.Duration

	NewFacade func(base.APICaller, names.MachineTag) (Facade, error)
	NewWorker func(FlagConfig) (worker.Worker, error)
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	var clock clock.Clock
	if err := context.Get(config.ClockName, &clock); err != nil {
		return nil, errors.Trace(err)
	}
	var apiCaller base.APICaller
	if err := context.Get(config.APICallerName, &apiCaller); err != nil {
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

	// TODO(fwereade): model id is implicit in apiCaller, would
	// be better if explicit.
	facade, err := config.NewFacade(apiCaller, machineTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	flag, err := config.NewWorker(FlagConfig{
		Clock:    clock,
		Facade:   facade,
		Duration: config.Duration,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return flag, nil
}

// Manifold returns a dependency.Manifold that will run a FlagWorker and
// expose it to clients as a engine.Flag resource.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.ClockName,
			config.APICallerName,
			config.AgentName,
		},
		Start:  config.start,
		Output: engine.FlagOutput,
	}
}
