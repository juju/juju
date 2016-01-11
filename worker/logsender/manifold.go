// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsender

import (
	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/logsender"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will
// depend.
type ManifoldConfig struct {
	util.PostUpgradeManifoldConfig
	LogSource LogRecordCh
}

// Manifold returns a dependency manifold that runs a logger worker, using the
// resource names defined in the supplied config. The DB logging feature tests
// are quite comprehensive, ensuring that this code works and integrates
// correctly with the agents.
func Manifold(config ManifoldConfig) dependency.Manifold {

	// newWorker trivially wraps "New" function for use in a util.PostUpgradeManifold.
	// It's not tested at the moment, because the scaffolding necessary is too
	// unwieldy/distracting to introduce at this point.
	var newWorker = func(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
		if !feature.IsDbLogEnabled() {
			logger.Warningf("log sender manifold disabled by feature flag")
			return nil, dependency.ErrMissing
		}

		return New(config.LogSource, logsender.NewAPI(apiCaller)), nil
	}

	return util.PostUpgradeManifold(config.PostUpgradeManifoldConfig, newWorker)
}
