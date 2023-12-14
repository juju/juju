// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/api"
)

// Logger represents the logging methods used by this manifold.
type Logger interface {
	Debugf(string, ...interface{})
	Warningf(string, ...interface{})
	Criticalf(string, ...interface{})
	Tracef(string, ...interface{})
}

// MachineStartupConfig provides the dependencies for the
// machinestartup manifold.
type MachineStartupConfig struct {
	APICallerName  string
	MachineStartup func(api.Connection, Logger) error
	Logger         Logger
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
		Start: func(context dependency.Context) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, err
			}
			config.Logger.Debugf("Starting machine setup requiring an API connection")

			// Get API connection.
			var apiConn api.Connection
			if err := context.Get(config.APICallerName, &apiConn); err != nil {
				return nil, err
			}
			if err := config.MachineStartup(apiConn, config.Logger); err != nil {
				return nil, err
			}
			config.Logger.Debugf("Finished machine setup requiring an API connection")
			return nil, dependency.ErrUninstall
		},
	}
}
