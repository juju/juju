// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/environment"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig struct {
	util.PostUpgradeManifoldConfig
	ShouldWriteProxyFiles func(conf agent.Config) bool
}

// Manifold returns a dependency manifold that runs a proxy updater worker,
// using the api connection resource named in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {

	// newWorker is not currently tested; it should eventually replace New as the
	// package's exposed factory func, and then all tests should pass through it.
	// It is covered by functional tests under machine agent.
	var newWorker = func(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
		var writeSystemFiles bool
		agentConfig := a.CurrentConfig()
		switch tag := agentConfig.Tag().(type) {
		case names.MachineTag:
			writeSystemFiles = config.ShouldWriteProxyFiles(agentConfig)
		case names.UnitTag:
			// keep default false value
		default:
			return nil, errors.Errorf("unknown agent type: %T", tag)
		}

		return New(environment.NewFacade(apiCaller), writeSystemFiles)
	}

	return util.PostUpgradeManifold(config.PostUpgradeManifoldConfig, newWorker)
}
