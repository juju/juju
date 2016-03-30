// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// APIWorkersConfig provides the dependencies for the
// apiworkers manifold.
type APIWorkersConfig struct {
	APICallerName   string
	StartAPIWorkers func(api.Connection) (worker.Worker, error)
}

// APIWorkersManifold starts workers that rely on an API connection
// using a function provided to it.
//
// This manifold exists to start API workers which have not yet been
// ported to work directly with the dependency engine. Once all API
// workers started by StartAPIWorkers have been migrated to the
// dependency engine, this manifold can be removed.
func APIWorkersManifold(config APIWorkersConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			if config.StartAPIWorkers == nil {
				return nil, errors.New("StartAPIWorkers not specified")
			}

			// Get API connection.
			// XXX(fwereade): what is this Connection actually
			// needed for? should generally be base.APICaller...
			var apiConn api.Connection
			if err := context.Get(config.APICallerName, &apiConn); err != nil {
				return nil, err
			}
			return config.StartAPIWorkers(apiConn)
		},
	}
}
