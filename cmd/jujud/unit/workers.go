// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit

import (
	"time"

	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/apiaddressupdater"
	"github.com/juju/juju/worker/apiconn"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/leadership"
	"github.com/juju/juju/worker/logger"
	"github.com/juju/juju/worker/machinelock"
	"github.com/juju/juju/worker/proxyupdater"
	"github.com/juju/juju/worker/rsyslog"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/filter"
	"github.com/juju/juju/worker/upgrader"
)

// These define the names of the dependency.Manifolds we use in a unit agent.
// This structure is not sophisticated enough to support running multiple unit
// agents in the same Engine.
var (
	// Long-term, we only expect one of each of these per process; apart from
	// a little bit of handwaving around the identity used for the api connection,
	// these elements should work just fine in a machine agent without changes.
	MachineLockName       = "machine-lock"
	BinaryUpgraderName    = "binary-upgrader"
	LoggerUpdaterName     = "logger-updater"
	ProxyUpdaterName      = "proxy-updater"
	RsyslogUpdaterName    = "rsyslog-updater"
	ApiAddressUpdaterName = "api-address-updater"
	ApiCallerName         = "api-caller"

	// We expect one of each of these per running unit; when we try to run N
	// units inside each agent process, we'll need to disambiguate the names
	// (and probably add/remove the following as a group).
	LeadershipTrackerName = "leadership-tracker"
	EventFilterName       = "event-filter"
	UniterName            = "uniter"
)

// AgentManifolds returns the mutually-referential manifolds and names needed
// to run the code for which the supplied unit agent is responsible, suitable
// for installation in a dependency.Engine.
//
// The agent itself is represented as a manifold, referenced by (most) others
// so that they can read and (sometimes) write its local configuration.
func AgentManifolds(a agent.Agent) map[string]dependency.Manifold {
	agentName := a.Tag().String()
	return map[string]dependency.Manifold{

		agentName: agent.Manifold(a),

		ApiAddressUpdaterName: apiaddressupdater.Manifold(apiaddressupdater.ManifoldConfig{
			AgentName:     agentName,
			ApiCallerName: ApiCallerName,
		}),

		ApiCallerName: apiconn.Manifold(apiconn.ManifoldConfig{
			AgentName: agentName,
		}),

		BinaryUpgraderName: upgrader.Manifold(upgrader.ManifoldConfig{
			AgentName:     agentName,
			ApiCallerName: ApiCallerName,
		}),

		EventFilterName: filter.Manifold(filter.ManifoldConfig{
			AgentName:     agentName,
			ApiCallerName: ApiCallerName,
		}),

		LeadershipTrackerName: leadership.Manifold(leadership.ManifoldConfig{
			AgentName:           agentName,
			ApiCallerName:       ApiCallerName,
			LeadershipGuarantee: 30 * time.Second,
		}),

		LoggerUpdaterName: logger.Manifold(logger.ManifoldConfig{
			AgentName:     agentName,
			ApiCallerName: ApiCallerName,
		}),

		MachineLockName: machinelock.Manifold(machinelock.ManifoldConfig{
			AgentName: agentName,
		}),

		ProxyUpdaterName: proxyupdater.Manifold(proxyupdater.ManifoldConfig{
			ApiCallerName: ApiCallerName,
		}),

		RsyslogUpdaterName: rsyslog.Manifold(rsyslog.ManifoldConfig{
			AgentName:     agentName,
			ApiCallerName: ApiCallerName,
		}),

		UniterName: uniter.Manifold(uniter.ManifoldConfig{
			AgentName:             agentName,
			ApiCallerName:         ApiCallerName,
			EventFilterName:       EventFilterName,
			LeadershipTrackerName: LeadershipTrackerName,
			MachineLockName:       MachineLockName,
		}),
	}
}
