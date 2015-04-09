// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit

import (
	"github.com/juju/juju/api/base"
	apirsyslog "github.com/juju/juju/api/rsyslog"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/rsyslog"
)

// RsyslogUpdaterManifoldConfig defines the names of the manifolds on which a
// RsyslogUpdaterManifold will depend.
type RsyslogUpdaterManifoldConfig struct {
	AgentName     string
	ApiCallerName string
}

// RsyslogUpdaterManifold returns a dependency manifold that runs an rsyslog
// worker, using the resource names defined in the supplied config.
//
// It should really be defined in worker/upgrader instead, but import loops render
// this impractical for the time being.
func RsyslogUpdaterManifold(config RsyslogUpdaterManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.ApiCallerName,
		},
		Start: rsyslogUpdaterStartFunc(config),
	}
}

// rsyslogUpdaterStartFunc returns a StartFunc that creates an rsyslog updater
// worker based on the manifolds named in the supplied config.
func rsyslogUpdaterStartFunc(config RsyslogUpdaterManifoldConfig) dependency.StartFunc {
	return func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
		var agent agent.Agent
		if err := getResource(config.AgentName, &agent); err != nil {
			return nil, err
		}
		var apiCaller base.APICaller
		if err := getResource(config.ApiCallerName, &apiCaller); err != nil {
			return nil, err
		}
		return newRsyslogUpdater(agent, apiCaller)
	}
}

// newRsyslogUpdater exists to put all the weird and hard-to-test bits in one
// place; it should be patched out for unit tests via NewRsyslogUpdater in
// export_test (and should ideally be directly tested itself, but the concrete
// facade makes that hard; for the moment we rely on the full-stack tests in
// cmd/jujud).
var newRsyslogUpdater = func(agent agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	return cmdutil.NewRsyslogConfigWorker(
		apirsyslog.NewState(apiCaller),
		agent.CurrentConfig(),
		rsyslog.RsyslogModeForwarding,
	)
}
