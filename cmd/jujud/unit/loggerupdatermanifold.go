// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit

import (
	"github.com/juju/juju/api/base"
	apilogger "github.com/juju/juju/api/logger"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/logger"
)

// LoggerUpdaterManifoldConfig defines the names of the manifolds on which a
// LoggerUpdaterManifold will depend.
type LoggerUpdaterManifoldConfig struct {
	AgentName     string
	ApiCallerName string
}

// LoggerUpdaterManifold returns a dependency manifold that runs a logger
// worker, using the resource names defined in the supplied config.
//
// It should really be defined in worker/logger instead, but import loops render
// this impractical for the time being.
func LoggerUpdaterManifold(config LoggerUpdaterManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.ApiCallerName,
		},
		Start: loggerUpdaterStartFunc(config),
	}
}

// loggerUpdaterStartFunc returns a StartFunc that creates a logger updater worker
// based on the manifolds named in the supplied config.
func loggerUpdaterStartFunc(config LoggerUpdaterManifoldConfig) dependency.StartFunc {
	return func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
		var agent agent.Agent
		if err := getResource(config.AgentName, &agent); err != nil {
			return nil, err
		}
		var apiCaller base.APICaller
		if err := getResource(config.ApiCallerName, &apiCaller); err != nil {
			return nil, err
		}
		return logger.NewLogger(apilogger.NewState(apiCaller), agent.CurrentConfig()), nil
	}
}
