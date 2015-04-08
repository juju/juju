// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit

import (
	"github.com/juju/juju/api"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/rsyslog"
)

// RsyslogUpdaterManifoldConfig defines the names of the manifolds on which a
// RsyslogUpdaterManifold will depend.
type RsyslogUpdaterManifoldConfig struct {
	AgentName         string
	ApiConnectionName string
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
			config.ApiConnectionName,
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
		var apiConnection *api.State
		if err := getResource(config.ApiConnectionName, &apiConnection); err != nil {
			return nil, err
		}
		return cmdutil.NewRsyslogConfigWorker(
			apiConnection.Rsyslog(),
			agent.CurrentConfig(),
			rsyslog.RsyslogModeForwarding,
		)
	}
}
