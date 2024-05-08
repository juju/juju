// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitsmanager

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/core/logger"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string

	Logger logger.Logger
	Clock  clock.Clock

	Hub Hub
}

// Validate ensures all the required values for the config are set.
func (config *ManifoldConfig) Validate() error {
	if config.Clock == nil {
		return errors.NotValidf("missing Clock")
	}
	if config.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	if config.Hub == nil {
		return errors.NotValidf("missing Hub")
	}
	return nil
}

// Manifold returns a dependency manifold that runs a caasunitmanager
// worker, using the resource names defined in the supplied config.
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

			return NewWorker(Config{
				Logger: config.Logger,
				Clock:  config.Clock,
				Hub:    config.Hub,
			})
		},
	}
}
