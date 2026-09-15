// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"context"
	"maps"
	"net/http"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v3"
	"github.com/juju/names/v6"
	"github.com/juju/proxy"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	"github.com/prometheus/client_golang/prometheus"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	proxyconfig "github.com/juju/juju/api/proxy/config"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/flightrecorder"
	corehttp "github.com/juju/juju/core/http"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	coretrace "github.com/juju/juju/core/trace"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/charmhub"
	containerbroker "github.com/juju/juju/internal/container/broker"
	"github.com/juju/juju/internal/container/lxd"
	internalhttp "github.com/juju/juju/internal/http"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/upgrades"
	jupgradesteps "github.com/juju/juju/internal/upgradesteps"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/agent"
	"github.com/juju/juju/internal/worker/apiaddressupdater"
	"github.com/juju/juju/internal/worker/apicaller"
	"github.com/juju/juju/internal/worker/apiconfigwatcher"
	"github.com/juju/juju/internal/worker/apiremotecaller"
	"github.com/juju/juju/internal/worker/apiserver"
	"github.com/juju/juju/internal/worker/apiservercertwatcher"
	"github.com/juju/juju/internal/worker/caasupgrader"
	lxdbroker "github.com/juju/juju/internal/worker/containerbroker"
	"github.com/juju/juju/internal/worker/containerprovisioner"
	"github.com/juju/juju/internal/worker/controlleragentconfig"
	"github.com/juju/juju/internal/worker/credentialvalidator"
	"github.com/juju/juju/internal/worker/dbaccessor"
	"github.com/juju/juju/internal/worker/deployer"
	"github.com/juju/juju/internal/worker/diskmanager"
	workerflightrecorder "github.com/juju/juju/internal/worker/flightrecorder"
	"github.com/juju/juju/internal/worker/fortress"
	"github.com/juju/juju/internal/worker/gate"
	"github.com/juju/juju/internal/worker/hostkeyreporter"
	"github.com/juju/juju/internal/worker/httpclient"
	"github.com/juju/juju/internal/worker/logger"
	"github.com/juju/juju/internal/worker/logrouter"
	"github.com/juju/juju/internal/worker/logsender"
	"github.com/juju/juju/internal/worker/lokiendpointupdater"
	"github.com/juju/juju/internal/worker/machineactions"
	"github.com/juju/juju/internal/worker/machiner"
	"github.com/juju/juju/internal/worker/migrationflag"
	"github.com/juju/juju/internal/worker/migrationminion"
	"github.com/juju/juju/internal/worker/modelworkermanager"
	"github.com/juju/juju/internal/worker/objectstore"
	"github.com/juju/juju/internal/worker/proxyupdater"
	"github.com/juju/juju/internal/worker/reboot"
	"github.com/juju/juju/internal/worker/sshkeyupdater"
	"github.com/juju/juju/internal/worker/sshsession"
	"github.com/juju/juju/internal/worker/storageprovisioner"
	"github.com/juju/juju/internal/worker/terminationworker"
	"github.com/juju/juju/internal/worker/trace"
	"github.com/juju/juju/internal/worker/traceconfigupdater"
	"github.com/juju/juju/internal/worker/upgrader"
	"github.com/juju/juju/internal/worker/upgradestepsagent"
)

// ManifoldsConfig allows specialisation of the result of Manifolds.
type ManifoldsConfig struct {
	// AgentName is the name of the machine agent, like "machine-12".
	// This will never change during the execution of an agent, and
	// is used to provide this as config into a worker rather than
	// making the worker get it from the agent worker itself.
	AgentName string

	// ControllerID is the numeric ID of the controller (e.g. "0" for
	// controller-0). It is passed directly to controller-only manifolds
	// that now take direct identity values instead of looking them up
	// through the agent manifold.
	ControllerID string

	// StartupValueProvider is used by workers that need to read values from the
	// agent config at startup, e.g. to get the API server certificate for the
	// apiservercertwatcher manifold. This is used instead of the agent manifold
	// to avoid unnecessary coupling and to allow these workers to be started
	// before the agent manifold.
	StartupValueProvider ControllerStartupValueProvider

	// ControllerUUID is the controller entity UUID. It is sourced from
	// agentConfig.Controller().Id() in makeEngineCreator and passed
	// directly to the lease-manager manifold instead of being looked
	// up from agent config at worker start.
	ControllerUUID string

	// ControllerModelUUID is the controller model UUID. It is sourced
	// from agentConfig.Model().Id() in makeEngineCreator and passed
	// directly to the lease-manager manifold instead of being looked
	// up from agent config at worker start.
	ControllerModelUUID string

	// ControllerAgentTag is the tag used for controller-agent log records.
	ControllerAgentTag names.Tag

	// LogDir is the controller process log directory for workers in this change
	// area that still take a fixed local path.
	LogDir string

	// ConfigChangeSocketPath is the path to the config-change reload socket.
	ConfigChangeSocketPath string

	// ControlSocketPath is the path to the local controller control socket.
	ControlSocketPath string

	// DataDir is the agent data directory used by bootstrap.
	DataDir string

	// APIPort is the controller API port advertised during bootstrap.
	APIPort int

	// AgentPassword is the agent password used during bootstrap finalization.
	AgentPassword string

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
	PreviousAgentVersion semversion.Number

	// BootstrapLock is passed to the bootstrap gate to coordinate
	// workers that shouldn't do anything until the bootstrap worker
	// is done.
	BootstrapLock gate.Lock

	// ProxyReadyLock is passed to the proxy ready gate to coordinate workers
	// that shouldn't do anything until the proxyupdater worker is done.
	ProxyReadyLock gate.Lock

	// UpgradeDBLock is passed to the upgrade database gate to
	// coordinate workers that shouldn't do anything until the
	// upgrade-database worker is done.
	UpgradeDBLock gate.Lock

	// UpgradeStepsLock is passed to the upgrade steps gate to
	// coordinate workers that shouldn't do anything until the
	// upgrade-steps worker is done.
	UpgradeStepsLock gate.Lock

	// UpgradeCheckLock is passed to the upgrade check gate to
	// coordinate workers that shouldn't do anything until the
	// upgrader worker completes it's first check.
	UpgradeCheckLock gate.Lock

	// ControllerAgentConfigReadyLock is passed to the controller agent
	// config ready gate to coordinate the deployer with the
	// controlleragentconfig worker. The controlleragentconfig worker runs
	// on every machine and unlocks this lock once the configchange.socket
	// is serving, so the deployer never starts before the socket exists.
	ControllerAgentConfigReadyLock gate.Lock

	// NewDBWorkerFunc returns a tracked db worker.
	NewDBWorkerFunc dbaccessor.NewDBWorkerFunc

	// PreUpgradeSteps is a function that is used by the upgradesteps
	// worker to ensure that conditions are OK for an upgrade to
	// proceed.
	PreUpgradeSteps func(model.ModelType) upgrades.PreUpgradeStepsFunc

	// UpgradeSteps is a function that is used by the upgradesteps
	// worker to perform the upgrade steps.
	UpgradeSteps upgrades.UpgradeStepsFunc

	// LogSource supplies log records to the logrouter. It is the output
	// channel of the BufferedLogWriter installed on the default loggo
	// context.
	LogSource logsender.LogRecordCh

	// LegacyLogSinkWriter is the TaggedRedirectWriter that writes log
	// records to the legacy log sink. It is managed by the logrouter
	// based on the active backend mode.
	LegacyLogSinkWriter loggo.Writer

	// LocalLogSink is the primed local log sink used for
	// controller-local log delivery. The controller-only model
	// logrouter uses this in logsink mode so controller model
	// loggers can switch away from the file sink without
	// depending on the API caller.
	LocalLogSink corelogger.LogSink

	// NewDeployContext gives the tests the opportunity to create a
	// deployer.Context that can be used for testing.
	NewDeployContext func(deployer.ContextConfig) (deployer.Context, error)

	// Clock supplies timekeeping services to various workers.
	Clock clock.Clock

	// FlightRecorder is used to record significant events.
	FlightRecorder flightrecorder.FlightRecorderWorker

	// ValidateMigration is called by the migrationminion during the
	// migration process to check that the agent will be ok when
	// connected to the new target controller.
	ValidateMigration func(context.Context, base.APICaller) error

	// PrometheusRegisterer is a prometheus.Registerer that may be used
	// by workers to register Prometheus metric collectors.
	PrometheusRegisterer prometheus.Registerer

	// UpdateLoggerConfig is a function that will save the specified
	// config value as the logging config in the agent.conf file.
	UpdateLoggerConfig func(string) error

	// NewAgentStatusSetter provides upgradesteps.StatusSetter.
	NewAgentStatusSetter func(context.Context, base.APICaller) (jupgradesteps.StatusSetter, error)

	// ControllerLeaseDuration defines for how long this agent will ask
	// for controller administration rights.
	ControllerLeaseDuration time.Duration

	// TransactionPruneInterval defines how frequently mgo/txn transactions
	// are pruned from the database.
	TransactionPruneInterval time.Duration

	// RegisterIntrospectionHTTPHandlers is a function that calls the
	// supplied function to register introspection HTTP handlers. The
	// function will be passed a path and a handler; the function may
	// alter the path as it sees fit, e.g. by adding a prefix.
	RegisterIntrospectionHTTPHandlers func(func(path string, _ http.Handler))

	// NewModelWorker returns a new worker for managing the model with
	// the specified UUID and type.
	NewModelWorker modelworkermanager.NewModelWorkerFunc

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

	// UnitEngineConfig is used by the deployer to initialize the unit's
	// dependency engine when running in the nested context.
	UnitEngineConfig func() dependency.EngineConfig

	// SetupLogging is used by the deployer to initialize the logging
	// context for the unit.
	SetupLogging func(corelogger.LoggerContext, coreagent.Config)

	// DependencyEngineMetrics creates a set of metrics for a model, so it's
	// possible to know the lifecycle of the workers in the dependency engine.
	DependencyEngineMetrics modelworkermanager.ModelMetrics

	// NewEnvironFunc is a function opens a provider "environment"
	// (typically environs.New).
	NewEnvironFunc func(context.Context, environs.OpenParams, environs.CredentialInvalidator) (environs.Environ, error)

	// NewCAASBrokerFunc is a function opens a CAAS broker.
	NewCAASBrokerFunc func(context.Context, environs.OpenParams, environs.CredentialInvalidator) (caas.Broker, error)

	// MachineStartup is passed to the machine manifold. It does
	// machine setup work which relies on an API connection.
	MachineStartup func(context.Context, api.Connection, corelogger.Logger) error
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

	apiBackedLogRouterRegisterer := prometheus.WrapRegistererWith(
		prometheus.Labels{"log_router": "api_backed"},
		config.PrometheusRegisterer,
	)

	manifolds := dependency.Manifolds{
		// The agent manifold references the enclosing agent, and is the
		// foundation stone on which most other manifolds ultimately depend.
		agentName: agent.Manifold(config.Agent),

		// controllerAgentConfigReadyGateName/FlagName coordinate the deployer
		// with the controlleragentconfig worker. The deployer must not start
		// until the configchange.socket is available. The lock is unlocked by
		// the controlleragentconfig worker on every machine, so the deployer
		// always waits for the socket, regardless of controller status.
		controllerAgentConfigReadyGateName: gate.ManifoldEx(config.ControllerAgentConfigReadyLock),
		controllerAgentConfigReadyFlagName: gate.FlagManifold(gate.FlagManifoldConfig{
			GateName:  controllerAgentConfigReadyGateName,
			NewWorker: gate.NewFlagWorker,
		}),

		// The termination worker returns ErrTerminateAgent if a
		// termination signal is received by the process it's running
		// in. It has no inputs and its only output is the error it
		// returns. It depends on the uninstall file having been
		// written *by the unmanaged provider* at install time; it would
		// be Very Wrong Indeed to use SetCanUninstall in conjunction
		// with this code.
		terminationName: terminationworker.Manifold(),

		clockName: clockManifold(config.Clock),

		flightRecorderName: workerflightrecorder.Manifold(config.FlightRecorder),

		// Controller agent config manifold watches the controller
		// agent config and bounces if it changes. It deliberately runs
		// on every machine (not just controllers) so that it creates
		// configchange.socket and unlocks controllerAgentConfigReadyLock
		// before the deployer starts. A machine may be promoted to a
		// controller later, and the controller charm's install hook
		// connects to that socket, so it must already exist.
		controllerAgentConfigName: controlleragentconfig.Manifold(controlleragentconfig.ManifoldConfig{
			ControllerID:      config.ControllerID,
			Logger:            internallogger.GetLogger("juju.worker.controlleragentconfig"),
			NewSocketListener: controlleragentconfig.NewSocketListener,
			SocketName:        config.ConfigChangeSocketPath,
			ReadyUnlocker:     config.ControllerAgentConfigReadyLock,
		}),

		// The api-config-watcher manifold monitors the API server
		// addresses in the agent config and bounces when they
		// change. It's required as part of model migrations.
		apiConfigWatcherName: apiconfigwatcher.Manifold(apiconfigwatcher.ManifoldConfig{
			AgentName:          agentName,
			AgentConfigChanged: config.AgentConfigChanged,
			Logger:             internallogger.GetLogger("juju.worker.apiconfigwatcher"),
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
			Logger:               internallogger.GetLogger("juju.worker.apicaller"),
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
			AgentName:             agentName,
			APICallerName:         apiCallerName,
			FortressName:          migrationFortressName,
			Clock:                 config.Clock,
			APIOpen:               api.Open,
			ValidateMigration:     config.ValidateMigration,
			NewWorker:             migrationminion.NewWorker,
			Logger:                internallogger.GetLogger("juju.worker.migrationminion", corelogger.MIGRATION),
			SendReport:            migrationminion.SendReport,
			FetchTargetLokiConfig: migrationminion.FetchTargetLokiConfig,
		}),

		// The logging config updater is a leaf worker that indirectly
		// controls the messages sent via the log sender or rsyslog,
		// according to changes in environment config. We should only need
		// one of these in a consolidated agent.
		loggingConfigUpdaterName: ifNotMigrating(logger.Manifold(logger.ManifoldConfig{
			AgentName:       agentName,
			APICallerName:   apiCallerName,
			LoggerContext:   internallogger.DefaultContext(),
			Logger:          internallogger.GetLogger("juju.worker.logger"),
			UpdateAgentFunc: config.UpdateLoggerConfig,
		})),

		// The api address updater is a leaf worker that rewrites agent config
		// as the state server addresses change. We should only need one of
		// these in a consolidated agent.
		apiAddressUpdaterName: ifNotMigrating(apiaddressupdater.Manifold(apiaddressupdater.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			Logger:        internallogger.GetLogger("juju.worker.apiaddressupdater"),
		})),

		lokiEndpointUpdaterName: ifNotMigrating(lokiendpointupdater.Manifold(lokiendpointupdater.ManifoldConfig{
			AgentName:          agentName,
			APICallerName:      apiCallerName,
			AgentConfigChanged: config.AgentConfigChanged,
			Logger:             internallogger.GetLogger("juju.worker.lokiendpointupdater"),
		})),

		traceConfigUpdaterName: ifNotMigrating(traceconfigupdater.Manifold(traceconfigupdater.ManifoldConfig{
			AgentName:          agentName,
			APICallerName:      apiCallerName,
			AgentConfigChanged: config.AgentConfigChanged,
			Logger:             internallogger.GetLogger("juju.worker.traceconfigupdater"),
		})),

		// The log router owns the buffered log stream and forwards records to
		// one active backend at a time.
		logRouterName: ifNotMigrating(logrouter.Manifold(logrouter.ManifoldConfig{
			AgentName:            agentName,
			APICallerName:        apiCallerName,
			HTTPClientName:       httpClientName,
			LogSource:            config.LogSource,
			AgentConfigChanged:   config.AgentConfigChanged,
			Logger:               internallogger.GetLogger("juju.worker.logrouter"),
			Clock:                config.Clock,
			PrometheusRegisterer: apiBackedLogRouterRegisterer,
			NewBackendFunc:       logrouter.NewBackend,
			RemoveLegacyLogSinkWriter: func() {
				logsender.RemoveLegacyLogSinkWriter()
			},
			AddLegacyLogSinkWriter: func() error {
				return logsender.AddLegacyLogSinkWriter(config.LegacyLogSinkWriter)
			},
		})),

		traceName: trace.Manifold(trace.ManifoldConfig{
			AgentName:          agentName,
			AgentConfigChanged: config.AgentConfigChanged,
			Clock:              config.Clock,
			Logger:             internallogger.GetLogger("juju.worker.trace"),
			NewTracerWorker:    trace.NewTracerWorker,
			Kind:               coretrace.KindController,
		}),

		// TODO (thumper): It doesn't really make sense in a machine manifold as
		// not every machine will have credentials. It is here for the
		// ifCredentialValid function that is used solely for the machine
		// storage provisioner. It isn't clear to me why we have a storage
		// provisioner in the machine agent and the model workers.
		validCredentialFlagName: credentialvalidator.Manifold(credentialvalidator.ManifoldConfig{
			APICallerName: apiCallerName,
			NewFacade:     credentialvalidator.NewFacade,
			NewWorker:     credentialvalidator.NewWorker,
			Logger:        internallogger.GetLogger("juju.worker.credentialvalidator"),
		}),

		httpClientName: httpclient.Manifold(httpclient.ManifoldConfig{
			NewHTTPClient: func(namespace corehttp.Purpose, opts ...internalhttp.Option) *internalhttp.Client {
				switch namespace {
				case corehttp.CharmhubPurpose:
					l := internallogger.GetLogger("juju.charmhub", corelogger.CHARMHUB)
					opts = append(
						opts,
						internalhttp.WithLogger(l),
						internalhttp.WithRequestRetrier(charmhub.DefaultRetryPolicy()),
					)

				case corehttp.S3Purpose:
					l := internallogger.GetLogger("juju.objectstore.s3", corelogger.OBJECTSTORE)
					opts = append(opts, internalhttp.WithLogger(l))

				case corehttp.SSHImporterPurpose:
					l := internallogger.GetLogger("juju.ssh.importer", corelogger.SSHIMPORTER)
					opts = append(opts, internalhttp.WithLogger(l))

				case corehttp.MacaroonPurpose:
					l := internallogger.GetLogger("juju.macaroon", corelogger.MACAROON)
					opts = append(opts, internalhttp.WithLogger(l))

				case corehttp.LokiPurpose:
					l := internallogger.GetLogger("juju.loki")
					opts = append(opts, internalhttp.WithLogger(l))

				case corehttp.SimpleStreamPurpose:
					l := internallogger.GetLogger("juju.simplestream", corelogger.SIMPLESTREAM)
					opts = append(opts, internalhttp.WithLogger(l))
				}

				return internalhttp.NewClient(opts...)
			},
			NewHTTPClientWorker:  httpclient.NewTrackedWorker,
			PrometheusRegisterer: config.PrometheusRegisterer,
			NewMetricsCollector:  httpclient.NewMetricsCollector,
			Clock:                config.Clock,
			Logger:               internallogger.GetLogger("juju.worker.httpclient"),
		}),
	}

	return manifolds
}

// IAASManifolds returns a set of co-configured manifolds covering the
// various responsibilities of a IAAS machine agent.
func IAASManifolds(config ManifoldsConfig) dependency.Manifolds {
	manifolds := dependency.Manifolds{
		// The proxy config updater is a leaf worker that sets http/https/apt/etc
		// proxy settings for non-controller agents using the API server.
		proxyConfigUpdater: ifNotMigrating(proxyupdater.Manifold(proxyupdater.ManifoldConfig{
			AgentName:           agentName,
			APICallerName:       apiCallerName,
			Logger:              internallogger.GetLogger("juju.worker.proxyupdater"),
			WorkerFunc:          proxyupdater.NewWorker,
			SupportLegacyValues: true,
			ExternalUpdate:      lxd.ConfigureLXDProxies,
			InProcessUpdate:     proxyconfig.DefaultConfig.Set,
			RunFunc:             proxyupdater.RunWithStdIn,
		})),

		sshKeyUpdaterWorkerName: ifNotMigrating(sshkeyupdater.Manifold(sshkeyupdater.Output)),

		sshSessionName: ifNotMigrating(sshsession.Manifold(sshsession.ManifoldConfig{
			AgentName:               agentName,
			APICallerName:           apiCallerName,
			SshKeyUpdaterWorkerName: sshKeyUpdaterWorkerName,
			Logger:                  internallogger.GetLogger("juju.worker.sshsession"),
			NewWorker:               sshsession.NewWorker,
			NewFacadeClient:         sshsession.NewFacadeClient,
		})),

		hostKeyReporterName: ifNotMigrating(hostkeyreporter.Manifold(hostkeyreporter.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			RootDir:       config.RootDir,
			NewFacade:     hostkeyreporter.NewFacade,
			NewWorker:     hostkeyreporter.NewWorker,
		})),

		// The machiner Worker will wait for the identified machine to become
		// Dying and make it Dead; or until the machine becomes Dead by other
		// means.
		machinerName: ifNotMigrating(machiner.Manifold(machiner.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
		})),

		// The diskmanager worker periodically lists block devices on the
		// machine it runs on. This worker will be run on all Juju-managed
		// machines (one per machine agent).
		diskManagerName: ifNotMigrating(diskmanager.Manifold(diskmanager.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
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
			Logger:               internallogger.GetLogger("juju.worker.upgrader"),
			Clock:                config.Clock,
		}),

		upgradeAgentStepsName: upgradestepsagent.Manifold(upgradestepsagent.ManifoldConfig{
			AgentName:            agentName,
			APICallerName:        apiCallerName,
			UpgradeStepsGateName: upgradeStepsGateName,
			PreUpgradeSteps:      config.PreUpgradeSteps(model.IAAS),
			UpgradeSteps:         config.UpgradeSteps,
			NewAgentStatusSetter: config.NewAgentStatusSetter,
			NewAgentWorker:       upgradestepsagent.NewAgentWorker,
			Logger:               internallogger.GetLogger("juju.worker.upgradestepsagent"),
			Clock:                config.Clock,
		}),

		// The deployer worker is primarily for deploying and recalling unit
		// agents, according to changes in a set of state units; and for the
		// final removal of its agents' units from state when they are no
		// longer needed. On controller machines it must also wait until the
		// controlleragentconfig socket is ready (controllerAgentConfigReadyFlag)
		// so the controller charm's install hook can reach the socket. On
		// non-controller machines that flag is pre-unlocked.
		deployerName: ifControllerAgentConfigNeededAndReady(ifFullyUpgraded(deployer.Manifold(deployer.ManifoldConfig{
			AgentName:          agentName,
			APICallerName:      apiCallerName,
			HTTPClientName:     httpClientName,
			AgentConfigChanged: config.AgentConfigChanged,
			FlightRecorder:     config.FlightRecorder,
			Clock:              config.Clock,
			Logger:             internallogger.GetLogger("juju.worker.deployer"),

			UnitEngineConfig: config.UnitEngineConfig,
			SetupLogging:     config.SetupLogging,
			NewDeployContext: config.NewDeployContext,
		}))),

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
			AgentName:     agentName,
			APICallerName: apiCallerName,
			Clock:         config.Clock,
			Logger:        internallogger.GetLogger("juju.worker.storageprovisioner"),
		}))),
		brokerTrackerName: ifNotMigrating(lxdbroker.Manifold(lxdbroker.ManifoldConfig{
			APICallerName: apiCallerName,
			AgentName:     agentName,
			MachineLock:   config.MachineLock,
			NewBrokerFunc: config.NewBrokerFunc,
			NewTracker:    lxdbroker.NewWorkerTracker,
		})),
		lxdContainerProvisioner: ifNotMigrating(containerprovisioner.Manifold(containerprovisioner.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			Logger:        internallogger.GetLogger("juju.worker.lxdprovisioner"),
			MachineLock:   config.MachineLock,
			ContainerType: instance.LXD,
		})),

		// The machineSetupName manifold runs small tasks required
		// to setup a machine, but requires the machine agent's API
		// connection. Once its work is complete, it stops.
		machineSetupName: ifNotMigrating(MachineStartupManifold(MachineStartupConfig{
			APICallerName:  apiCallerName,
			MachineStartup: config.MachineStartup,
			Logger:         internallogger.GetLogger("juju.worker.machinesetup"),
		})),
	}

	return mergeManifolds(config, manifolds)
}

// CAASManifolds returns a set of co-configured manifolds covering the
// various responsibilities of a CAAS machine agent.
func CAASManifolds(config ManifoldsConfig) dependency.Manifolds {
	return mergeManifolds(config, dependency.Manifolds{
		// The proxy config updater is a leaf worker that sets http/https/apt/etc
		// proxy settings for non-controller agents using the API server.
		proxyConfigUpdater: ifNotMigrating(proxyupdater.Manifold(proxyupdater.ManifoldConfig{
			AgentName:           agentName,
			APICallerName:       apiCallerName,
			Logger:              internallogger.GetLogger("juju.worker.proxyupdater"),
			WorkerFunc:          proxyupdater.NewWorker,
			SupportLegacyValues: false,
			ExternalUpdate:      func(proxy.Settings) error { return nil },
			InProcessUpdate:     proxyconfig.DefaultConfig.Set,
			RunFunc:             proxyupdater.RunWithStdIn,
		})),

		// TODO(caas) - when we support HA, only want this on primary
		upgraderName: caasupgrader.Manifold(caasupgrader.ManifoldConfig{
			AgentName:            agentName,
			APICallerName:        apiCallerName,
			UpgradeStepsGateName: upgradeStepsGateName,
			UpgradeCheckGateName: upgradeCheckGateName,
			PreviousAgentVersion: config.PreviousAgentVersion,
		}),

		upgradeAgentStepsName: upgradestepsagent.Manifold(upgradestepsagent.ManifoldConfig{
			AgentName:            agentName,
			APICallerName:        apiCallerName,
			UpgradeStepsGateName: upgradeStepsGateName,
			PreUpgradeSteps:      config.PreUpgradeSteps(model.CAAS),
			UpgradeSteps:         config.UpgradeSteps,
			NewAgentStatusSetter: config.NewAgentStatusSetter,
			NewAgentWorker:       upgradestepsagent.NewAgentWorker,
			Logger:               internallogger.GetLogger("juju.worker.upgradestepsagent"),
			Clock:                config.Clock,
		}),
	})
}

func mergeManifolds(config ManifoldsConfig, manifolds dependency.Manifolds) dependency.Manifolds {
	result := commonManifolds(config)
	maps.Copy(result, manifolds)
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

// ifControllerAgentConfigNeededAndReady gates a manifold on two conditions:
// "needed"  - the machine is a controller, so configchange.socket must exist
//
//	before the gated worker starts (e.g. the controller charm's
//	install hook connects to it); on non-controller machines the gate
//	lock is pre-unlocked by the caller, making this a no-op there.
//
// "ready"   - controlleragentconfig has started its socket listener,
//
//	meaning configchange.socket is on disk (created synchronously
//	inside NewWorker before the manifold reports as running).
var ifControllerAgentConfigNeededAndReady = engine.Housing{
	Flags: []string{
		controllerAgentConfigReadyFlagName,
	},
}.Decorate

// ControllerStartupValueProvider is the set of methods required to provide
// startup values to controller-only workers. This is implemented by the
// config.StartupValueProvider, which is passed to the manifolds in the config.
type ControllerStartupValueProvider interface {
	objectstore.RootDirReader
	dbaccessor.ControllerStartupValuesProvider
	apiservercertwatcher.CertReader
	apiserver.LocalConfigReader
	apiremotecaller.APIInfoProvider
	logrouter.LokiConfigProvider
}

const (
	agentName            = "agent"
	terminationName      = "termination-signal-handler"
	apiCallerName        = "api-caller"
	apiConfigWatcherName = "api-config-watcher"
	clockName            = "clock"
	flightRecorderName   = "flight-recorder"

	upgraderName          = "upgrader"
	upgradeAgentStepsName = "upgrade-agent-steps-runner"
	upgradeStepsGateName  = "upgrade-steps-gate"
	upgradeStepsFlagName  = "upgrade-steps-flag"
	upgradeCheckGateName  = "upgrade-check-gate"
	upgradeCheckFlagName  = "upgrade-check-flag"

	migrationFortressName     = "migration-fortress"
	migrationInactiveFlagName = "migration-inactive-flag"
	migrationMinionName       = "migration-minion"

	apiAddressUpdaterName              = "api-address-updater"
	sshKeyUpdaterWorkerName            = "ssh-authkeys-updater"
	brokerTrackerName                  = "broker-tracker"
	controllerAgentConfigName          = "controller-agent-config"
	controllerAgentConfigReadyGateName = "controller-agent-config-ready-gate"
	controllerAgentConfigReadyFlagName = "controller-agent-config-ready-flag"
	deployerName                       = "deployer"
	diskManagerName                    = "disk-manager"
	hostKeyReporterName                = "host-key-reporter"
	httpClientName                     = "http-client"
	loggingConfigUpdaterName           = "logging-config-updater"
	lokiEndpointUpdaterName            = "loki-endpoint-updater"
	traceConfigUpdaterName             = "trace-config-updater"
	logRouterName                      = "log-router"
	lxdContainerProvisioner            = "lxd-container-provisioner"
	machineActionName                  = "machine-action-runner"
	machinerName                       = "machiner"
	proxyConfigUpdater                 = "proxy-config-updater"
	rebootName                         = "reboot-executor"
	sshSessionName                     = "ssh-session"
	storageProvisionerName             = "storage-provisioner"
	traceName                          = "trace"
	validCredentialFlagName            = "valid-credential-flag"
	machineSetupName                   = "machine-setup"
)
