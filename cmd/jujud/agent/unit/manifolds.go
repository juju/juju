// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/voyeur"
	"github.com/prometheus/client_golang/prometheus"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	msapi "github.com/juju/juju/api/meterstatus"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/utils/proxy"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/apiaddressupdater"
	"github.com/juju/juju/worker/apicaller"
	"github.com/juju/juju/worker/apiconfigwatcher"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/leadership"
	"github.com/juju/juju/worker/logger"
	"github.com/juju/juju/worker/logsender"
	"github.com/juju/juju/worker/meterstatus"
	"github.com/juju/juju/worker/metrics/collect"
	"github.com/juju/juju/worker/metrics/sender"
	"github.com/juju/juju/worker/metrics/spool"
	"github.com/juju/juju/worker/migrationflag"
	"github.com/juju/juju/worker/migrationminion"
	"github.com/juju/juju/worker/proxyupdater"
	"github.com/juju/juju/worker/retrystrategy"
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

	// AgentConfigChanged is set whenever the unit agent's config
	// is updated.
	AgentConfigChanged *voyeur.Value

	// ValidateMigration is called by the migrationminion during the
	// migration process to check that the agent will be ok when
	// connected to the new target controller.
	ValidateMigration func(base.APICaller) error

	// PrometheusRegisterer is a prometheus.Registerer that may be used
	// by workers to register Prometheus metric collectors.
	PrometheusRegisterer prometheus.Registerer
}

// Manifolds returns a set of co-configured manifolds covering the various
// responsibilities of a standalone unit agent. It also accepts the logSource
// argument because we haven't figured out how to thread all the logging bits
// through a dependency engine yet.
//
// Thou Shalt Not Use String Literals In This Function. Or Else.
func Manifolds(config ManifoldsConfig) dependency.Manifolds {

	// connectFilter exists to let us retry api connections immediately
	// on password change, rather than causing the dependency engine to
	// wait for a while.
	connectFilter := func(err error) error {
		cause := errors.Cause(err)
		if cause == apicaller.ErrChangedPassword {
			return dependency.ErrBounce
		} else if cause == apicaller.ErrConnectImpossible {
			return worker.ErrTerminateAgent
		}
		return err
	}

	return dependency.Manifolds{

		// The agent manifold references the enclosing agent, and is the
		// foundation stone on which most other manifolds ultimately depend.
		// (Currently, that is "all manifolds", but consider a shared clock.)
		agentName: agent.Manifold(config.Agent),

		// The api-config-watcher manifold monitors the API server
		// addresses in the agent config and bounces when they
		// change. It's required as part of model migrations.
		apiConfigWatcherName: apiconfigwatcher.Manifold(apiconfigwatcher.ManifoldConfig{
			AgentName:          agentName,
			AgentConfigChanged: config.AgentConfigChanged,
		}),

		// The api caller is a thin concurrent wrapper around a connection
		// to some API server. It's used by many other manifolds, which all
		// select their own desired facades. It will be interesting to see
		// how this works when we consolidate the agents; might be best to
		// handle the auth changes server-side..?
		apiCallerName: apicaller.Manifold(apicaller.ManifoldConfig{
			AgentName:            agentName,
			APIConfigWatcherName: apiConfigWatcherName,
			APIOpen:              api.Open,
			NewConnection:        apicaller.ScaryConnect,
			Filter:               connectFilter,
		}),

		// The log sender is a leaf worker that sends log messages to some
		// API server, when configured so to do. We should only need one of
		// these in a consolidated agent.
		logSenderName: logsender.Manifold(logsender.ManifoldConfig{
			APICallerName: apiCallerName,
			LogSource:     config.LogSource,
		}),

		// The upgrader is a leaf worker that returns a specific error type
		// recognised by the unit agent, causing other workers to be stopped
		// and the agent to be restarted running the new tools. We should only
		// need one of these in a consolidated agent, but we'll need to be
		// careful about behavioural differences, and interactions with the
		// upgradesteps worker.
		upgraderName: upgrader.Manifold(upgrader.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
		}),

		// The migration workers collaborate to run migrations;
		// and to create a mechanism for running other workers
		// so they can't accidentally interfere with a migration
		// in progress. Such a manifold should (1) depend on the
		// migration-inactive flag, to know when to start or die;
		// and (2) occupy the migration-fortress, so as to avoid
		// possible interference with the minion (which will not
		// take action until it's gained sole control of the
		// fortress).
		migrationFortressName: fortress.Manifold(),
		migrationInactiveFlagName: migrationflag.Manifold(migrationflag.ManifoldConfig{
			APICallerName: apiCallerName,
			Check:         migrationflag.IsTerminal,
			NewFacade:     migrationflag.NewFacade,
			NewWorker:     migrationflag.NewWorker,
		}),
		migrationMinionName: migrationminion.Manifold(migrationminion.ManifoldConfig{
			AgentName:         agentName,
			APICallerName:     apiCallerName,
			FortressName:      migrationFortressName,
			APIOpen:           api.Open,
			ValidateMigration: config.ValidateMigration,
			NewFacade:         migrationminion.NewFacade,
			NewWorker:         migrationminion.NewWorker,
		}),

		// The logging config updater is a leaf worker that indirectly
		// controls the messages sent via the log sender according to
		// changes in environment config. We should only need one of
		// these in a consolidated agent.
		loggingConfigUpdaterName: ifNotMigrating(logger.Manifold(logger.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
		})),

		// The api address updater is a leaf worker that rewrites agent config
		// as the controller addresses change. We should only need one of
		// these in a consolidated agent.
		apiAddressUpdaterName: ifNotMigrating(apiaddressupdater.Manifold(apiaddressupdater.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
		})),

		// The proxy config updater is a leaf worker that sets http/https/apt/etc
		// proxy settings.
		// TODO(fwereade): timing of this is suspicious. There was superstitious
		// code trying to run this early; if that ever helped, it was only by
		// coincidence. Probably we ought to be making components that might
		// need proxy config into explicit dependencies of the proxy updater...
		proxyConfigUpdaterName: ifNotMigrating(proxyupdater.Manifold(proxyupdater.ManifoldConfig{
			AgentName:       agentName,
			APICallerName:   apiCallerName,
			WorkerFunc:      proxyupdater.NewWorker,
			InProcessUpdate: proxy.DefaultConfig.Set,
		})),

		// The charmdir resource coordinates whether the charm directory is
		// available or not; after 'start' hook and before 'stop' hook
		// executes, and not during upgrades.
		charmDirName: ifNotMigrating(fortress.Manifold()),

		// The leadership tracker attempts to secure and retain leadership of
		// the unit's service, and is consulted on such matters by the
		// uniter. As it stands today, we'll need one per unit in a
		// consolidated agent.
		leadershipTrackerName: ifNotMigrating(leadership.Manifold(leadership.ManifoldConfig{
			AgentName:           agentName,
			APICallerName:       apiCallerName,
			Clock:               clock.WallClock,
			LeadershipGuarantee: config.LeadershipGuarantee,
		})),

		// HookRetryStrategy uses a retrystrategy worker to get a
		// retry strategy that will be used by the uniter to run its hooks.
		hookRetryStrategyName: ifNotMigrating(retrystrategy.Manifold(retrystrategy.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			NewFacade:     retrystrategy.NewFacade,
			NewWorker:     retrystrategy.NewRetryStrategyWorker,
		})),

		// The uniter installs charms; manages the unit's presence in its
		// relations; creates suboordinate units; runs all the hooks; sends
		// metrics; etc etc etc. We expect to break it up further in the
		// coming weeks, and to need one per unit in a consolidated agent
		// (and probably one for each component broken out).
		uniterName: ifNotMigrating(uniter.Manifold(uniter.ManifoldConfig{
			AgentName:       agentName,
			APICallerName:   apiCallerName,
			MachineLockName: coreagent.MachineLockName,
			Clock:           clock.WallClock,
			LeadershipTrackerName: leadershipTrackerName,
			CharmDirName:          charmDirName,
			HookRetryStrategyName: hookRetryStrategyName,
			TranslateResolverErr:  uniter.TranslateFortressErrors,
		})),

		// TODO (mattyw) should be added to machine agent.
		metricSpoolName: ifNotMigrating(spool.Manifold(spool.ManifoldConfig{
			AgentName: agentName,
		})),

		// The metric collect worker executes the collect-metrics hook in a
		// restricted context that can safely run concurrently with other hooks.
		metricCollectName: ifNotMigrating(collect.Manifold(collect.ManifoldConfig{
			AgentName:       agentName,
			MetricSpoolName: metricSpoolName,
			CharmDirName:    charmDirName,
		})),

		// The meter status worker executes the meter-status-changed hook when it detects
		// that the meter status has changed.
		meterStatusName: ifNotMigrating(meterstatus.Manifold(meterstatus.ManifoldConfig{
			AgentName:                agentName,
			APICallerName:            apiCallerName,
			MachineLockName:          coreagent.MachineLockName,
			Clock:                    clock.WallClock,
			NewHookRunner:            meterstatus.NewHookRunner,
			NewMeterStatusAPIClient:  msapi.NewClient,
			NewConnectedStatusWorker: meterstatus.NewConnectedStatusWorker,
			NewIsolatedStatusWorker:  meterstatus.NewIsolatedStatusWorker,
		})),

		// The metric sender worker periodically sends accumulated metrics to the controller.
		metricSenderName: ifNotMigrating(sender.Manifold(sender.ManifoldConfig{
			AgentName:       agentName,
			APICallerName:   apiCallerName,
			MetricSpoolName: metricSpoolName,
		})),
	}
}

var ifNotMigrating = engine.Housing{
	Flags: []string{
		migrationInactiveFlagName,
	},
	Occupy: migrationFortressName,
}.Decorate

const (
	agentName            = "agent"
	apiConfigWatcherName = "api-config-watcher"
	apiCallerName        = "api-caller"
	logSenderName        = "log-sender"
	upgraderName         = "upgrader"

	migrationFortressName     = "migration-fortress"
	migrationInactiveFlagName = "migration-inactive-flag"
	migrationMinionName       = "migration-minion"

	loggingConfigUpdaterName = "logging-config-updater"
	proxyConfigUpdaterName   = "proxy-config-updater"
	apiAddressUpdaterName    = "api-address-updater"

	charmDirName          = "charm-dir"
	leadershipTrackerName = "leadership-tracker"
	hookRetryStrategyName = "hook-retry-strategy"
	uniterName            = "uniter"

	metricSpoolName   = "metric-spool"
	meterStatusName   = "meter-status"
	metricCollectName = "metric-collect"
	metricSenderName  = "metric-sender"
)
