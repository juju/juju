// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spacenamer

import (
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/spacenamer"
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
	AgentName     string
	APICallerName string
	Logger        Logger
	NewWorker     func(WorkerConfig) (worker.Worker, error)
	NewClient     func(base.APICaller) SpaceNamerAPI
}

// Manifold returns a dependency manifold that runs a logger
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			var a agent.Agent
			if err := context.Get(config.AgentName, &a); err != nil {
				return nil, err
			}
			currentConfig := a.CurrentConfig()

			var apiCaller base.APICaller
			if err := context.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, err
			}

			spaceNamerFacade := spacenamer.NewClient(apiCaller)
			workerConfig := WorkerConfig{
				API:    spaceNamerFacade,
				Tag:    currentConfig.Tag(),
				Logger: config.Logger,
			}
			return NewWorker(workerConfig)
		},
	}
}
