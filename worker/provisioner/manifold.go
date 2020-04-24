// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	apiprovisioner "github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker/common"
)

// Logger defines the logging methods that the worker uses.
type Logger interface {
	Tracef(string, ...interface{})
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
	Warningf(string, ...interface{})
	Errorf(string, ...interface{})
}

// ManifoldConfig defines an environment provisioner's dependencies. It's not
// currently clear whether it'll be easier to extend this type to include all
// provisioners, or to create separate (Environ|Container)Manifold[Config]s;
// for now we dodge the question because we don't need container provisioners
// in dependency engines. Yet.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string
	EnvironName   string
	Logger        Logger

	NewProvisionerFunc           func(*apiprovisioner.State, agent.Config, Logger, environs.Environ, common.CredentialAPI) (Provisioner, error)
	NewCredentialValidatorFacade func(base.APICaller) (common.CredentialAPI, error)
}

// Manifold creates a manifold that runs an environment provisioner. See the
// ManifoldConfig type for discussion about how this can/should evolve.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
			config.EnvironName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			var agent agent.Agent
			if err := context.Get(config.AgentName, &agent); err != nil {
				return nil, errors.Trace(err)
			}

			var apiCaller base.APICaller
			if err := context.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}

			var environ environs.Environ
			if err := context.Get(config.EnvironName, &environ); err != nil {
				return nil, errors.Trace(err)
			}

			api := apiprovisioner.NewState(apiCaller)
			agentConfig := agent.CurrentConfig()

			credentialAPI, err := config.NewCredentialValidatorFacade(apiCaller)
			if err != nil {
				return nil, errors.Trace(err)
			}

			w, err := config.NewProvisionerFunc(api, agentConfig, config.Logger, environ, credentialAPI)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}
