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
	APICallerName     string
	UpgradeWaiterName string
	StartAPIWorkers   func(api.Connection) (worker.Worker, error)
}

// APIWorkersManifold starts workers that rely on an API connection
// using a function provided to it. It waits until the machine agent's
// initial upgrade operations have completed (using the upgradewaiter
// manifold).
//
// This manifold exists to start API workers which have not yet been
// ported to work directly with the dependency engine. Once all API
// workers started by StartAPIWorkers have been migrated to the
// dependency engine, this manifold can be removed.
func APIWorkersManifold(config APIWorkersConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
			config.UpgradeWaiterName,
		},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			if config.StartAPIWorkers == nil {
				return nil, errors.New("StartAPIWorkers not specified")
			}

			// Check if upgrades have completed.
			var upgradesDone bool
			if err := getResource(config.UpgradeWaiterName, &upgradesDone); err != nil {
				return nil, err
			}
			if !upgradesDone {
				return nil, dependency.ErrMissing
			}

			// Get API connection.
			var apiConn api.Connection
			if err := getResource(config.APICallerName, &apiConn); err != nil {
				return nil, err
			}

			return config.StartAPIWorkers(apiConn)
		},
	}
}
