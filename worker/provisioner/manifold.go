// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	apiprovisioner "github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig defines an environment provisioner's dependencies. It's not
// currently clear whether it'll be easier to extend this type to include all
// provisioners, or to create separate (Environ|Container)Manifold[Config]s;
// for now we dodge the question because we don't need container provisioners
// in dependency engines. Yet.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string
}

// Manifold creates a manifold that runs an environemnt provisioner. See the
// ManifoldConfig type for discussion about how this can/should evolve.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.AgentName, config.APICallerName},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			var agent agent.Agent
			if err := getResource(config.AgentName, &agent); err != nil {
				return nil, errors.Trace(err)
			}
			var apiCaller base.APICaller
			if err := getResource(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}
			api := apiprovisioner.NewState(apiCaller)
			config := agent.CurrentConfig()
			w, err := NewEnvironProvisioner(api, config)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}
