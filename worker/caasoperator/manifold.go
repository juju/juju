// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/uniter/resolver"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	AgentName            string
	APICallerName        string
	Clock                clock.Clock
	TranslateResolverErr func(error) error
	AgentDir             string
}

// Manifold returns a dependency manifold that runs a caasoperator worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			if config.Clock == nil {
				return nil, errors.NotValidf("missing Clock")
			}
			logger.Errorf("collecting resources")
			// Collect all required resources.
			var agent agent.Agent
			if err := context.Get(config.AgentName, &agent); err != nil {
				return nil, err
			}
			var apiConn api.Connection
			if err := context.Get(config.APICallerName, &apiConn); err != nil {
				return nil, err
			}

			manifoldConfig := config
			// Configure and start the caasoperator.
			agentConfig := agent.CurrentConfig()
			tag := agentConfig.Tag()
			caasoperatorTag, ok := tag.(names.ApplicationTag)
			if !ok {
				return nil, errors.Errorf("expected a caasoperator tag, got %v", tag)
			}
			caasoperator, err := NewCaasOperator(&CaasOperatorParams{
				CaasOperatorTag:      caasoperatorTag,
				DataDir:              agentConfig.DataDir(),
				AgentDir:             manifoldConfig.AgentDir,
				TranslateResolverErr: config.TranslateResolverErr,
				Clock:                manifoldConfig.Clock,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return caasoperator, nil
		},
	}
}

// TranslateFortressErrors turns errors returned by dependent
// manifolds due to fortress lockdown (i.e. model migration) into an
// error which causes the resolver loop to be restarted. When this
// happens the caasoperator is about to be shut down anyway.
func TranslateFortressErrors(err error) error {
	if fortress.IsFortressError(err) {
		return resolver.ErrRestart
	}
	return err
}
