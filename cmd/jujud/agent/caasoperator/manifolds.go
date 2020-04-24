// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/voyeur"
	"github.com/juju/version"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	"github.com/prometheus/client_golang/prometheus"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	caasoperatorapi "github.com/juju/juju/api/caasoperator"
	"github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/juju/sockets"
	"github.com/juju/juju/state"
	"github.com/juju/juju/utils/proxy"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/apiaddressupdater"
	"github.com/juju/juju/worker/apicaller"
	"github.com/juju/juju/worker/apiconfigwatcher"
	"github.com/juju/juju/worker/caasoperator"
	"github.com/juju/juju/worker/caasupgrader"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/introspection"
	"github.com/juju/juju/worker/logger"
	"github.com/juju/juju/worker/logsender"
	"github.com/juju/juju/worker/migrationflag"
	"github.com/juju/juju/worker/migrationminion"
	"github.com/juju/juju/worker/proxyupdater"
	"github.com/juju/juju/worker/retrystrategy"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/upgradesteps"
)

// ManifoldsConfig allows specialisation of the result of Manifolds.
type ManifoldsConfig struct {

	// Agent contains the agent that will be wrapped and made available to
	// its dependencies via a dependency.Engine.
	Agent coreagent.Agent

	// AgentConfigChanged is set whenever the unit agent's config
	// is updated.
	AgentConfigChanged *voyeur.Value

	// Clock contains the clock that will be made available to manifolds.
	Clock clock.Clock

	// LogSource will be read from by the logsender component.
	LogSource logsender.LogRecordCh

	// UpdateLoggerConfig is a function that will save the specified
	// config value as the logging config in the agent.conf file.
	UpdateLoggerConfig func(string) error

	// PrometheusRegisterer is a prometheus.Registerer that may be used
	// by workers to register Prometheus metric collectors.
	PrometheusRegisterer prometheus.Registerer

	// LeadershipGuarantee controls the behaviour of the leadership tracker.
	LeadershipGuarantee time.Duration

	// ValidateMigration is called by the migrationminion during the
	// migration process to check that the agent will be ok when
	// connected to the new target controller.
	ValidateMigration func(base.APICaller) error

	// UpgradeStepsLock is passed to the upgrade steps gate to
	// coordinate workers that shouldn't do anything until the
	// upgrade-steps worker is done.
	UpgradeStepsLock gate.Lock

	// PreUpgradeSteps is a function that is used by the upgradesteps
	// worker to ensure that conditions are OK for an upgrade to
	// proceed.
	PreUpgradeSteps func(*state.StatePool, coreagent.Config, bool, bool, bool) error

	// MachineLock is a central source for acquiring the machine lock.
	// This is used by a number of workers to ensure serialisation of actions
	// across the machine.
	MachineLock machinelock.Lock

	// PreviousAgentVersion passes through the version the unit
	// agent was running before the current restart.
	PreviousAgentVersion version.Number

	// NewExecClient provides k8s execframework functionality for juju run commands or actions.
	NewExecClient func(namespace string) (exec.Executor, error)

	// NewContainerStartWatcherClient provides the container start watcher client.
	NewContainerStartWatcherClient func(caasoperator.Client) caasoperator.ContainerStartWatcher

	// RunListenerSocket returns a function to create a run listener socket.
	RunListenerSocket func(*uniter.SocketConfig) (*sockets.Socket, error)
}

// Manifolds returns a set of co-configured manifolds covering the various
// responsibilities of a caasoperator agent. It also accepts the logSource
// argument because we haven't figured out how to thread all the logging bits
// through a dependency engine yet.
//
// Thou Shalt Not Use String Literals In This Function. Or Else.
func Manifolds(config ManifoldsConfig) dependency.Manifolds {

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

		clockName: clockManifold(config.Clock),

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

		// The upgradesteps worker runs soon after the operator
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
		})),

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
			Logger:        loggo.GetLogger("juju.worker.apiaddressupdater"),
		})),

		// The charmdir resource coordinates whether the charm directory is
		// available or not; after 'start' hook and before 'stop' hook
		// executes, and not during upgrades.
		charmDirName: ifNotMigrating(fortress.Manifold()),

		// HookRetryStrategy uses a retrystrategy worker to get a
		// retry strategy that will be used by the uniter to run its hooks.
		hookRetryStrategyName: ifNotMigrating(retrystrategy.Manifold(retrystrategy.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			NewFacade:     retrystrategy.NewFacade,
			NewWorker:     retrystrategy.NewRetryStrategyWorker,
			Logger:        loggo.GetLogger("juju.worker.retrystrategy"),
		})),

		// The operator installs and deploys charm containers;
		// manages the unit's presence in its relations;
		// creates subordinate units; runs all the hooks;
		// sends metrics; etc etc etc.

		operatorName: ifNotMigrating(caasoperator.Manifold(caasoperator.ManifoldConfig{
			Logger:                loggo.GetLogger("juju.worker.caasoperator"),
			AgentName:             agentName,
			APICallerName:         apiCallerName,
			ClockName:             clockName,
			MachineLock:           config.MachineLock,
			LeadershipGuarantee:   config.LeadershipGuarantee,
			CharmDirName:          charmDirName,
			ProfileDir:            introspection.ProfileDir,
			HookRetryStrategyName: hookRetryStrategyName,
			TranslateResolverErr:  uniter.TranslateFortressErrors,

			NewWorker: caasoperator.NewWorker,
			NewClient: func(caller base.APICaller) caasoperator.Client {
				return caasoperatorapi.NewClient(caller)
			},
			NewCharmDownloader: func(caller base.APICaller) caasoperator.Downloader {
				return api.NewCharmDownloader(caller)
			},
			NewExecClient:                  config.NewExecClient,
			NewContainerStartWatcherClient: config.NewContainerStartWatcherClient,
			RunListenerSocket:              config.RunListenerSocket,
		})),
	}
}

func clockManifold(clock clock.Clock) dependency.Manifold {
	return dependency.Manifold{
		Start: func(dependency.Context) (worker.Worker, error) {
			return engine.NewValueWorker(clock)
		},
		Output: engine.ValueWorkerOutput,
	}
}

var ifFullyUpgraded = engine.Housing{
	Flags: []string{
		upgradeStepsFlagName,
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
	clockName            = "clock"
	operatorName         = "operator"
	logSenderName        = "log-sender"

	charmDirName          = "charm-dir"
	hookRetryStrategyName = "hook-retry-strategy"

	upgraderName         = "upgrader"
	upgradeStepsName     = "upgrade-steps-runner"
	upgradeStepsGateName = "upgrade-steps-gate"
	upgradeStepsFlagName = "upgrade-steps-flag"

	migrationFortressName     = "migration-fortress"
	migrationInactiveFlagName = "migration-inactive-flag"
	migrationMinionName       = "migration-minion"

	proxyConfigUpdaterName   = "proxy-config-updater"
	loggingConfigUpdaterName = "logging-config-updater"
	apiAddressUpdaterName    = "api-address-updater"
)

type noopStatusSetter struct{}

// SetStatus implements upgradesteps.StatusSetter
func (a *noopStatusSetter) SetStatus(setableStatus status.Status, info string, data map[string]interface{}) error {
	return nil
}
