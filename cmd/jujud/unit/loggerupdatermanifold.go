// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit

import (
	"github.com/juju/juju/api"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/logger"
)

type LoggerUpdaterManifoldConfig struct {
	AgentName         string
	ApiConnectionName string
}

func LoggerUpdaterManifold(config LoggerUpdaterManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.ApiConnectionName,
		},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			var agent agent.Agent
			if !getResource(config.AgentName, &agent) {
				return nil, dependency.ErrUnmetDependencies
			}
			var apiConnection *api.State
			if !getResource(config.ApiConnectionName, &apiConnection) {
				return nil, dependency.ErrUnmetDependencies
			}
			return logger.NewLogger(apiConnection.Logger(), agent.CurrentConfig()), nil
		},
	}
}
