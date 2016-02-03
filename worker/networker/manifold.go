// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
	"fmt"

	apinetworker "github.com/juju/juju/api/networker"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig util.PostUpgradeManifoldConfig

// Manifold returns a dependency manifold that runs a reboot worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return util.PostUpgradeManifold(util.PostUpgradeManifoldConfig(config), newWorker)
}

// newWorker trivially wraps NewNetworker for use in a util.PostUpgradeManifold.
func newWorker(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	apiConn, ok := apiCaller.(api.Connection)
	if !ok {
		return nil, errors.New("unable to obtain api.Connection")
	}
	envConfig, err := apiConn.Environment().EnvironConfig()
	if err != nil {
		return nil, fmt.Errorf("cannot read environment config: %v", err)
	}

	// Check if the network management is disabled.
	disableNetworkManagement, _ := envConfig.DisableNetworkManagement()
	if disableNetworkManagement {
		logger.Infof("network management is disabled")
	}

	// Grab the tag and ensure that it's for a machine.
	tag, ok := a.CurrentConfig().Tag().(names.MachineTag)
	if !ok {
		return nil, errors.New("agent's tag is not a machine tag")
	}

	// Get the machine agent's jobs.
	entity, err := apiConn.Agent().Entity(tag)
	if err != nil {
		return nil, err
	}

	var isNetworkManager bool
	for _, job := range entity.Jobs() {
		if job == multiwatcher.JobManageNetworking {
			isNetworkManager = true
			break
		}
	}

	// Start networker depending on configuration and job.
	intrusiveMode := isNetworkManager && !disableNetworkManagement

	// TODO(waigani) continue here remove apiconn.Networker()
	w, err := NewNetworker(apinetworker.NewState(apiCaller), a.CurrentConfig(), intrusiveMode, DefaultConfigBaseDir)
	if err != nil {
		return nil, errors.Annotate(err, "cannot start networker worker")
	}
	return w, nil
}
