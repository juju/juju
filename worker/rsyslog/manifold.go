// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rsyslog

import (
	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/rsyslog"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig util.AgentApiManifoldConfig

// Manifold returns a dependency manifold that runs an rsyslog
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return util.AgentApiManifold(util.AgentApiManifoldConfig(config), newWorker)
}

// newWorker exists to wrap NewRsyslogConfigWorker in a format convenient for an
// AgentApiManifold.
// TODO(fwereade) 2015-05-11 Eventually, the method should be the sole accessible
// package factory function -- as part of the manifold -- and all tests should
// thus be routed through it.
var newWorker = func(agent agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	agentConfig := agent.CurrentConfig()
	tag := agentConfig.Tag()
	namespace := agentConfig.Value(coreagent.Namespace)
	addrs, err := agentConfig.APIAddresses()
	if err != nil {
		return nil, err
	}
	return NewRsyslogConfigWorker(
		rsyslog.NewState(apiCaller),
		RsyslogModeForwarding,
		tag,
		namespace,
		addrs,
	)
}
