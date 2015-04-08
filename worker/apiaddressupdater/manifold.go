// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddressupdater

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/dependency"
)

type ManifoldConfig struct {
	AgentName         string
	ApiConnectionName string
}

func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.ApiConnectionName,
		},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			var agent agent.Agent
			if err := getResource(config.AgentName, &agent); err != nil {
				return nil, err
			}
			var apiConnection *api.State
			if err := getResource(config.ApiConnectionName, &apiConnection); err != nil {
				return nil, err
			}
			// TODO(fwereade): why on earth are we using the uniter facade here?
			uniterFacade, err := apiConnection.Uniter()
			if err != nil {
				return nil, errors.Trace(err)
			}
			return NewAPIAddressUpdater(uniterFacade, agent), nil
		},
	}
}
