// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitterminationworker

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/caasapplication"
	"github.com/juju/juju/worker/uniter"
)

// Logger for logging messages.
type Logger interface {
	Infof(string, ...interface{})
	Errorf(string, ...interface{})
}

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string
	UniterName    string
	Clock         clock.Clock
	Logger        Logger
}

// Validate ensures all the required values for the config are set.
func (config *ManifoldConfig) Validate() error {
	if config.Clock == nil {
		return errors.NotValidf("missing Clock")
	}
	if config.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	return nil
}

// Manifold returns a manifold whose worker returns ErrTerminateAgent
// if a termination signal is received by the process it's running in.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
			config.UniterName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}
			var agent agent.Agent
			if err := context.Get(config.AgentName, &agent); err != nil {
				return nil, err
			}
			var apiConn api.Connection
			if err := context.Get(config.APICallerName, &apiConn); err != nil {
				return nil, err
			}
			var uniter *uniter.Uniter
			if err := context.Get(config.UniterName, &uniter); err != nil {
				return nil, err
			}
			state := caasapplication.NewClient(apiConn)
			return NewWorker(Config{
				Agent:          agent,
				State:          state,
				UnitTerminator: uniter,
				Logger:         config.Logger,
				Clock:          config.Clock,
			}), nil
		},
	}
}
