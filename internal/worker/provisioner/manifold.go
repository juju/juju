// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	apiprovisioner "github.com/juju/juju/api/agent/provisioner"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/worker/common"
)

// ManifoldConfig defines an environment provisioner's dependencies. It's not
// currently clear whether it'll be easier to extend this type to include all
// provisioners, or to create separate (Environ|Container)Manifold[Config]s;
// for now we dodge the question because we don't need container provisioners
// in dependency engines. Yet.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string
	EnvironName   string
	Logger        logger.Logger

	NewProvisionerFunc           func(ControllerAPI, MachinesAPI, ToolsFinder, DistributionGroupFinder, agent.Config, logger.Logger, Environ, common.CredentialAPI) (Provisioner, error)
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
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			var agent agent.Agent
			if err := getter.Get(config.AgentName, &agent); err != nil {
				return nil, errors.Trace(err)
			}

			var apiCaller base.APICaller
			if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}

			var environ environs.Environ
			if err := getter.Get(config.EnvironName, &environ); err != nil {
				return nil, errors.Trace(err)
			}

			api := apiprovisioner.NewClient(apiCaller)
			agentConfig := agent.CurrentConfig()

			credentialAPI, err := config.NewCredentialValidatorFacade(apiCaller)
			if err != nil {
				return nil, errors.Trace(err)
			}

			w, err := config.NewProvisionerFunc(api, api, api, api, agentConfig, config.Logger, environ, credentialAPI)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}
