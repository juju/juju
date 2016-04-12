// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	apiproxyupdater "github.com/juju/juju/api/proxyupdater"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig util.AgentApiManifoldConfig

// Manifold returns a dependency manifold that runs a proxy updater worker,
// using the api connection resource named in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	typedConfig := util.AgentApiManifoldConfig(config)
	return util.AgentApiManifold(typedConfig, newWorker)
}

// newWorker is not currently tested; it should eventually replace New as the
// package's exposed factory func, and then all tests should pass through it.
func newWorker(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	agentConfig := a.CurrentConfig()
	switch tag := agentConfig.Tag().(type) {
	case names.MachineTag, names.UnitTag:
	default:
		return nil, errors.Errorf("unknown agent type: %T", tag)
	}

	proxyAPI, err := apiproxyupdater.NewAPI(apiCaller, agentConfig.Tag())
	if err != nil {
		return nil, err
	}
	return NewWorker(proxyAPI, Config{})
}
