// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/v3/voyeur"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v3/dependency"
	"github.com/prometheus/client_golang/prometheus"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/utils/proxy"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/apiaddressupdater"
	"github.com/juju/juju/worker/apicaller"
	"github.com/juju/juju/worker/apiconfigwatcher"
	"github.com/juju/juju/worker/caasprober"
	"github.com/juju/juju/worker/caasunitterminationworker"
	"github.com/juju/juju/worker/caasupgrader"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/leadership"
	wlogger "github.com/juju/juju/worker/logger"
	"github.com/juju/juju/worker/logsender"
	"github.com/juju/juju/worker/migrationflag"
	"github.com/juju/juju/worker/migrationminion"
	"github.com/juju/juju/worker/muxhttpserver"
	"github.com/juju/juju/worker/proxyupdater"
	"github.com/juju/juju/worker/retrystrategy"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/upgradesteps"
)

// manifoldsConfig allows specialisation of the result of Manifolds.
type manifoldsConfig struct {
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

	// PreviousAgentVersion passes through the version the unit
	// agent was running before the current restart.
	PreviousAgentVersion version.Number

	// UpgradeStepsLock is passed to the upgrade steps gate to
	// coordinate workers that shouldn't do anything until the
	// upgrade-steps worker is done.
	UpgradeStepsLock gate.Lock

	// PreUpgradeSteps is a function that is used by the upgradesteps
	// worker to ensure that conditions are OK for an upgrade to
	// proceed.
	PreUpgradeSteps upgrades.PreUpgradeStepsFunc

	// PrometheusRegisterer is a prometheus.Registerer that may be used
	// by workers to register Prometheus metric collectors.
	PrometheusRegisterer prometheus.Registerer

	// UpdateLoggerConfig is a function that will save the specified
	// config value as the logging config in the agent.conf file.
	UpdateLoggerConfig func(string) error

	// ProbePort describes the http port to operator on for receiving agent
	// probe requests.
	ProbePort string

	// MachineLock is a central source for acquiring the machine lock.
	// This is used by a number of workers to ensure serialisation of actions
	// across the machine.
	MachineLock machinelock.Lock

	// Clock contains the clock that will be made available to manifolds.
	Clock clock.Clock

	// CharmModifiedVersion to validate downloaded charm is for the provided
	// infrastructure.
	CharmModifiedVersion int

	// ContainerNames this unit is running with.
	ContainerNames []string
}

// Manifolds returns a set of co-configured manifolds covering the various
// responsibilities of a k8s agent unit command. It also accepts the logSource
// argument because we haven't figured out how to thread all the logging bits
// through a dependency engine yet.
//
// Thou Shalt Not Use String Literals In This Function. Or Else.
func Manifolds(config manifoldsConfig) dependency.Manifolds {
	// NOTE: this agent doesn't have any upgrade steps checks because it will just be restarted when upgrades happen.
	return dependency.Manifolds{
		// The agent manifold references the enclosing agent, and is the
		// foundation stone on which most other manifolds ultimately depend.
		agentName: agent.Manifold(config.Agent),

		// The api-config-watcher manifold monitors the API server
		// addresses in the agent config and bounces when they
		// change. It's required as part of model migrations.
		apiConfigWatcherName: apiconfigwatcher.Manifold(apiconfigwatcher.ManifoldConfig{
			AgentName:          agentName,
			AgentConfigChanged: config.AgentConfigChanged,
			Logger:             loggo.GetLogger("juju.worker.apiconfigwatcher"),
		}),

		apiCallerName: apicaller.Manifold(apicaller.ManifoldConfig{
			AgentName:            agentName,
			APIOpen:              api.Open,
			APIConfigWatcherName: apiConfigWatcherName,
			NewConnection:        apicaller.OnlyConnect,
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

		upgraderName: caasupgrader.Manifold(caasupgrader.ManifoldConfig{
			AgentName:            agentName,
			APICallerName:        apiCallerName,
			UpgradeStepsGateName: upgradeStepsGateName,
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
			// Realistically,  operators should not open state for any reason.
			OpenStateForUpgrade: func() (*state.StatePool, error) {
				return nil, errors.New("operator cannot open state")
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
			Clock:             config.Clock,
			APIOpen:           api.Open,
			ValidateMigration: config.ValidateMigration,
			NewFacade:         migrationminion.NewFacade,
			NewWorker:         migrationminion.NewWorker,
			Logger:            loggo.GetLogger("juju.worker.migrationminion"),
		}),

		// The proxy config updater is a leaf worker that sets http/https/apt/etc
		// proxy settings.
		proxyConfigUpdaterName: ifNotMigrating(proxyupdater.Manifold(proxyupdater.ManifoldConfig{
			AgentName:           agentName,
			APICallerName:       apiCallerName,
			Logger:              loggo.GetLogger("juju.worker.proxyupdater"),
			WorkerFunc:          proxyupdater.NewWorker,
			InProcessUpdate:     proxy.DefaultConfig.Set,
			SupportLegacyValues: false,
			RunFunc:             proxyupdater.RunWithStdIn,
		})),

		// The logging config updater is a leaf worker that indirectly
		// controls the messages sent via the log sender according to
		// changes in environment config. We should only need one of
		// these in a consolidated agent.
		loggingConfigUpdaterName: ifNotMigrating(wlogger.Manifold(wlogger.ManifoldConfig{
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
			Logger:        loggo.GetLogger("juju.worker.apiaddressupdater"),
		})),

		// Probe HTTP server is a http server for handling probe requests from
		// Kubernetes. It provides a mux that is used by the caas prober to
		// register handlers.
		probeHTTPServerName: muxhttpserver.Manifold(muxhttpserver.ManifoldConfig{
			Logger: loggo.GetLogger("juju.worker.probehttpserver"),
			Port:   config.ProbePort,
		}),

		// Kubernetes probe handler responsible for reporting status for
		// Kubernetes probes
		caasProberName: caasprober.Manifold(caasprober.ManifoldConfig{
			MuxName: probeHTTPServerName,
		}),

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
			AgentName:                    agentName,
			ModelType:                    model.CAAS,
			APICallerName:                apiCallerName,
			MachineLock:                  config.MachineLock,
			Clock:                        config.Clock,
			LeadershipTrackerName:        leadershipTrackerName,
			CharmDirName:                 charmDirName,
			HookRetryStrategyName:        hookRetryStrategyName,
			TranslateResolverErr:         uniter.TranslateFortressErrors,
			Logger:                       loggo.GetLogger("juju.worker.uniter"),
			Sidecar:                      true,
			EnforcedCharmModifiedVersion: config.CharmModifiedVersion,
			ContainerNames:               config.ContainerNames,
		})),

		// The CAAS unit termination worker handles SIGTERM from the container runtime.
		caasUnitTerminationWorker: ifNotMigrating(caasunitterminationworker.Manifold(caasunitterminationworker.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			Clock:         config.Clock,
			Logger:        loggo.GetLogger("juju.worker.caasunitterminationworker"),
			UniterName:    uniterName,
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
	uniterName           = "uniter"
	logSenderName        = "log-sender"

	charmDirName          = "charm-dir"
	leadershipTrackerName = "leadership-tracker"
	hookRetryStrategyName = "hook-retry-strategy"

	upgraderName         = "upgrader"
	upgradeStepsName     = "upgrade-steps-runner"
	upgradeStepsGateName = "upgrade-steps-gate"
	upgradeStepsFlagName = "upgrade-steps-flag"

	migrationFortressName     = "migration-fortress"
	migrationInactiveFlagName = "migration-inactive-flag"
	migrationMinionName       = "migration-minion"

	caasProberName      = "caas-prober"
	probeHTTPServerName = "probe-http-server"

	proxyConfigUpdaterName   = "proxy-config-updater"
	loggingConfigUpdaterName = "logging-config-updater"
	apiAddressUpdaterName    = "api-address-updater"

	caasUnitTerminationWorker = "caas-unit-termination-worker"
)

type noopStatusSetter struct{}

// SetStatus implements upgradesteps.StatusSetter
func (a *noopStatusSetter) SetStatus(setableStatus status.Status, info string, data map[string]interface{}) error {
	return nil
}
