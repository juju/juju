// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"context"
	"net/http"
	"runtime"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/proxy"
	"github.com/juju/pubsub/v2"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	"github.com/prometheus/client_golang/prometheus"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/presence"
	coretrace "github.com/juju/juju/core/trace"
	"github.com/juju/juju/environs"
	containerbroker "github.com/juju/juju/internal/container/broker"
	"github.com/juju/juju/internal/container/lxd"
	proxyconfig "github.com/juju/juju/internal/proxy/config"
	"github.com/juju/juju/internal/upgrades"
	jupgradesteps "github.com/juju/juju/internal/upgradesteps"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/agent"
	"github.com/juju/juju/internal/worker/apiaddressupdater"
	"github.com/juju/juju/internal/worker/apicaller"
	"github.com/juju/juju/internal/worker/apiconfigwatcher"
	"github.com/juju/juju/internal/worker/authenticationworker"
	"github.com/juju/juju/internal/worker/caasunitsmanager"
	"github.com/juju/juju/internal/worker/caasupgrader"
	"github.com/juju/juju/internal/worker/common"
	lxdbroker "github.com/juju/juju/internal/worker/containerbroker"
	"github.com/juju/juju/internal/worker/credentialvalidator"
	"github.com/juju/juju/internal/worker/deployer"
	"github.com/juju/juju/internal/worker/diskmanager"
	"github.com/juju/juju/internal/worker/fanconfigurer"
	"github.com/juju/juju/internal/worker/fortress"
	"github.com/juju/juju/internal/worker/gate"
	"github.com/juju/juju/internal/worker/hostkeyreporter"
	"github.com/juju/juju/internal/worker/identityfilewriter"
	"github.com/juju/juju/internal/worker/instancemutater"
	"github.com/juju/juju/internal/worker/logger"
	"github.com/juju/juju/internal/worker/logsender"
	"github.com/juju/juju/internal/worker/machineactions"
	"github.com/juju/juju/internal/worker/machiner"
	"github.com/juju/juju/internal/worker/migrationflag"
	"github.com/juju/juju/internal/worker/migrationminion"
	"github.com/juju/juju/internal/worker/provisioner"
	"github.com/juju/juju/internal/worker/proxyupdater"
	psworker "github.com/juju/juju/internal/worker/pubsub"
	"github.com/juju/juju/internal/worker/reboot"
	"github.com/juju/juju/internal/worker/stateconverter"
	"github.com/juju/juju/internal/worker/storageprovisioner"
	"github.com/juju/juju/internal/worker/terminationworker"
	"github.com/juju/juju/internal/worker/toolsversionchecker"
	"github.com/juju/juju/internal/worker/trace"
	"github.com/juju/juju/internal/worker/upgrader"
	"github.com/juju/juju/internal/worker/upgradeseries"
	"github.com/juju/juju/internal/worker/upgradestepsmachine"
	"github.com/juju/juju/state"
)

// ManifoldsConfig allows specialisation of the result of Manifolds.
type ManifoldsConfig struct {
	// AgentName is the name of the machine agent, like "machine-12".
	// This will never change during the execution of an agent, and
	// is used to provide this as config into a worker rather than
	// making the worker get it from the agent worker itself.
	AgentName string

	// Agent contains the agent that will be wrapped and made available to
	// its dependencies via a dependency.Engine.
	Agent coreagent.Agent

	// AgentConfigChanged is set whenever the machine agent's config
	// is updated.
	AgentConfigChanged *voyeur.Value

	// RootDir is the root directory that any worker that needs to
	// access local filesystems should use as a base. In actual use it
	// will be "" but it may be overridden in tests.
	RootDir string

	// PreviousAgentVersion passes through the version the machine
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

	// MachineStartup is passed to the machine manifold. It does
	// machine setup work which relies on an API connection.
	MachineStartup func(context.Context, api.Connection, Logger) error

	// PreUpgradeSteps is a function that is used by the upgradesteps
	// worker to ensure that conditions are OK for an upgrade to
	// proceed.
	PreUpgradeSteps func(state.ModelType) upgrades.PreUpgradeStepsFunc

	// UpgradeSteps is a function that is used by the upgradesteps
	// worker to perform the upgrade steps.
	UpgradeSteps upgrades.UpgradeStepsFunc

	// LogSource defines the channel type used to send log message
	// structs within the machine agent.
	LogSource logsender.LogRecordCh

	// NewDeployContext gives the tests the opportunity to create a
	// deployer.Context that can be used for testing.
	NewDeployContext func(deployer.ContextConfig) (deployer.Context, error)

	// Clock supplies timekeeping services to various workers.
	Clock clock.Clock

	// ValidateMigration is called by the migrationminion during the
	// migration process to check that the agent will be ok when
	// connected to the new target controller.
	ValidateMigration func(context.Context, base.APICaller) error

	// PrometheusRegisterer is a prometheus.Registerer that may be used
	// by workers to register Prometheus metric collectors.
	PrometheusRegisterer prometheus.Registerer

	// LocalHub is a simple pubsub that is used for internal agent
	// messaging only. This is used for interactions between workers
	// and the introspection worker.
	LocalHub *pubsub.SimpleHub

	// PubSubReporter is the introspection reporter for the pubsub forwarding
	// worker.
	PubSubReporter psworker.Reporter

	// PresenceRecorder
	PresenceRecorder presence.Recorder

	// UpdateLoggerConfig is a function that will save the specified
	// config value as the logging config in the agent.conf file.
	UpdateLoggerConfig func(string) error

	// NewAgentStatusSetter provides upgradesteps.StatusSetter.
	NewAgentStatusSetter func(context.Context, base.APICaller) (jupgradesteps.StatusSetter, error)

	// RegisterIntrospectionHTTPHandlers is a function that calls the
	// supplied function to register introspection HTTP handlers. The
	// function will be passed a path and a handler; the function may
	// alter the path as it sees fit, e.g. by adding a prefix.
	RegisterIntrospectionHTTPHandlers func(func(path string, _ http.Handler))

	// MachineLock is a central source for acquiring the machine lock.
	// This is used by a number of workers to ensure serialisation of actions
	// across the machine.
	MachineLock machinelock.Lock

	// MuxShutdownWait is the maximum time the http-server worker will wait
	// for all mux clients to gracefully terminate before the http-worker
	// exits regardless.
	MuxShutdownWait time.Duration

	// NewBrokerFunc is a function opens a instance broker (LXD/KVM)
	NewBrokerFunc containerbroker.NewBrokerFunc

	// IsCaasConfig is true if this config is for a caas agent.
	IsCaasConfig bool

	// UnitEngineConfig is used by the deployer to initialize the unit's
	// dependency engine when running in the nested context.
	UnitEngineConfig func() dependency.EngineConfig

	// SetupLogging is used by the deployer to initialize the logging
	// context for the unit.
	SetupLogging func(*loggo.Context, coreagent.Config)

	// CharmhubHTTPClient is the HTTP client used for Charmhub API requests.
	CharmhubHTTPClient HTTPClient

	// S3HTTPClient is the HTTP client used for S3 API requests.
	S3HTTPClient HTTPClient

	// NewEnvironFunc is a function opens a provider "environment"
	// (typically environs.New).
	NewEnvironFunc func(context.Context, environs.OpenParams) (environs.Environ, error)
}

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// commonManifolds returns a set of co-configured manifolds covering the
// various responsibilities of a machine agent.
//
// Thou Shalt Not Use String Literals In This Function. Or Else.
func commonManifolds(config ManifoldsConfig) dependency.Manifolds {

	// connectFilter exists:
	//  1) to let us retry api connections immediately on password change,
	//     rather than causing the dependency engine to wait for a while;
	//  2) to decide how to deal with fatal, non-recoverable errors
	//     e.g. apicaller.ErrConnectImpossible.
	connectFilter := func(err error) error {
		cause := errors.Cause(err)
		if cause == apicaller.ErrConnectImpossible {
			return jworker.ErrTerminateAgent
		} else if cause == apicaller.ErrChangedPassword {
			return dependency.ErrBounce
		}
		return err
	}

	var externalUpdateProxyFunc func(proxy.Settings) error
	if runtime.GOOS == "linux" && !config.IsCaasConfig {
		externalUpdateProxyFunc = lxd.ConfigureLXDProxies
	}

	manifolds := dependency.Manifolds{
		// The agent manifold references the enclosing agent, and is the
		// foundation stone on which most other manifolds ultimately depend.
		agentName: agent.Manifold(config.Agent),

		// The termination worker returns ErrTerminateAgent if a
		// termination signal is received by the process it's running
		// in. It has no inputs and its only output is the error it
		// returns. It depends on the uninstall file having been
		// written *by the manual provider* at install time; it would
		// be Very Wrong Indeed to use SetCanUninstall in conjunction
		// with this code.
		terminationName: terminationworker.Manifold(),

		clockName: clockManifold(config.Clock),

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

		// The migration workers collaborate to run migrations;
		// and to create a mechanism for running other workers
		// so they can't accidentally interfere with a migration
		// in progress. Such a manifold should (1) depend on the
		// migration-inactive flag, to know when to start or die;
		// and (2) occupy the migration-fortress, so as to avoid
		// possible interference with the minion (which will not
		// take action until it's gained sole control of the
		// fortress).
		//
		// Note that the fortress itself will not be created
		// until the upgrade process is complete; this frees all
		// its dependencies from upgrade concerns.
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
			Logger:            loggo.GetLoggerWithTags("juju.worker.migrationminion", corelogger.MIGRATION),
		}),

		// The logging config updater is a leaf worker that indirectly
		// controls the messages sent via the log sender or rsyslog,
		// according to changes in environment config. We should only need
		// one of these in a consolidated agent.
		loggingConfigUpdaterName: ifNotMigrating(logger.Manifold(logger.ManifoldConfig{
			AgentName:       agentName,
			APICallerName:   apiCallerName,
			LoggingContext:  loggo.DefaultContext(),
			Logger:          loggo.GetLogger("juju.worker.logger"),
			UpdateAgentFunc: config.UpdateLoggerConfig,
		})),

		// The log sender is a leaf worker that sends log messages to some
		// API server, when configured so to do. We should only need one of
		// these in a consolidated agent.
		//
		// NOTE: the LogSource will buffer a large number of messages as an upgrade
		// runs; it currently seems better to fill the buffer and send when stable,
		// optimising for stable controller upgrades rather than up-to-the-moment
		// observable normal-machine upgrades.
		logSenderName: ifNotMigrating(logsender.Manifold(logsender.ManifoldConfig{
			APICallerName: apiCallerName,
			LogSource:     config.LogSource,
		})),

		identityFileWriterName: ifNotMigrating(identityfilewriter.Manifold(identityfilewriter.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
		})),

		traceName: trace.Manifold(trace.ManifoldConfig{
			AgentName:       agentName,
			Clock:           config.Clock,
			Logger:          loggo.GetLogger("juju.worker.trace"),
			NewTracerWorker: trace.NewTracerWorker,
			Kind:            coretrace.KindController,
		}),

		charmhubHTTPClientName: dependency.Manifold{
			Start: func(_ context.Context, _ dependency.Getter) (worker.Worker, error) {
				return engine.NewValueWorker(config.CharmhubHTTPClient)
			},
			Output: engine.ValueWorkerOutput,
		},

		// The proxy config updater is a leaf worker that sets http/https/apt/etc
		// proxy settings.
		proxyConfigUpdater: ifNotMigrating(proxyupdater.Manifold(proxyupdater.ManifoldConfig{
			AgentName:           agentName,
			APICallerName:       apiCallerName,
			Logger:              loggo.GetLogger("juju.worker.proxyupdater"),
			WorkerFunc:          proxyupdater.NewWorker,
			SupportLegacyValues: !config.IsCaasConfig,
			ExternalUpdate:      externalUpdateProxyFunc,
			InProcessUpdate:     proxyconfig.DefaultConfig.Set,
			RunFunc:             proxyupdater.RunWithStdIn,
		})),

		// TODO (thumper): It doesn't really make sense in a machine manifold as
		// not every machine will have credentials. It is here for the
		// ifCredentialValid function that is used solely for the machine
		// storage provisioner. It isn't clear to me why we have a storage
		// provisioner in the machine agent and the model workers.
		validCredentialFlagName: credentialvalidator.Manifold(credentialvalidator.ManifoldConfig{
			APICallerName: apiCallerName,
			NewFacade:     credentialvalidator.NewFacade,
			NewWorker:     credentialvalidator.NewWorker,
			Logger:        loggo.GetLogger("juju.worker.credentialvalidator"),
		}),
	}

	return manifolds
}

// IAASManifolds returns a set of co-configured manifolds covering the
// various responsibilities of a IAAS machine agent.
func IAASManifolds(config ManifoldsConfig) dependency.Manifolds {
	manifolds := dependency.Manifolds{
		toolsVersionCheckerName: ifNotMigrating(toolsversionchecker.Manifold(toolsversionchecker.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
		})),

		authenticationWorkerName: ifNotMigrating(authenticationworker.Manifold(authenticationworker.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
		})),

		hostKeyReporterName: ifNotMigrating(hostkeyreporter.Manifold(hostkeyreporter.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			RootDir:       config.RootDir,
			NewFacade:     hostkeyreporter.NewFacade,
			NewWorker:     hostkeyreporter.NewWorker,
		})),

		fanConfigurerName: ifNotMigrating(fanconfigurer.Manifold(fanconfigurer.ManifoldConfig{
			APICallerName: apiCallerName,
			Clock:         config.Clock,
		})),

		// The machiner Worker will wait for the identified machine to become
		// Dying and make it Dead; or until the machine becomes Dead by other
		// means. This worker needs to be launched after fanconfigurer
		// so that it reports interfaces created by it.
		machinerName: ifNotMigrating(machiner.Manifold(machiner.ManifoldConfig{
			AgentName:         agentName,
			APICallerName:     apiCallerName,
			FanConfigurerName: fanConfigurerName,
		})),

		// The diskmanager worker periodically lists block devices on the
		// machine it runs on. This worker will be run on all Juju-managed
		// machines (one per machine agent).
		diskManagerName: ifNotMigrating(diskmanager.Manifold(diskmanager.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
		})),

		// The api address updater is a leaf worker that rewrites agent config
		// as the state server addresses change. We should only need one of
		// these in a consolidated agent.
		apiAddressUpdaterName: ifNotMigrating(apiaddressupdater.Manifold(apiaddressupdater.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			Logger:        loggo.GetLogger("juju.worker.apiaddressupdater"),
		})),

		machineActionName: ifNotMigrating(machineactions.Manifold(machineactions.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			NewFacade:     machineactions.NewFacade,
			NewWorker:     machineactions.NewMachineActionsWorker,
			MachineLock:   config.MachineLock,
		})),

		// The upgrader is a leaf worker that returns a specific error
		// type recognised by the machine agent, causing other workers
		// to be stopped and the agent to be restarted running the new
		// tools. We should only need one of these in a consolidated
		// agent, but we'll need to be careful about behavioural
		// differences, and interactions with the upgrade-steps
		// worker.
		upgraderName: upgrader.Manifold(upgrader.ManifoldConfig{
			AgentName:            agentName,
			APICallerName:        apiCallerName,
			UpgradeStepsGateName: upgradeStepsGateName,
			UpgradeCheckGateName: upgradeCheckGateName,
			PreviousAgentVersion: config.PreviousAgentVersion,
			Logger:               loggo.GetLogger("juju.worker.upgrader"),
			Clock:                config.Clock,
		}),

		upgradeSeriesWorkerName: ifNotMigrating(upgradeseries.Manifold(upgradeseries.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			Logger:        loggo.GetLogger("juju.worker.upgradeseries"),
			NewFacade:     upgradeseries.NewFacade,
			NewWorker:     upgradeseries.NewWorker,
		})),

		// The upgradesteps worker runs soon after the machine agent
		// starts and runs any steps required to upgrade to the
		// running jujud version. Once upgrade steps have run, the
		// upgradesteps gate is unlocked and the worker exits.
		upgradeStepsName: upgradestepsmachine.Manifold(upgradestepsmachine.ManifoldConfig{
			AgentName:            agentName,
			APICallerName:        apiCallerName,
			UpgradeStepsGateName: upgradeStepsGateName,
			PreUpgradeSteps:      config.PreUpgradeSteps(state.ModelTypeIAAS),
			UpgradeSteps:         config.UpgradeSteps,
			NewAgentStatusSetter: config.NewAgentStatusSetter,
			Logger:               loggo.GetLogger("juju.worker.upgradesteps"),
			Clock:                config.Clock,
		}),

		// The deployer worker is primarily for deploying and recalling unit
		// agents, according to changes in a set of state units; and for the
		// final removal of its agents' units from state when they are no
		// longer needed.
		deployerName: ifFullyUpgraded(deployer.Manifold(deployer.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			Clock:         config.Clock,
			Hub:           config.LocalHub,
			Logger:        loggo.GetLogger("juju.worker.deployer"),

			UnitEngineConfig: config.UnitEngineConfig,
			SetupLogging:     config.SetupLogging,
			NewDeployContext: config.NewDeployContext,
		})),

		// The reboot manifold manages a worker which will reboot the
		// machine when requested. It needs an API connection and
		// waits for upgrades to be complete.
		rebootName: ifNotMigrating(reboot.Manifold(reboot.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			MachineLock:   config.MachineLock,
		})),

		// The storageProvisioner worker manages provisioning
		// (deprovisioning), and attachment (detachment) of first-class
		// volumes and filesystems.
		storageProvisionerName: ifNotMigrating(ifCredentialValid(storageprovisioner.MachineManifold(storageprovisioner.MachineManifoldConfig{
			AgentName:                    agentName,
			APICallerName:                apiCallerName,
			Clock:                        config.Clock,
			Logger:                       loggo.GetLogger("juju.worker.storageprovisioner"),
			NewCredentialValidatorFacade: common.NewCredentialInvalidatorFacade,
		}))),
		brokerTrackerName: ifNotMigrating(lxdbroker.Manifold(lxdbroker.ManifoldConfig{
			APICallerName: apiCallerName,
			AgentName:     agentName,
			MachineLock:   config.MachineLock,
			NewBrokerFunc: config.NewBrokerFunc,
			NewTracker:    lxdbroker.NewWorkerTracker,
		})),
		instanceMutaterName: ifNotMigrating(instancemutater.MachineManifold(instancemutater.MachineManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			BrokerName:    brokerTrackerName,
			Logger:        loggo.GetLogger("juju.worker.instancemutater.container"),
			NewClient:     instancemutater.NewClient,
			NewWorker:     instancemutater.NewContainerWorker,
		})),
		// The machineSetupName manifold runs small tasks required
		// to setup a machine, but requires the machine agent's API
		// connection. Once its work is complete, it stops.
		machineSetupName: ifNotMigrating(MachineStartupManifold(MachineStartupConfig{
			APICallerName:  apiCallerName,
			MachineStartup: config.MachineStartup,
			Logger:         loggo.GetLogger("juju.worker.machinesetup"),
		})),
		kvmContainerProvisioner: ifNotMigrating(provisioner.ContainerProvisioningManifold(provisioner.ContainerManifoldConfig{
			AgentName:                    agentName,
			APICallerName:                apiCallerName,
			Logger:                       loggo.GetLogger("juju.worker.kvmprovisioner"),
			MachineLock:                  config.MachineLock,
			NewCredentialValidatorFacade: common.NewCredentialInvalidatorFacade,
			ContainerType:                instance.KVM,
		})),
		lxdContainerProvisioner: ifNotMigrating(provisioner.ContainerProvisioningManifold(provisioner.ContainerManifoldConfig{
			AgentName:                    agentName,
			APICallerName:                apiCallerName,
			Logger:                       loggo.GetLogger("juju.worker.lxdprovisioner"),
			MachineLock:                  config.MachineLock,
			NewCredentialValidatorFacade: common.NewCredentialInvalidatorFacade,
			ContainerType:                instance.LXD,
		})),
		stateConverterName: ifNotMigrating(stateconverter.Manifold(stateconverter.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			Logger:        loggo.GetLogger("juju.worker.stateconverter"),
		})),
	}

	return mergeManifolds(config, manifolds)
}

// CAASManifolds returns a set of co-configured manifolds covering the
// various responsibilities of a CAAS machine agent.
func CAASManifolds(config ManifoldsConfig) dependency.Manifolds {
	return mergeManifolds(config, dependency.Manifolds{
		// TODO(caas) - when we support HA, only want this on primary
		upgraderName: caasupgrader.Manifold(caasupgrader.ManifoldConfig{
			AgentName:            agentName,
			APICallerName:        apiCallerName,
			UpgradeStepsGateName: upgradeStepsGateName,
			UpgradeCheckGateName: upgradeCheckGateName,
			PreviousAgentVersion: config.PreviousAgentVersion,
		}),

		// The upgradesteps worker runs soon after the machine agent
		// starts and runs any steps required to upgrade to the
		// running jujud version. Once upgrade steps have run, the
		// upgradesteps gate is unlocked and the worker exits.
		upgradeStepsName: upgradestepsmachine.Manifold(upgradestepsmachine.ManifoldConfig{
			AgentName:            agentName,
			APICallerName:        apiCallerName,
			UpgradeStepsGateName: upgradeStepsGateName,
			PreUpgradeSteps:      config.PreUpgradeSteps(state.ModelTypeCAAS),
			UpgradeSteps:         config.UpgradeSteps,
			NewAgentStatusSetter: config.NewAgentStatusSetter,
			Logger:               loggo.GetLogger("juju.worker.upgradesteps"),
			Clock:                config.Clock,
		}),

		// The CAAS units manager worker runs on CAAS agent and subscribes and handles unit topics on the localhub.
		caasUnitsManager: caasunitsmanager.Manifold(caasunitsmanager.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			Clock:         config.Clock,
			Logger:        loggo.GetLogger("juju.worker.caasunitsmanager"),
			Hub:           config.LocalHub,
		}),
	})
}

func mergeManifolds(config ManifoldsConfig, manifolds dependency.Manifolds) dependency.Manifolds {
	result := commonManifolds(config)
	for name, manifold := range manifolds {
		result[name] = manifold
	}
	return result
}

func clockManifold(clock clock.Clock) dependency.Manifold {
	return dependency.Manifold{
		Start: func(_ context.Context, _ dependency.Getter) (worker.Worker, error) {
			return engine.NewValueWorker(clock)
		},
		Output: engine.ValueWorkerOutput,
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

var ifCredentialValid = engine.Housing{
	Flags: []string{
		validCredentialFlagName,
	},
}.Decorate

const (
	agentName            = "agent"
	terminationName      = "termination-signal-handler"
	apiCallerName        = "api-caller"
	apiConfigWatcherName = "api-config-watcher"
	clockName            = "clock"

	upgraderName         = "upgrader"
	upgradeStepsName     = "upgrade-steps-runner"
	upgradeStepsGateName = "upgrade-steps-gate"
	upgradeStepsFlagName = "upgrade-steps-flag"
	upgradeCheckGateName = "upgrade-check-gate"
	upgradeCheckFlagName = "upgrade-check-flag"

	migrationFortressName     = "migration-fortress"
	migrationInactiveFlagName = "migration-inactive-flag"
	migrationMinionName       = "migration-minion"

	machineSetupName         = "machine-setup"
	rebootName               = "reboot-executor"
	loggingConfigUpdaterName = "logging-config-updater"
	diskManagerName          = "disk-manager"
	proxyConfigUpdater       = "proxy-config-updater"
	apiAddressUpdaterName    = "api-address-updater"
	machinerName             = "machiner"
	logSenderName            = "log-sender"
	deployerName             = "deployer"
	authenticationWorkerName = "ssh-authkeys-updater"
	storageProvisionerName   = "storage-provisioner"
	identityFileWriterName   = "ssh-identity-writer"
	toolsVersionCheckerName  = "tools-version-checker"
	machineActionName        = "machine-action-runner"
	hostKeyReporterName      = "host-key-reporter"
	fanConfigurerName        = "fan-configurer"
	instanceMutaterName      = "instance-mutater"
	auditConfigUpdaterName   = "audit-config-updater"
	stateConverterName       = "state-converter"
	lxdContainerProvisioner  = "lxd-container-provisioner"
	kvmContainerProvisioner  = "kvm-container-provisioner"

	upgradeSeriesWorkerName = "upgrade-series"

	traceName = "trace"

	caasUnitsManager = "caas-units-manager"

	validCredentialFlagName = "valid-credential-flag"

	brokerTrackerName = "broker-tracker"

	charmhubHTTPClientName = "charmhub-http-client"
)
