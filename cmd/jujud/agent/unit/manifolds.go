// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit

import (
	"time"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/apiaddressupdater"
	"github.com/juju/juju/worker/apicaller"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/leadership"
	"github.com/juju/juju/worker/logger"
	"github.com/juju/juju/worker/logsender"
	"github.com/juju/juju/worker/machinelock"
	"github.com/juju/juju/worker/metrics/spool"
	"github.com/juju/juju/worker/proxyupdater"
	"github.com/juju/juju/worker/rsyslog"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/upgrader"
)

// ManifoldsConfig allows specialisation of the result of Manifolds.
type ManifoldsConfig struct {

	// Agent contains the agent that will be wrapped and made available to
	// its dependencies via a dependency.Engine.
	Agent coreagent.Agent

	// LogSource will be read from by the logsender component.
	LogSource logsender.LogRecordCh

	// LeadershipGuarantee controls the behaviour of the leadership tracker.
	LeadershipGuarantee time.Duration
}

// Manifolds returns a set of co-configured manifolds covering the various
// responsibilities of a standalone unit agent. It also accepts the logSource
// argument because we haven't figured out how to thread all the logging bits
// through a dependency engine yet.
//
// Thou Shalt Not Use String Literals In This Function. Or Else.
func Manifolds(config ManifoldsConfig) dependency.Manifolds {
	return dependency.Manifolds{

		// The agent manifold references the enclosing agent, and is the
		// foundation stone on which most other manifolds ultimately depend.
		// (Currently, that is "all manifolds", but consider a shared clock.)
		AgentName: agent.Manifold(config.Agent),

		// The machine lock manifold is a thin concurrent wrapper around an
		// FSLock in an agreed location. We expect it to be replaced with an
		// in-memory lock when the unit agent moves into the machine agent.
		MachineLockName: machinelock.Manifold(machinelock.ManifoldConfig{
			AgentName: AgentName,
		}),

		// The api caller is a thin concurrent wrapper around a connection
		// to some API server. It's used by many other manifolds, which all
		// select their own desired facades. It will be interesting to see
		// how this works when we consolidate the agents; might be best to
		// handle the auth changes server-side..?
		APICallerName: apicaller.Manifold(apicaller.ManifoldConfig{
			AgentName:       AgentName,
			APIInfoGateName: APIInfoGateName,
		}),

		// This manifold is used to coordinate between the api caller and the
		// log sender, which share the API credentials that the API caller may
		// update. To avoid surprising races, the log sender waits for the api
		// caller to unblock this, indicating that any password dance has been
		// completed and the log-sender can now connect without confusion.
		APIInfoGateName: gate.Manifold(),

		// The log sender is a leaf worker that sends log messages to some
		// API server, when configured so to do. We should only need one of
		// these in a consolidated agent.
		LogSenderName: logsender.Manifold(logsender.ManifoldConfig{
			AgentName:       AgentName,
			APIInfoGateName: APIInfoGateName,
			LogSource:       config.LogSource,
		}),

		// The rsyslog config updater is a leaf worker that causes rsyslog
		// to send messages to the state servers. We should only need one
		// of these in a consolidated agent.
		RsyslogConfigUpdaterName: rsyslog.Manifold(rsyslog.ManifoldConfig{
			AgentName:     AgentName,
			APICallerName: APICallerName,
		}),

		// The logging config updater is a leaf worker that indirectly
		// controls the messages sent via the log sender or rsyslog,
		// according to changes in environment config. We should only need
		// one of these in a consolidated agent.
		LoggingConfigUpdaterName: logger.Manifold(logger.ManifoldConfig{
			AgentName:     AgentName,
			APICallerName: APICallerName,
		}),

		// The api address updater is a leaf worker that rewrites agent config
		// as the state server addresses change. We should only need one of
		// these in a consolidated agent.
		APIAdddressUpdaterName: apiaddressupdater.Manifold(apiaddressupdater.ManifoldConfig{
			AgentName:     AgentName,
			APICallerName: APICallerName,
		}),

		// The proxy config updater is a leaf worker that sets http/https/apt/etc
		// proxy settings.
		// TODO(fwereade): timing of this is suspicious. There was superstitious
		// code trying to run this early; if that ever helped, it was only by
		// coincidence. Probably we ought to be making components that might
		// need proxy config into explicit dependencies of the proxy updater...
		ProxyConfigUpdaterName: proxyupdater.Manifold(proxyupdater.ManifoldConfig{
			APICallerName: APICallerName,
		}),

		// The upgrader is a leaf worker that returns a specific error type
		// recognised by the unit agent, causing other workers to be stopped
		// and the agent to be restarted running the new tools. We should only
		// need one of these in a consolidated agent, but we'll need to be
		// careful about behavioural differences, and interactions with the
		// upgrade-steps worker.
		UpgraderName: upgrader.Manifold(upgrader.ManifoldConfig{
			AgentName:     AgentName,
			APICallerName: APICallerName,
		}),

		// The leadership tracker attempts to secure and retain leadership of
		// the unit's service, and is consulted on such matters by the
		// uniter. As it stannds today, we'll need one per unit in a
		// consolidated agent.
		LeadershipTrackerName: leadership.Manifold(leadership.ManifoldConfig{
			AgentName:           AgentName,
			APICallerName:       APICallerName,
			LeadershipGuarantee: config.LeadershipGuarantee,
		}),

		// The uniter installs charms; manages the unit's presence in its
		// relations; creates suboordinate units; runs all the hooks; sends
		// metrics; etc etc etc. We expect to break it up further in the
		// coming weeks, and to need one per unit in a consolidated agent
		// (and probably one for each component broken out).
		UniterName: uniter.Manifold(uniter.ManifoldConfig{
			AgentName:             AgentName,
			APICallerName:         APICallerName,
			LeadershipTrackerName: LeadershipTrackerName,
			MachineLockName:       MachineLockName,
		}),

		// TODO (mattyw) should be added to machine agent.
		MetricSpoolName: spool.Manifold(spool.ManifoldConfig{
			AgentName: AgentName,
		}),
	}
}

const (
	AgentName                = "agent"
	APIAdddressUpdaterName   = "api-address-updater"
	APICallerName            = "api-caller"
	APIInfoGateName          = "api-info-gate"
	LeadershipTrackerName    = "leadership-tracker"
	LoggingConfigUpdaterName = "logging-config-updater"
	LogSenderName            = "log-sender"
	MachineLockName          = "machine-lock"
	ProxyConfigUpdaterName   = "proxy-config-updater"
	RsyslogConfigUpdaterName = "rsyslog-config-updater"
	UniterName               = "uniter"
	UpgraderName             = "upgrader"
	MetricSpoolName          = "metric-spool"
)
