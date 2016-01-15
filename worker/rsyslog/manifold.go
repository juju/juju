// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rsyslog

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/featureflag"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	apirsyslog "github.com/juju/juju/api/rsyslog"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	util.PostUpgradeManifoldConfig
	NewRsyslogConfigWorker func(st *apirsyslog.State, agentConfig agent.Config, mode RsyslogMode) (worker.Worker, error)
}

// Manifold returns a dependency manifold that runs an rsyslog
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {

	// newWorker exists to wrap NewRsyslogConfigWorker in a format convenient for an
	// AgentApiManifold.
	// TODO(fwereade) 2015-05-11 Eventually, the method should be the sole accessible
	// package factory function -- as part of the manifold -- and all tests should
	// thus be routed through it.
	var newWorker = func(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
		if featureflag.Enabled(feature.DisableRsyslog) {
			logger.Warningf("rsyslog manifold disabled by feature flag")
			return nil, dependency.ErrMissing
		}

		agentConfig := a.CurrentConfig()
		tag := agentConfig.Tag()

		rsyslogMode := RsyslogModeForwarding
		if _, ok := tag.(names.MachineTag); ok {

			// Get API connection.
			apiConn, ok := apiCaller.(api.Connection)
			if !ok {
				return nil, errors.New("unable to obtain api.Connection")
			}

			// Get the machine agent's jobs.
			entity, err := apiConn.Agent().Entity(tag)
			if err != nil {
				return nil, err
			}
			jobs := entity.Jobs()

			for _, job := range jobs {
				if job == multiwatcher.JobManageEnviron {
					rsyslogMode = RsyslogModeAccumulate
					break
				}
			}
		}

		api := apirsyslog.NewState(apiCaller)
		w, err := config.NewRsyslogConfigWorker(api, agentConfig, rsyslogMode)
		if err != nil {
			return nil, errors.Annotate(err, "cannot start rsyslog config updater worker")
		}
		return w, nil
	}

	return util.PostUpgradeManifold(config.PostUpgradeManifoldConfig, newWorker)
}
