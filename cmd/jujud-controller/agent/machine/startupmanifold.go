// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/api"
	corelogger "github.com/juju/juju/core/logger"
)

// MachineStartupConfig provides the dependencies for the
// machinestartup manifold.
type MachineStartupConfig struct {
	APICallerName  string
	MachineStartup func(context.Context, api.Connection, corelogger.Logger) error
	Logger         corelogger.Logger
}

func (c MachineStartupConfig) Validate() error {
	if c.APICallerName == "" {
		return errors.NotValidf("missing API Caller name")
	}
	if c.MachineStartup == nil {
		return errors.NotValidf("missing MachineStartup")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	return nil
}

// MachineStartupManifold starts a worker that rely on an API connection
// to complete machine setup.
func MachineStartupManifold(config MachineStartupConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, err
			}
			config.Logger.Debugf(ctx, "Starting machine setup requiring an API connection")

			// Get API connection.
			var apiConn api.Connection
			if err := getter.Get(config.APICallerName, &apiConn); err != nil {
				return nil, err
			}
			if err := config.MachineStartup(ctx, apiConn, config.Logger); err != nil {
				return nil, err
			}
			config.Logger.Debugf(ctx, "Finished machine setup requiring an API connection")
			return nil, dependency.ErrUninstall
		},
	}
}
