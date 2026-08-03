// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pebblelokiconfig

import (
	"context"

	"github.com/canonical/pebble/client"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/agent/logger"
	"github.com/juju/juju/api/base"
	corelogger "github.com/juju/juju/core/logger"
)

// DefaultPebbleClient is the production implementation of NewPebbleClientFunc.
func DefaultPebbleClient(socketPath string) (PebbleClient, error) {
	c, err := client.New(&client.Config{Socket: socketPath})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &pebbleClientWrapper{Client: c}, nil
}

// pebbleClientWrapper adapts *client.Client to the PebbleClient interface.
type pebbleClientWrapper struct {
	*client.Client
}

func (w *pebbleClientWrapper) CloseIdleConnections() {
	w.Client.CloseIdleConnections()
}

// ManifoldConfig defines the names of the manifolds on which a Manifold will
// depend.
type ManifoldConfig struct {
	AgentName       string
	APICallerName   string
	Clock           clock.Clock
	Logger          corelogger.Logger
	PebbleSocket    string
	NewPebbleClient NewPebbleClientFunc
}

// Validate ensures all the necessary fields have values.
func (c ManifoldConfig) Validate() error {
	if c.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if c.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if c.Clock == nil {
		return errors.NotValidf("missing Clock")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	if c.NewPebbleClient == nil {
		return errors.NotValidf("missing NewPebbleClient")
	}
	return nil
}

// Manifold returns a dependency manifold that runs a pebble-loki-config
// worker using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var a agent.Agent
			if err := getter.Get(config.AgentName, &a); err != nil {
				return nil, err
			}

			var apiCaller base.APICaller
			if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, err
			}

			return NewWorker(WorkerConfig{
				Agent:            a,
				API:              logger.NewClient(apiCaller),
				Clock:            config.Clock,
				Logger:           config.Logger,
				NewPebbleClient:  config.NewPebbleClient,
				PebbleSocketPath: config.PebbleSocket,
			})
		},
	}
}
