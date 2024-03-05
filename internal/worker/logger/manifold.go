// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"context"

	"github.com/juju/loggo/v2"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/agent/logger"
	"github.com/juju/juju/api/base"
)

// Logger represents a loggo logger for the purpose of recording what is going
// on.
type Logger interface {
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
	Warningf(string, ...interface{})
	Errorf(string, ...interface{})
}

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	AgentName       string
	APICallerName   string
	LoggingContext  *loggo.Context
	Logger          Logger
	UpdateAgentFunc func(string) error
}

// Manifold returns a dependency manifold that runs a logger
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			var a agent.Agent
			if err := getter.Get(config.AgentName, &a); err != nil {
				return nil, err
			}
			currentConfig := a.CurrentConfig()
			loggingOverride := currentConfig.Value(agent.LoggingOverride)

			var apiCaller base.APICaller
			if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, err
			}

			loggerFacade := logger.NewClient(apiCaller)
			workerConfig := WorkerConfig{
				Context:  config.LoggingContext,
				API:      loggerFacade,
				Tag:      currentConfig.Tag(),
				Logger:   config.Logger,
				Override: loggingOverride,
				Callback: config.UpdateAgentFunc,
			}
			return NewLogger(workerConfig)
		},
	}
}
