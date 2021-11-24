// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"path/filepath"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/pubsub/v2"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/worker/common"
)

// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...interface{})
	Infof(message string, args ...interface{})
	Warningf(message string, args ...interface{})
	Tracef(message string, args ...interface{})
}

// ManifoldConfig defines the names of the manifolds on which a Manifold will
// depend.
type ManifoldConfig struct {
	AgentName      string
	CentralHubName string
	Clock          clock.Clock
	Logger         Logger
}

// Manifold returns a dependency manifold that runs the dbaccessor
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.CentralHubName,
			config.AgentName,
		},
		Output: dbAccessorOutput,
		Start: func(context dependency.Context) (worker.Worker, error) {
			var agent coreagent.Agent
			if err := context.Get(config.AgentName, &agent); err != nil {
				return nil, err
			}
			agentConfig := agent.CurrentConfig()

			// Get the hub.
			var hub *pubsub.StructuredHub
			if err := context.Get(config.CentralHubName, &hub); err != nil {
				return nil, err
			}

			cfg := WorkerConfig{
				AgentMachineID: agentConfig.Tag().Id(),
				DataDir:        filepath.Join(agentConfig.DataDir(), "dqlite"),
				Hub:            hub,
				Clock:          config.Clock,
				Logger:         config.Logger,
			}

			w, err := NewWorker(cfg)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}

func dbAccessorOutput(in worker.Worker, out interface{}) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	w, ok := in.(*dbWorker)
	if !ok {
		return errors.Errorf("expected input of type dbWorker, got %T", in)
	}

	switch out := out.(type) {
	case *DBGetter:
		var target DBGetter = w
		*out = target
	default:
		return errors.Errorf("expected output of *dbaccessor.DBGetter, got %T", out)
	}
	return nil
}
