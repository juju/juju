// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit

import (
	"context"
	"os"
	"time"

	"github.com/juju/clock"
	"github.com/juju/pubsub/v2"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v4/dependency"
	"github.com/prometheus/client_golang/prometheus"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/api"
	agentlifeflag "github.com/juju/juju/api/agent/lifeflag"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/life"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	coretrace "github.com/juju/juju/core/trace"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/observability/probe"
	proxy "github.com/juju/juju/internal/proxy/config"
	"github.com/juju/juju/internal/upgrades"
	"github.com/juju/juju/internal/upgradesteps"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/agent"
	"github.com/juju/juju/internal/worker/apiaddressupdater"
	"github.com/juju/juju/internal/worker/apicaller"
	"github.com/juju/juju/internal/worker/apiconfigwatcher"
	"github.com/juju/juju/internal/worker/caasprobebinder"
	"github.com/juju/juju/internal/worker/caasprober"
	"github.com/juju/juju/internal/worker/caasunitterminationworker"
	"github.com/juju/juju/internal/worker/caasupgrader"
	"github.com/juju/juju/internal/worker/fortress"
	"github.com/juju/juju/internal/worker/gate"
	"github.com/juju/juju/internal/worker/leadership"
	"github.com/juju/juju/internal/worker/lifeflag"
	wlogger "github.com/juju/juju/internal/worker/logger"
	"github.com/juju/juju/internal/worker/logsender"
	"github.com/juju/juju/internal/worker/migrationflag"
	"github.com/juju/juju/internal/worker/migrationminion"
	"github.com/juju/juju/internal/worker/muxhttpserver"
	"github.com/juju/juju/internal/worker/proxyupdater"
	"github.com/juju/juju/internal/worker/retrystrategy"
	"github.com/juju/juju/internal/worker/secretsdrainworker"
	"github.com/juju/juju/internal/worker/simplesignalhandler"
	"github.com/juju/juju/internal/worker/trace"
	"github.com/juju/juju/internal/worker/uniter"
	"github.com/juju/juju/internal/worker/units3caller"
	"github.com/juju/juju/internal/worker/upgradestepsmachine"
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
	ValidateMigration func(context.Context, base.APICaller) error

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

	// UpgradeSteps is a function that is used by the upgradesteps
	// worker to run any upgrade steps required to upgrade to the
	// running jujud version.
	UpgradeSteps upgrades.UpgradeStepsFunc

	// PrometheusRegisterer is a prometheus.Registerer that may be used
	// by workers to register Prometheus metric collectors.
	PrometheusRegisterer prometheus.Registerer

	// UpdateLoggerConfig is a function that will save the specified
	// config value as the logging config in the agent.conf file.
	UpdateLoggerConfig func(string) error

	// ProbeAddress describes the net dial address to use for binding the
	// receiving agent probe requests.
	ProbeAddress string

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

	// LocalHub is a simple pubsub that is used for internal agent
	// messaging only. This is used for interactions between workers
	// and the introspection worker.
	LocalHub *pubsub.SimpleHub

	// ColocatedWithController is true when the unit agent is running on
	// the same machine/pod as a Juju controller, where they share the same
	// networking namespace in linux.
	ColocatedWithController bool

	// SignalCh is os.Signal channel to receive signals on.
	SignalCh chan os.Signal
}

var (
	ifDead = engine.Housing{
		Flags: []string{
			deadFlagName,
		},
	}.Decorate

	ifNotDead = engine.Housing{
		Flags: []string{
			notDeadFlagName,
		},
	}.Decorate

	ifNotMigrating = engine.Housing{
		Flags: []string{
			migrationInactiveFlagName,
		},
		Occupy: migrationFortressName,
	}.Decorate
)

// Manifolds returns a set of co-configured manifolds covering the various
// responsibilities of a k8s agent unit command. It also accepts the logSource
// argument because we haven't figured out how to thread all the logging bits
// through a dependency engine yet.
//
// Thou Shalt Not Use String Literals In This Function. Or Else.
func Manifolds(config manifoldsConfig) dependency.Manifolds {
	// NOTE: this agent doesn't have any upgrade steps checks because it will just be restarted when upgrades happen.
	dp := dependency.Manifolds{
		// The agent manifold references the enclosing agent, and is the
		// foundation stone on which most other manifolds ultimately depend.
		agentName: agent.Manifold(config.Agent),

		// The api-config-watcher manifold monitors the API server
		// addresses in the agent config and bounces when they
		// change. It's required as part of model migrations.
		apiConfigWatcherName: apiconfigwatcher.Manifold(apiconfigwatcher.ManifoldConfig{
			AgentName:          agentName,
			AgentConfigChanged: config.AgentConfigChanged,
			Logger:             internallogger.GetLogger("juju.worker.apiconfigwatcher"),
		}),

		apiCallerName: apicaller.Manifold(apicaller.ManifoldConfig{
			AgentName:            agentName,
			APIOpen:              api.Open,
			APIConfigWatcherName: apiConfigWatcherName,
			NewConnection:        apicaller.OnlyConnect,
			Logger:               internallogger.GetLogger("juju.worker.apicaller"),
		}),

		// The S3 API caller is a shim API that wraps the /charms REST
		// API for uploading and downloading charms. It provides a
		// S3-compatible API.
		s3CallerName: units3caller.Manifold(units3caller.ManifoldConfig{
			APICallerName: apiCallerName,
			NewClient:     units3caller.NewS3Client,
			Logger:        internallogger.GetLogger("juju.worker.units3caller"),
		}),

		deadFlagName: lifeflag.Manifold(lifeflag.ManifoldConfig{
			APICallerName:  apiCallerName,
			AgentName:      agentName,
			Result:         life.IsDead,
			Filter:         LifeFilter,
			NotFoundIsDead: true,
			NewFacade: func(b base.APICaller) (lifeflag.Facade, error) {
				return agentlifeflag.NewClient(b), nil
			},
			NewWorker: lifeflag.NewWorker,
		}),

		notDeadFlagName: lifeflag.Manifold(lifeflag.ManifoldConfig{
			APICallerName: apiCallerName,
			AgentName:     agentName,
			Result:        life.IsNotDead,
			Filter:        LifeFilter,
			NewFacade: func(b base.APICaller) (lifeflag.Facade, error) {
				return agentlifeflag.NewClient(b), nil
			},
			NewWorker: lifeflag.NewWorker,
		}),

		// The log sender is a leaf worker that sends log messages to some
		// API server, when configured so to do. We should only need one of
		// these in a consolidated agent.
		logSenderName: ifNotDead(logsender.Manifold(logsender.ManifoldConfig{
			APICallerName: apiCallerName,
			LogSource:     config.LogSource,
		})),

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

		upgraderName: ifNotDead(caasupgrader.Manifold(caasupgrader.ManifoldConfig{
			AgentName:            agentName,
			APICallerName:        apiCallerName,
			UpgradeStepsGateName: upgradeStepsGateName,
			PreviousAgentVersion: config.PreviousAgentVersion,
		})),

		// The upgradesteps worker runs soon after the unit agent
		// starts and runs any steps required to upgrade to the
		// running jujud version. Once upgrade steps have run, the
		// upgradesteps gate is unlocked and the worker exits.
		upgradeStepsName: ifNotDead(upgradestepsmachine.Manifold(upgradestepsmachine.ManifoldConfig{
			AgentName:            agentName,
			APICallerName:        apiCallerName,
			UpgradeStepsGateName: upgradeStepsGateName,
			PreUpgradeSteps:      config.PreUpgradeSteps,
			UpgradeSteps:         config.UpgradeSteps,
			NewAgentStatusSetter: func(ctx context.Context, a base.APICaller) (upgradesteps.StatusSetter, error) {
				return noopStatusSetter{}, nil
			},
			Logger: internallogger.GetLogger("juju.worker.upgradestepsmachine"),
			Clock:  config.Clock,
		})),

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
			Logger:            internallogger.GetLogger("juju.worker.migrationminion", corelogger.MIGRATION),
		}),

		// The proxy config updater is a leaf worker that sets http/https/apt/etc
		// proxy settings.
		proxyConfigUpdaterName: ifNotMigrating(proxyupdater.Manifold(proxyupdater.ManifoldConfig{
			AgentName:           agentName,
			APICallerName:       apiCallerName,
			Logger:              internallogger.GetLogger("juju.worker.proxyupdater"),
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
			LoggerContext:   internallogger.DefaultContext(),
			Logger:          internallogger.GetLogger("juju.worker.logger"),
			UpdateAgentFunc: config.UpdateLoggerConfig,
		})),

		// Probe HTTP server is a http server for handling probe requests from
		// Kubernetes. It provides a mux that is used by the caas prober to
		// register handlers.
		probeHTTPServerName: muxhttpserver.Manifold(muxhttpserver.ManifoldConfig{
			Logger:  internallogger.GetLogger("juju.worker.probehttpserver"),
			Address: config.ProbeAddress,
			Port:    config.ProbePort,
		}),

		// Kubernetes probe handler responsible for reporting status for
		// Kubernetes probes
		caasProberName: caasprober.Manifold(caasprober.ManifoldConfig{
			MuxName: probeHTTPServerName,
		}),

		caasUniterProberBinderName: ifNotDead(caasprobebinder.Manifold(caasprobebinder.ManifoldConfig{
			ProberName:         caasProberName,
			ProbeProviderNames: []string{uniterName},
		})),

		caasZombieProberBinderName: ifDead(caasprobebinder.Manifold(caasprobebinder.ManifoldConfig{
			ProberName: caasProberName,
			DefaultProviders: map[string]probe.ProbeProvider{
				"zombie-readiness": probe.ReadinessProvider(probe.Failure),
			},
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
			Logger:        internallogger.GetLogger("juju.worker.retrystrategy"),
		})),

		// The uniter installs charms; manages the unit's presence in its
		// relations; creates subordinate units; runs all the hooks; sends
		// metrics; etc etc etc. We expect to break it up further in the
		// coming weeks, and to need one per unit in a consolidated agent
		// (and probably one for each component broken out).
		uniterName: ifNotMigrating(ifNotDead(uniter.Manifold(uniter.ManifoldConfig{
			AgentName:                    agentName,
			ModelType:                    model.CAAS,
			APICallerName:                apiCallerName,
			S3CallerName:                 s3CallerName,
			TraceName:                    traceName,
			MachineLock:                  config.MachineLock,
			Clock:                        config.Clock,
			LeadershipTrackerName:        leadershipTrackerName,
			CharmDirName:                 charmDirName,
			HookRetryStrategyName:        hookRetryStrategyName,
			TranslateResolverErr:         uniter.TranslateFortressErrors,
			Logger:                       internallogger.GetLogger("juju.worker.uniter"),
			Sidecar:                      true,
			EnforcedCharmModifiedVersion: config.CharmModifiedVersion,
			ContainerNames:               config.ContainerNames,
		}))),

		traceName: trace.Manifold(trace.ManifoldConfig{
			AgentName:       agentName,
			Clock:           config.Clock,
			Logger:          internallogger.GetLogger("juju.worker.trace"),
			NewTracerWorker: trace.NewTracerWorker,
			Kind:            coretrace.KindUnit,
		}),

		// The CAAS unit termination worker handles SIGTERM from the container runtime.
		caasUnitTerminationWorker: ifNotMigrating(ifNotDead(caasunitterminationworker.Manifold(caasunitterminationworker.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			Clock:         config.Clock,
			Logger:        internallogger.GetLogger("juju.worker.caasunitterminationworker"),
			UniterName:    uniterName,
		}))),

		// The secretDrainWorker is the worker that drains secrets from the inactive backend to the current active backend.
		secretDrainWorker: ifNotMigrating(secretsdrainworker.Manifold(secretsdrainworker.ManifoldConfig{
			APICallerName:         apiCallerName,
			Logger:                internallogger.GetLogger("juju.worker.secretsdrainworker"),
			LeadershipTrackerName: leadershipTrackerName,
			NewSecretsDrainFacade: secretsdrainworker.NewSecretsDrainFacadeForAgent,
			NewWorker:             secretsdrainworker.NewWorker,
			NewBackendsClient:     secretsdrainworker.NewSecretBackendsClientForAgent,
		})),

		// Signal handler for handling SIGTERM to shut this agent down when in
		// placed in zombie mode.
		signalHandlerName: ifDead(simplesignalhandler.Manifold(simplesignalhandler.ManifoldConfig{
			Logger:              internallogger.GetLogger("juju.worker.simplesignalhandler"),
			DefaultHandlerError: jworker.ErrTerminateAgent,
			SignalCh:            config.SignalCh,
		})),
	}

	// If the container agent is colocated with the controller for the controller charm, then it doesn't
	// need the api address updater, http probe server or the cass prober workers.
	// For every other deployment of the containeragent, these workers are required.
	if !config.ColocatedWithController {
		// The api address updater is a leaf worker that rewrites agent config
		// as the controller addresses change. We should only need one of
		// these in a consolidated agent.
		dp[apiAddressUpdaterName] = ifNotMigrating(apiaddressupdater.Manifold(apiaddressupdater.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			Logger:        internallogger.GetLogger("juju.worker.apiaddressupdater"),
		}))
	}

	return dp
}

const (
	agentName            = "agent"
	apiConfigWatcherName = "api-config-watcher"
	apiCallerName        = "api-caller"
	s3CallerName         = "s3-caller"
	uniterName           = "uniter"
	logSenderName        = "log-sender"
	traceName            = "trace"

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

	caasProberName             = "caas-prober"
	caasZombieProberBinderName = "caas-zombie-prober-binder"
	caasUniterProberBinderName = "caas-unit-prober-binder"
	probeHTTPServerName        = "probe-http-server"

	proxyConfigUpdaterName   = "proxy-config-updater"
	loggingConfigUpdaterName = "logging-config-updater"
	apiAddressUpdaterName    = "api-address-updater"

	caasUnitTerminationWorker = "caas-unit-termination-worker"

	secretDrainWorker = "secret-drain-worker"

	signalHandlerName = "signal-handler"

	deadFlagName    = "dead-flag"
	notDeadFlagName = "not-dead-flag"
)

type noopStatusSetter struct{}

func (noopStatusSetter) SetStatus(ctx context.Context, setableStatus status.Status, info string, data map[string]any) error {
	return nil
}
