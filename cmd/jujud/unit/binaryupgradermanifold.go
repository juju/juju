// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit

import (
	"github.com/juju/juju/api/base"
	apiupgrader "github.com/juju/juju/api/upgrader"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/upgrader"
)

// BinaryUpgraderManifoldConfig defines the names of the manifolds on which a
// BinaryUpgraderManifold will depend.
type BinaryUpgraderManifoldConfig struct {
	AgentName     string
	ApiCallerName string
}

// BinaryUpgraderManifold returns a dependency manifold that runs an upgrader
// worker, using the resource names defined in the supplied config.
//
// It should really be defined in worker/upgrader instead, but import loops render
// this impractical for the time being.
func BinaryUpgraderManifold(config BinaryUpgraderManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.ApiCallerName,
		},
		Start: binaryUpgraderStartFunc(config),
	}
}

// binaryUpgraderStartFunc returns a StartFunc that creates an upgrade worker
// based on the manifolds named in the supplied config.
func binaryUpgraderStartFunc(config BinaryUpgraderManifoldConfig) dependency.StartFunc {
	return func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
		var agent agent.Agent
		if err := getResource(config.AgentName, &agent); err != nil {
			return nil, err
		}
		var apiCaller base.APICaller
		if err := getResource(config.ApiCallerName, &apiCaller); err != nil {
			return nil, err
		}
		return newBinaryUpgrader(agent, apiCaller)
	}
}

// newBinaryUpgrader exists to put all the weird and hard-to-test bits in one
// place; it should be patched out for unit tests via NewBinaryUpgrader in
// export_test (and should ideally be directly tested itself, but the concrete
// facade makes that hard; for the moment we rely on the full-stack tests in
// cmd/jujud).
var newBinaryUpgrader = func(agent agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	currentConfig := agent.CurrentConfig()
	upgraderFacade := apiupgrader.NewState(apiCaller)
	return upgrader.NewUpgrader(
		upgraderFacade,
		currentConfig,
		currentConfig.UpgradedToVersion(),
		func() bool { return false },
	), nil
}
