// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackendrotate

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller/secretsbackendmanager"
)

// ManifoldConfig holds dependencies and configuration for a
// secretbackendrotate worker.
type ManifoldConfig struct {
	Logger        Logger
	APICallerName string
}

// Manifold returns a dependency.Manifold that runs a secretbackendrotate worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
		},
		Start: config.start,
	}
}

func (c ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := c.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var apiCaller base.APICaller
	if err := context.Get(c.APICallerName, &apiCaller); err != nil {
		return nil, err
	}
	return NewWorker(Config{
		SecretBackendManagerFacade: secretsbackendmanager.NewClient(apiCaller),
		Logger:                     c.Logger,
		Clock:                      clock.WallClock,
	})
}

// Validate validates a manifold config.
func (c ManifoldConfig) Validate() error {
	if c.APICallerName == "" {
		return errors.NotValidf("missing APICallerName")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}
