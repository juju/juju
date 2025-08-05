// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fanconfigurer

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	apifanconfigurer "github.com/juju/juju/api/agent/fanconfigurer"
	"github.com/juju/juju/api/base"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	// These are the dependency resource names.
	APICallerName string
	AgentName     string
	Clock         clock.Clock
}

// Manifold returns a dependency manifold that runs a fan configurer
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
			config.AgentName,
		},
		Output: func(in worker.Worker, out interface{}) error {
			inWorker, _ := in.(*FanConfigurer)
			if inWorker == nil {
				return errors.Errorf("in should be a %T; got %T", inWorker, in)
			}
			switch outPointer := out.(type) {
			case *bool:
				*outPointer = true
			default:
				return errors.Errorf("out should be *bool; got %T", out)
			}
			return nil
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			var apiCaller base.APICaller
			if err := context.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}

			var agent agent.Agent
			if err := context.Get(config.AgentName, &agent); err != nil {
				return nil, errors.Trace(err)
			}
			machineTag, ok := agent.CurrentConfig().Tag().(names.MachineTag)
			if !ok {
				return nil, errors.Errorf("agent tag should be a MachineTag; got %T", agent.CurrentConfig().Tag())
			}

			facade := apifanconfigurer.NewFacade(apiCaller)

			fanconfigurer, err := NewFanConfigurer(FanConfigurerConfig{
				Facade: facade,
				Tag:    machineTag,
			}, config.Clock)
			return fanconfigurer, errors.Annotate(err, "creating fanconfigurer orchestrator")
		},
	}
}
