// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/voyeur"
	"github.com/juju/version"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/juju/worker.v1/dependency"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	commonapi "github.com/juju/juju/api/common"
	msapi "github.com/juju/juju/api/meterstatus"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	"github.com/juju/juju/utils/proxy"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/apiaddressupdater"
	"github.com/juju/juju/worker/apicaller"
	"github.com/juju/juju/worker/apiconfigwatcher"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/gate"
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
	"github.com/juju/juju/worker/upgradesteps"
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

	// UpdateLoggerConfig is a function that will save the specified
	// config value as the logging config in the agent.conf file.
	UpdateLoggerConfig func(string) error

	// PreviousAgentVersion passes through the version the unit
	// agent was running before the current restart.
	PreviousAgentVersion version.Number

	// UpgradeStepsLock is passed to the upgrade steps gate to
	// coordinate workers that shouldn't do anything until the
	// upgrade-steps worker is done.
	UpgradeStepsLock gate.Lock

	// UpgradeCheckLock is passed to the upgrade check gate to
	// coordinate workers that shouldn't do anything until the
	// upgrader worker completes it's first check.
	UpgradeCheckLock gate.Lock

	// PreUpgradeSteps is a function that is used by the upgradesteps
	// worker to ensure that conditions are OK for an upgrade to
	// proceed.
	PreUpgradeSteps func(*state.StatePool, coreagent.Config, bool, bool, bool) error

	// MachineLock is a central source for acquiring the machine lock.
	// This is used by a number of workers to ensure serialisation of actions
	// across the machine.
	MachineLock machinelock.Lock

	// Clock supplies timekeeping services to various workers.
	Clock clock.Clock
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
			Logger:             loggo.GetLogger("juju.worker.apiconfigwatcher"),
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
			Logger:               loggo.GetLogger("juju.worker.apicaller"),
		}),

		// The log sender is a leaf worker that sends log messages to some
		// API server, when configured so to do. We should only need one of
		// these in a consolidated agent.
		logSenderName: logsender.Manifold(logsender.ManifoldConfig{
			APICallerName: apiCallerName,
			LogSource:     config.LogSource,
		}),

		// The upgrade steps gate is used to coordinate workers which
		// shouldn't do anything until the upgrade-steps worker has
		// finished running any required upgrade steps. The flag of
		// similar name is used to implement the isFullyUpgraded func
		// that keeps upgrade concerns out of unrelated manifolds.
		upgradeStepsGateName: gate.ManifoldEx(config.UpgradeStepsLock),
		upgradeStepsFlagName: gate.FlagManifold(gate.FlagManifoldConfig{
			GateName:  upgradeStepsGateName,
			NewWorker: gate.NewFlagWorker,
		}),

		// The upgrade check gate is used to coordinate workers which
		// shouldn't do anything until the upgrader worker has
		// completed its first check for a new tools version to
		// upgrade to. The flag of similar name is used to implement
		// the isFullyUpgraded func that keeps upgrade concerns out of
		// unrelated manifolds.
		upgradeCheckGateName: gate.ManifoldEx(config.UpgradeCheckLock),
		upgradeCheckFlagName: gate.FlagManifold(gate.FlagManifoldConfig{
			GateName:  upgradeCheckGateName,
			NewWorker: gate.NewFlagWorker,
		}),

		// The upgrader is a leaf worker that returns a specific error type
		// recognised by the unit agent, causing other workers to be stopped
		// and the agent to be restarted running the new tools. We should only
		// need one of these in a consolidated agent, but we'll need to be
		// careful about behavioural differences, and interactions with the
		// upgradesteps worker.
		upgraderName: upgrader.Manifold(upgrader.ManifoldConfig{
			AgentName:            agentName,
			APICallerName:        apiCallerName,
			UpgradeStepsGateName: upgradeStepsGateName,
			UpgradeCheckGateName: upgradeCheckGateName,
			PreviousAgentVersion: config.PreviousAgentVersion,
		}),

		// The upgradesteps worker runs soon after the unit agent
		// starts and runs any steps required to upgrade to the
		// running jujud version. Once upgrade steps have run, the
		// upgradesteps gate is unlocked and the worker exits.
		upgradeStepsName: upgradesteps.Manifold(upgradesteps.ManifoldConfig{
			AgentName:            agentName,
			APICallerName:        apiCallerName,
			UpgradeStepsGateName: upgradeStepsGateName,
			// Realistically,  units should not open state for any reason.
			OpenStateForUpgrade: func() (*state.StatePool, error) {
				return nil, errors.New("unit agent cannot open state")
			},
			PreUpgradeSteps: config.PreUpgradeSteps,
			NewAgentStatusSetter: func(apiConn api.Connection) (upgradesteps.StatusSetter, error) {
				return &noopStatusSetter{}, nil
			},
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
		migrationFortressName: ifFullyUpgraded(fortress.Manifold()),
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
			Clock:             config.Clock,
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
			AgentName:       agentName,
			APICallerName:   apiCallerName,
			LoggingContext:  loggo.DefaultContext(),
			Logger:          loggo.GetLogger("juju.worker.logger"),
			UpdateAgentFunc: config.UpdateLoggerConfig,
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
			Logger:          loggo.GetLogger("juju.worker.proxyupdater"),
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
			Clock:               config.Clock,
			LeadershipGuarantee: config.LeadershipGuarantee,
		})),

		// HookRetryStrategy uses a retrystrategy worker to get a
		// retry strategy that will be used by the uniter to run its hooks.
		hookRetryStrategyName: ifNotMigrating(retrystrategy.Manifold(retrystrategy.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			NewFacade:     retrystrategy.NewFacade,
			NewWorker:     retrystrategy.NewRetryStrategyWorker,
			Logger:        loggo.GetLogger("juju.worker.retrystrategy"),
		})),

		// The uniter installs charms; manages the unit's presence in its
		// relations; creates subordinate units; runs all the hooks; sends
		// metrics; etc etc etc. We expect to break it up further in the
		// coming weeks, and to need one per unit in a consolidated agent
		// (and probably one for each component broken out).
		uniterName: ifNotMigrating(uniter.Manifold(uniter.ManifoldConfig{
			AgentName:             agentName,
			APICallerName:         apiCallerName,
			MachineLock:           config.MachineLock,
			Clock:                 config.Clock,
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
			MachineLock:              config.MachineLock,
			Clock:                    config.Clock,
			NewHookRunner:            meterstatus.NewHookRunner,
			NewMeterStatusAPIClient:  msapi.NewClient,
			NewUniterStateAPIClient:  commonapi.NewUniterStateAPI,
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

var ifFullyUpgraded = engine.Housing{
	Flags: []string{
		upgradeStepsFlagName,
		upgradeCheckFlagName,
	},
}.Decorate

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
	upgradeStepsName     = "upgrade-steps-runner"
	upgradeStepsGateName = "upgrade-steps-gate"
	upgradeStepsFlagName = "upgrade-steps-flag"
	upgradeCheckGateName = "upgrade-check-gate"
	upgradeCheckFlagName = "upgrade-check-flag"

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

type noopStatusSetter struct{}

// SetStatus implements upgradesteps.StatusSetter
func (a *noopStatusSetter) SetStatus(setableStatus status.Status, info string, data map[string]interface{}) error {
	return nil
}
