// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logforwarder

import (
	"github.com/juju/errors"

	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	StateName     string
	APICallerName string
}

// Manifold returns a dependency manifold that runs a log forwarding
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.StateName, // ...just to force it to run only on the controller.
			config.APICallerName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			var apiCaller base.APICaller
			if err := context.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}

			agentFacade := apiagent.NewState(apiCaller)
			modelConfig, err := agentFacade.ModelConfig()
			if err != nil {
				return nil, errors.Annotate(err, "cannot read environment config")
			}
			if modelConfig.Name() != environs.ControllerModelName {
				return nil, errors.New("model-level log forwarding not supported")
			}

			return nil, errors.Errorf("finish me")
		},
	}
}
