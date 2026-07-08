// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"maps"
	"net/http"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	"github.com/prometheus/client_golang/prometheus"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller/crosscontroller"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/flightrecorder"
	corehttp "github.com/juju/juju/core/http"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/environs"
	internalbootstrap "github.com/juju/juju/internal/bootstrap"
	"github.com/juju/juju/internal/charmhub"
	internalhttp "github.com/juju/juju/internal/http"
	internallease "github.com/juju/juju/internal/lease"
	internallogger "github.com/juju/juju/internal/logger"
	internalobjectstore "github.com/juju/juju/internal/objectstore"
	"github.com/juju/juju/internal/upgrades"
	"github.com/juju/juju/internal/upgradesteps"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/apiaddresssetter"
	"github.com/juju/juju/internal/worker/apicaller"
	"github.com/juju/juju/internal/worker/apiremotecaller"
	"github.com/juju/juju/internal/worker/apiremoterelationcaller"
	"github.com/juju/juju/internal/worker/apiserver"
	"github.com/juju/juju/internal/worker/apiservercertwatcher"
	"github.com/juju/juju/internal/worker/auditconfigupdater"
	"github.com/juju/juju/internal/worker/bootstrap"
	"github.com/juju/juju/internal/worker/certupdater"
	"github.com/juju/juju/internal/worker/changestream"
	"github.com/juju/juju/internal/worker/changestreampruner"
	"github.com/juju/juju/internal/worker/controlleragentconfig"
	"github.com/juju/juju/internal/worker/controllerlogger"
	"github.com/juju/juju/internal/worker/controllerlokiupdater"
	"github.com/juju/juju/internal/worker/controllerpresence"
	"github.com/juju/juju/internal/worker/controlsocket"
	"github.com/juju/juju/internal/worker/dbaccessor"
	workerdomainservices "github.com/juju/juju/internal/worker/domainservices"
	"github.com/juju/juju/internal/worker/externalcontrollerupdater"
	"github.com/juju/juju/internal/worker/filenotifywatcher"
	workerflightrecorder "github.com/juju/juju/internal/worker/flightrecorder"
	"github.com/juju/juju/internal/worker/fortress"
	"github.com/juju/juju/internal/worker/gate"
	"github.com/juju/juju/internal/worker/httpclient"
	"github.com/juju/juju/internal/worker/httpserver"
	"github.com/juju/juju/internal/worker/httpserverargs"
	"github.com/juju/juju/internal/worker/identityfilewriter"
	"github.com/juju/juju/internal/worker/jwtparser"
	leasemanager "github.com/juju/juju/internal/worker/lease"
	"github.com/juju/juju/internal/worker/leaseexpiry"
	"github.com/juju/juju/internal/worker/logrouter"
	"github.com/juju/juju/internal/worker/logsink"
	"github.com/juju/juju/internal/worker/migrationminion"
	"github.com/juju/juju/internal/worker/modelworkermanager"
	"github.com/juju/juju/internal/worker/objectstore"
	"github.com/juju/juju/internal/worker/objectstoredrainer"
	"github.com/juju/juju/internal/worker/objectstorefacade"
	"github.com/juju/juju/internal/worker/objectstores3caller"
	"github.com/juju/juju/internal/worker/objectstoreservices"
	"github.com/juju/juju/internal/worker/providerservices"
	"github.com/juju/juju/internal/worker/providertracker"
	"github.com/juju/juju/internal/worker/querylogger"
	"github.com/juju/juju/internal/worker/secretbackendrotate"
	"github.com/juju/juju/internal/worker/singular"
	"github.com/juju/juju/internal/worker/sshserver"
	"github.com/juju/juju/internal/worker/storageregistry"
	"github.com/juju/juju/internal/worker/terminationworker"
	"github.com/juju/juju/internal/worker/trace"
	"github.com/juju/juju/internal/worker/traceservices"
	"github.com/juju/juju/internal/worker/undertaker"
	"github.com/juju/juju/internal/worker/upgradedatabase"
	"github.com/juju/juju/internal/worker/upgrader"
	"github.com/juju/juju/internal/worker/upgradeservices"
	"github.com/juju/juju/internal/worker/upgradestepscontroller"
	"github.com/juju/juju/internal/worker/watcherregistry"
)

// ManifoldsConfig allows specialisation of the result of
// Manifolds.
type ManifoldsConfig struct {
	// AgentName is the name of the controller agent, like
	// "controller-0". This will never change during the execution of
	// an agent, and is used to provide this as config into a worker
	// rather than making the worker get it from the agent worker
	// itself.
	AgentName string

	// ControllerID is the numeric ID of the controller (e.g. "0" for
	// controller-0). It is passed directly to manifolds that require
	// the controller's identity without needing the agent manifold for
	// the lookup.
	ControllerID string

	// StartupValueProvider is used to read values from the controller runtime
	// config file, which is passed to the manifolds in the config.
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

	// ControllerTag is the tag identifying the controller entity
	// (e.g. controller-UUID). It is passed directly to manifolds that
	// require it instead of being looked up from the agent config.
	ControllerTag names.ControllerTag

	// LogDir is the controller process log directory for workers in this change
	// area that still take a fixed local path.
	LogDir string

	// ControllerRuntimePath is the path to the controller runtime
	// configuration file (runtime.conf). It is used by the controller
	// loki config updater to persist Loki endpoint changes.
	ControllerRuntimePath string

	// ConfigChangeSocketPath is the path to the config-change reload socket.
	ConfigChangeSocketPath string

	// ControlSocketPath is the path to the local controller control socket.
	ControlSocketPath string

	// DataDir is the controller agent data directory used by bootstrap.
	DataDir string

	// APIPort is the controller API port advertised during bootstrap.
	APIPort int

	// AgentPassword is the controller agent password used during bootstrap
	// finalization.
	AgentPassword string

	// RootDir is the root directory that any worker that needs to
	// access local filesystems should use as a base. In actual use it
	// will be "" but it may be overridden in tests.
	RootDir string

	// PreviousAgentVersion passes through the version the controller
	// agent was running before the current restart.
	PreviousAgentVersion semversion.Number

	// BootstrapLock is passed to the bootstrap gate to coordinate
	// workers that should not do anything until the bootstrap worker
	// is done.
	BootstrapLock gate.Lock

	// UpgradeDBLock is passed to the upgrade database gate to
	// coordinate workers that should not do anything until the
	// upgrade-database worker is done.
	UpgradeDBLock gate.Waiter

	// UpgradeStepsLock is passed to the upgrade steps gate to
	// coordinate workers that should not do anything until the
	// upgrade-steps worker is done.
	UpgradeStepsLock gate.Lock

	// UpgradeCheckLock is passed to the upgrade check gate to
	// coordinate workers that should not do anything until the
	// upgrader worker completes its first check.
	UpgradeCheckLock gate.Lock

	// ControllerUpgradeLock keeps upgrade and migration workers inactive
	// during fresh controller bootstrap, while those flows are still
	// out-of-scope and their API/server-side support is not yet in
	// place. This temporary outer gate should be removed once the
	// controller upgrade and migration flows are fully implemented.
	ControllerUpgradeLock gate.Lock

	// NewDBWorkerFunc returns a tracked db worker.
	NewDBWorkerFunc dbaccessor.NewDBWorkerFunc

	// PreUpgradeSteps is a function that is used by the upgradesteps
	// worker to ensure that conditions are OK for an upgrade to
	// proceed.
	PreUpgradeSteps func(model.ModelType) upgrades.PreUpgradeStepsFunc

	// UpgradeSteps is a function that is used by the upgradesteps
	// worker to perform the upgrade steps.
	UpgradeSteps upgrades.UpgradeStepsFunc

	// LogSink defines an interface for writing log records to a log
	// sink.
	LogSink corelogger.LogSink

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
	NewAgentStatusSetter func(context.Context, base.APICaller) (upgradesteps.StatusSetter, error)

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

	// MuxShutdownWait is the maximum time the http-server worker will
	// wait for all mux clients to gracefully terminate before the
	// http-worker exits regardless.
	MuxShutdownWait time.Duration

	// SetupLogging is used to initialize the logging context for model
	// workers.
	SetupLogging func(corelogger.LoggerContext, coreagent.Config)

	// DependencyEngineMetrics creates a set of metrics for a model, so
	// it is possible to know the lifecycle of the workers in the
	// dependency engine.
	DependencyEngineMetrics modelworkermanager.ModelMetrics

	// NewEnvironFunc is a function that opens a provider
	// "environment" (typically environs.New).
	NewEnvironFunc func(context.Context, environs.OpenParams, environs.CredentialInvalidator) (environs.Environ, error)
}

// commonManifolds returns the shared controller manifolds.
func commonManifolds(config ManifoldsConfig) dependency.Manifolds {
	newExternalControllerWatcherClient := func(ctx context.Context, apiInfo *api.Info) (
		externalcontrollerupdater.ExternalControllerWatcherClientCloser, string, error,
	) {
		conn, err := apicaller.NewExternalControllerConnection(ctx, apiInfo)
		if err != nil {
			return nil, "", errors.Trace(err)
		}
		return crosscontroller.NewClient(conn), conn.IPAddr(), nil
	}
	logRouterConfigChanged := voyeur.NewValue(false)

	return dependency.Manifolds{
		// Bootstrap gate/flag manifolds coordinate workers that should
		// not do anything until the bootstrap worker is done.
		isBootstrapGateName: gate.ManifoldEx(config.BootstrapLock),
		isBootstrapFlagName: gate.FlagManifold(gate.FlagManifoldConfig{
			GateName:  isBootstrapGateName,
			NewWorker: gate.NewFlagWorker,
		}),

		controllerUpgradeGateName: gate.ManifoldEx(config.ControllerUpgradeLock),
		controllerUpgradeFlagName: gate.FlagManifold(gate.FlagManifoldConfig{
			GateName:  controllerUpgradeGateName,
			NewWorker: gate.NewFlagWorker,
		}),

		// The termination worker returns ErrTerminateAgent if a
		// termination signal is received by the process it's running
		// in.
		terminationName: terminationworker.Manifold(),

		flightRecorderName: workerflightrecorder.Manifold(config.FlightRecorder),

		// Controller agent config manifold watches the controller agent
		// config and bounces if it changes.
		controllerAgentConfigName: controlleragentconfig.Manifold(controlleragentconfig.ManifoldConfig{
			ControllerID:      config.ControllerID,
			Logger:            internallogger.GetLogger("juju.worker.controlleragentconfig"),
			NewSocketListener: controlleragentconfig.NewSocketListener,
			SocketName:        config.ConfigChangeSocketPath,
		}),

		// logRouterReloadBridgeName bridges controller agent config
		// reload notifications onto the dedicated Value watched by the
		// controller-local logrouter.
		logRouterReloadBridgeName: controlleragentconfig.ConfigChangedValueBridgeManifold(
			controlleragentconfig.ConfigChangedValueBridgeManifoldConfig{
				ControllerAgentConfigName: controllerAgentConfigName,
				ConfigChangedValue:        logRouterConfigChanged,
			},
		),

		// The certificate-watcher manifold monitors the API server
		// certificate in the agent config for changes, and parses and
		// offers the result to other manifolds.
		certificateWatcherName: apiservercertwatcher.Manifold(apiservercertwatcher.ManifoldConfig{
			CertReader: config.StartupValueProvider,
		}),

		// The upgrade database gate/flag coordinate workers that should
		// not do anything until the upgrade-database worker has finished
		// running any required database upgrade steps.
		upgradeDatabaseGateName: gate.ManifoldEx(config.UpgradeDBLock),
		upgradeDatabaseFlagName: gate.FlagManifold(gate.FlagManifoldConfig{
			GateName:  upgradeDatabaseGateName,
			NewWorker: gate.NewFlagWorker,
		}),

		// The upgrade-database worker runs soon after the controller
		// agent starts and runs any steps required to upgrade the
		// database to the current version.
		// TODO: remove disabledManifold wrapper once upgrade-database manifold is
		// reworked for standalone controller.
		upgradeDatabaseName: disabledManifold(upgradedatabase.Manifold(upgradedatabase.ManifoldConfig{
			// AgentName:           agentName,
			UpgradeDBGateName:   upgradeDatabaseGateName,
			UpgradeServicesName: upgradeDomainServicesName,
			DBAccessorName:      dbAccessorName,
			NewWorker:           upgradedatabase.NewUpgradeDatabaseWorker,
			Logger:              internallogger.GetLogger("juju.worker.upgradedatabase"),
			Clock:               config.Clock,
			UpgradeSteps:        upgradedatabase.UpgradeSteps,
		})),

		// The upgrade services worker provides domain services for
		// upgrading the controller. This worker MUST never take on a
		// dependency which relies on the database upgrade having been
		// performed.
		upgradeDomainServicesName: ifControllerUpgradeComplete(upgradeservices.Manifold(upgradeservices.ManifoldConfig{
			ChangeStreamName:         changeStreamName,
			Logger:                   internallogger.GetLogger("juju.worker.upgradeservices"),
			NewUpgradeServices:       upgradeservices.NewUpgradeServices,
			NewUpgradeServicesGetter: upgradeservices.NewUpgradeServicesGetter,
			NewWorker:                upgradeservices.NewWorker,
		})),

		// Upgrade steps gate/flag coordinate workers that should not do
		// anything until all upgrade steps have run. The flag of similar
		// name is used to implement the isFullyUpgraded func that keeps
		// upgrade concerns out of unrelated manifolds.
		upgradeStepsGateName: ifControllerUpgradeComplete(gate.ManifoldEx(config.UpgradeStepsLock)),
		upgradeStepsFlagName: ifControllerUpgradeComplete(gate.FlagManifold(gate.FlagManifoldConfig{
			GateName:  upgradeStepsGateName,
			NewWorker: gate.NewFlagWorker,
		})),

		// Upgrade check gate/flag coordinate workers that should not do
		// anything until the upgrader has completed its first check for
		// a new tools version to upgrade to.
		upgradeCheckGateName: ifControllerUpgradeComplete(gate.ManifoldEx(config.UpgradeCheckLock)),
		upgradeCheckFlagName: ifControllerUpgradeComplete(gate.FlagManifold(gate.FlagManifoldConfig{
			GateName:  upgradeCheckGateName,
			NewWorker: gate.NewFlagWorker,
		})),

		// The migration workers collaborate to run migrations and create
		// a mechanism for running other workers so they can't
		// accidentally interfere with a migration in progress.
		migrationFortressName: fortress.Manifold(),

		// migration-inactive-flag is out of scope for Phase 1; migration is never
		// active in the standalone controller, so this unconditionally reports
		// "inactive".
		// TODO: replace with migrationflag.Manifold when controller migration is
		// implemented.
		migrationInactiveFlagName: dependency.Manifold{
			Start: func(_ context.Context, _ dependency.Getter) (worker.Worker, error) {
				return engine.NewStaticFlagWorker(true), nil
			},
			Output: engine.FlagOutput,
		},

		// TODO: remove disabledManifold wrapper once migration-minion manifold is
		// reworked for standalone controller.
		migrationMinionName: ifControllerUpgradeComplete(disabledManifold(migrationminion.Manifold(migrationminion.ManifoldConfig{
			// AgentName:         agentName,
			// APICallerName:     apiCallerName,
			FortressName:          migrationFortressName,
			Clock:                 config.Clock,
			APIOpen:               api.Open,
			ValidateMigration:     config.ValidateMigration,
			NewWorker:             migrationminion.NewWorker,
			Logger:                internallogger.GetLogger("juju.worker.migrationminion", corelogger.MIGRATION),
			SendReport:            migrationminion.SendReport,
			FetchTargetLokiConfig: migrationminion.FetchTargetLokiConfig,
		}))),

		// The primary controller flag manifold will attempt to claim
		// responsibility for running certain workers that must not be
		// run concurrently by multiple controller agents.
		isPrimaryControllerFlagName: singular.Manifold(singular.ManifoldConfig{
			ModelUUID:        config.ControllerModelUUID,
			LeaseManagerName: leaseManagerName,
			Clock:            clock.WallClock,
			Duration:         config.ControllerLeaseDuration,
			Claimant:         config.ControllerAgentTag,
			Entity:           config.ControllerTag,
			NewWorker:        singular.NewFlagWorker,
		}),

		// The logging config updater controls the messages sent via the
		// log sender, according to changes in environment config.
		loggingControllerConfigUpdaterName: ifNotMigrating(controllerlogger.Manifold(controllerlogger.ManifoldConfig{
			DomainServicesName:    domainServicesName,
			LoggerContext:         internallogger.DefaultContext(),
			Logger:                internallogger.GetLogger("juju.worker.logger"),
			LoggingOverrideReader: config.StartupValueProvider,
			UpdateAgentFunc:       config.UpdateLoggerConfig,
		})),

		identityFileWriterName: ifNotMigrating(identityfilewriter.Manifold(identityfilewriter.ManifoldConfig{
			SystemIdentityReader: config.StartupValueProvider,
			NewWorker:            identityfilewriter.NewWorker,
		})),

		externalControllerUpdaterName: ifNotMigrating(ifPrimaryController(externalcontrollerupdater.Manifold(
			externalcontrollerupdater.ManifoldConfig{
				DomainServicesName:                 domainServicesName,
				Clock:                              config.Clock,
				NewExternalControllerWatcherClient: newExternalControllerWatcherClient,
			},
		))),

		traceServicesName: traceservices.Manifold(traceservices.ManifoldConfig{
			ChangeStreamName: changeStreamName,
			Logger:           internallogger.GetLogger("juju.worker.traceservices"),
			NewWorker:        traceservices.NewWorker,
			NewTraceServices: traceservices.NewTraceServices,
		}),

		controllerTraceName: trace.ControllerManifold(trace.ControllerManifoldConfig{
			Tag:               config.ControllerAgentTag,
			TraceServicesName: traceServicesName,
			Clock:             config.Clock,
			Logger:            internallogger.GetLogger("juju.worker.trace"),
			GetTracingService: trace.GetTracingService,
			NewTracerWorker:   trace.NewTracerWorker,
		}),

		httpServerArgsName: ifBootstrapComplete(httpserverargs.Manifold(httpserverargs.ManifoldConfig{
			Clock:                 clock.WallClock,
			DomainServicesName:    domainServicesName,
			NewStateAuthenticator: httpserverargs.NewStateAuthenticator,
		})),

		httpServerName: httpserver.Manifold(httpserver.ManifoldConfig{
			AuthorityName:       certificateWatcherName,
			DomainServicesName:  domainServicesName,
			MuxName:             httpServerArgsName,
			APIServerName:       apiServerName,
			Clock:               config.Clock,
			MuxShutdownWait:     config.MuxShutdownWait,
			Logger:              internallogger.GetLogger("juju.worker.httpserver"),
			GetControllerConfig: httpserver.GetControllerConfig,
			NewTLSConfig:        httpserver.NewTLSConfig,
			NewWorker:           httpserver.NewWorkerShim,
		}),

		// lokiConfigUpdaterName watches the logging domain for
		// Loki config changes and persists them to runtime.conf so the
		// controller logrouter picks up updates on bounce.
		lokiConfigUpdaterName: controllerlokiupdater.Manifold(controllerlokiupdater.ManifoldConfig{
			DomainServicesName:     domainServicesName,
			RuntimeConfigPath:      config.ControllerRuntimePath,
			ConfigChangeSocketPath: config.ConfigChangeSocketPath,
			Logger:                 internallogger.GetLogger("juju.worker.controllerlokiupdater"),
		}),

		// logRouterName is a controller-only logrouter that
		// writes to the local logsink directly in logsink mode, avoiding
		// the cycle: log-router -> api-caller -> api-server -> log-sink ->
		// log-router.
		logRouterName: logrouter.ControllerManifold(logrouter.ControllerManifoldConfig{
			HTTPClientName:       httpClientName,
			LokiConfigProvider:   config.StartupValueProvider,
			AgentConfigChanged:   logRouterConfigChanged,
			Logger:               internallogger.GetLogger("juju.worker.logrouter.controller"),
			Clock:                config.Clock,
			PrometheusRegisterer: config.PrometheusRegisterer,
			LocalLogSink:         config.LogSink,
			NewBackendFunc:       logrouter.NewControllerBackend,
		}),

		// logSinkName is the controller-local log sink that
		// uses the controller-local logrouter.
		logSinkName: logsink.Manifold(logsink.ManifoldConfig{
			AgentTag:       config.ControllerAgentTag,
			LogRouterName:  logRouterName,
			NewWorker:      logsink.NewWorker,
			NewModelLogger: logsink.NewModelLogger,
		}),

		apiServerName: apiserver.Manifold(apiserver.ManifoldConfig{
			AuthenticatorName:      httpServerArgsName,
			Clock:                  clock.WallClock,
			ControllerTag:          config.ControllerAgentTag,
			LocalConfigReader:      config.StartupValueProvider,
			LogSinkName:            logSinkName,
			MuxName:                httpServerArgsName,
			LeaseManagerName:       leaseManagerName,
			UpgradeGateName:        upgradeStepsGateName,
			AuditConfigUpdaterName: auditConfigUpdaterName,
			HTTPClientName:         httpClientName,
			TraceName:              controllerTraceName,
			ObjectStoreName:        objectStoreFacadeName,
			JWTParserName:          jwtParserName,
			WatcherRegistryName:    watcherRegistryName,
			FlightRecorderName:     flightRecorderName,
			ProviderTrackerName:    providerTrackerName,

			// Note that although there is a transient dependency on
			// dbaccessor via changestream, the direct dependency
			// supplies the capability to remove databases corresponding
			// to destroyed/migrated models.
			DomainServicesName: domainServicesName,
			ChangeStreamName:   changeStreamName,

			PrometheusRegisterer:              config.PrometheusRegisterer,
			RegisterIntrospectionHTTPHandlers: config.RegisterIntrospectionHTTPHandlers,
			GetControllerConfigService:        apiserver.GetControllerConfigService,
			GetModelService:                   apiserver.GetModelService,
			NewWorker:                         apiserver.NewWorker,
			NewMetricsCollector:               apiserver.NewMetricsCollector,
		}),

		modelWorkerManagerName: ifFullyUpgraded(modelworkermanager.Manifold(modelworkermanager.ManifoldConfig{
			AuthorityName:                certificateWatcherName,
			LogSinkName:                  logSinkName,
			DomainServicesName:           domainServicesName,
			LeaseManagerName:             leaseManagerName,
			HTTPClientName:               httpClientName,
			APIRemoteCallerGetterName:    apiRemoteRelationCallerName,
			ProviderServiceFactoriesName: providerDomainServicesName,
			NewWorker:                    modelworkermanager.New,
			NewModelWorker:               config.NewModelWorker,
			ModelMetrics:                 config.DependencyEngineMetrics,
			Logger:                       internallogger.GetLogger("juju.workers.modelworkermanager"),
			GetProviderServicesGetter:    modelworkermanager.GetProviderServicesGetter,
			GetControllerConfig:          modelworkermanager.GetControllerConfig,
		})),

		domainServicesName: workerdomainservices.Manifold(workerdomainservices.ManifoldConfig{
			DBAccessorName:              dbAccessorName,
			ChangeStreamName:            changeStreamName,
			ProviderFactoryName:         providerTrackerName,
			ObjectStoreName:             objectStoreFacadeName,
			StorageRegistryName:         storageRegistryName,
			HTTPClientName:              httpClientName,
			LeaseManagerName:            leaseManagerName,
			LogSinkName:                 logSinkName,
			Logger:                      internallogger.GetLogger("juju.worker.services"),
			Clock:                       config.Clock,
			LogDir:                      config.LogDir,
			NewWorker:                   workerdomainservices.NewWorker,
			NewDomainServicesGetter:     workerdomainservices.NewDomainServicesGetter,
			NewControllerDomainServices: workerdomainservices.NewControllerDomainServices,
			NewModelDomainServices:      workerdomainservices.NewProviderTrackerModelDomainServices,
		}),

		providerDomainServicesName: providerservices.Manifold(providerservices.ManifoldConfig{
			ChangeStreamName:          changeStreamName,
			Logger:                    internallogger.GetLogger("juju.worker.providerserivces"),
			NewWorker:                 providerservices.NewWorker,
			NewProviderServicesGetter: providerservices.NewProviderServicesGetter,
			NewProviderServices:       providerservices.NewProviderServices,
		}),

		queryLoggerName: querylogger.Manifold(querylogger.ManifoldConfig{
			LogDir: config.LogDir,
			Logger: internallogger.GetLogger("juju.worker.querylogger"),
		}),

		fileNotifyWatcherName: filenotifywatcher.Manifold(filenotifywatcher.ManifoldConfig{
			Clock:             config.Clock,
			Logger:            internallogger.GetLogger("juju.worker.filenotifywatcher"),
			NewWatcher:        filenotifywatcher.NewWatcher,
			NewINotifyWatcher: filenotifywatcher.NewINotifyWatcher,
		}),

		changeStreamName: changestream.Manifold(changestream.ManifoldConfig{
			ControllerID:         config.ControllerID,
			DBAccessor:           dbAccessorName,
			FileNotifyWatcher:    fileNotifyWatcherName,
			Clock:                config.Clock,
			Logger:               internallogger.GetLogger("juju.worker.changestream"),
			PrometheusRegisterer: config.PrometheusRegisterer,
			NewWatchableDB:       changestream.NewWatchableDB,
			NewMetricsCollector:  changestream.NewMetricsCollector,
		}),

		changeStreamPrunerName: ifPrimaryController(changestreampruner.Manifold(changestreampruner.ManifoldConfig{
			DomainServiceName:      domainServicesName,
			Clock:                  config.Clock,
			Logger:                 internallogger.GetLogger("juju.worker.changestreampruner"),
			NewWorker:              changestreampruner.NewWorker,
			GetChangeStreamService: changestreampruner.GetControllerChangeStreamService,
		})),

		auditConfigUpdaterName: ifDatabaseUpgradeComplete(auditconfigupdater.Manifold(auditconfigupdater.ManifoldConfig{
			LogDir:                     config.LogDir,
			DomainServicesName:         domainServicesName,
			NewWorker:                  auditconfigupdater.NewWorker,
			GetControllerConfigService: auditconfigupdater.GetControllerConfigService,
		})),

		// The lease expiry worker constantly deletes leases with an
		// expiry time in the past.
		leaseExpiryName: ifPrimaryController(leaseexpiry.Manifold(leaseexpiry.ManifoldConfig{
			DBAccessorName: dbAccessorName,
			TraceName:      controllerTraceName,
			Clock:          config.Clock,
			Logger:         internallogger.GetLogger("juju.worker.leaseexpiry"),
			NewWorker:      leaseexpiry.NewWorker,
			NewStore:       leaseexpiry.NewStore,
		})),

		// The global lease manager tracks lease information in the
		// Dqlite database.
		leaseManagerName: leasemanager.Manifold(leasemanager.ManifoldConfig{
			DBAccessorName:       dbAccessorName,
			TraceName:            controllerTraceName,
			ControllerUUID:       config.ControllerUUID,
			ControllerModelUUID:  config.ControllerModelUUID,
			Clock:                config.Clock,
			Logger:               internallogger.GetLogger("juju.worker.lease"),
			LogDir:               config.LogDir,
			PrometheusRegisterer: config.PrometheusRegisterer,
			NewWorker:            leasemanager.NewWorker,
			NewStore:             leasemanager.NewStore,
			NewSecretaryFinder:   internallease.NewSecretaryFinder,
		}),

		secretBackendRotateName: ifNotMigrating(ifPrimaryController(secretbackendrotate.Manifold(
			secretbackendrotate.ManifoldConfig{
				DomainServicesName:      domainServicesName,
				Logger:                  internallogger.GetLogger("juju.worker.secretbackendsrotate"),
				GetSecretBackendService: secretbackendrotate.GetSecretBackendService,
				NewWorker:               secretbackendrotate.NewWorker,
			},
		))),

		// The controlsocket worker runs on the controller.
		controlSocketName: ifDatabaseUpgradeComplete(controlsocket.Manifold(controlsocket.ManifoldConfig{
			DomainServicesName:              domainServicesName,
			ObjectStoreServicesName:         objectStoreServicesName,
			Logger:                          internallogger.GetLogger("juju.worker.controlsocket"),
			NewWorker:                       controlsocket.NewWorker,
			NewSocketListener:               controlsocket.NewSocketListener,
			SocketName:                      config.ControlSocketPath,
			GetControllerDomainServices:     controlsocket.GetControllerDomainServices,
			GetControllerObjectStoreService: controlsocket.GetControllerObjectStoreService,
		})),

		// The ssh server worker runs on the controller.
		sshServerName: sshserver.Manifold(sshserver.ManifoldConfig{
			DomainServicesName:         domainServicesName,
			Logger:                     internallogger.GetLogger("juju.worker.sshserver"),
			NewServerWrapperWorker:     sshserver.NewServerWrapperWorker,
			NewServerWorker:            sshserver.NewServerWorker,
			GetControllerConfigService: sshserver.GetControllerConfigService,
		}),

		// The objectstore draining workers collaborate to run draining
		// of blobs between underlying object stores (s3 compatible).
		// They are used to drain; and to create a mechanism for running
		// other workers so they can't accidentally interfere with a
		// draining in progress.
		objectStoreFortressName: fortress.Manifold(),
		objectStoreDrainerName: objectstoredrainer.Manifold(objectstoredrainer.ManifoldConfig{
			S3ClientName:                    objectStoreS3CallerName,
			ObjectStoreName:                 objectStoreName,
			ObjectStoreServicesName:         objectStoreServicesName,
			RootDirReader:                   config.StartupValueProvider,
			FortressName:                    objectStoreFortressName,
			GetControllerService:            objectstoredrainer.GetControllerService,
			GeObjectStoreServices:           objectstoredrainer.GeObjectStoreServicesGetter,
			GetControllerObjectStoreService: objectstoredrainer.GetControllerObjectStoreService,
			GetDrainingService:              objectstoredrainer.GetDrainingService,
			GetControllerConfigService:      objectstoredrainer.GetControllerConfigService,
			NewHashFileSystemAccessor:       objectstoredrainer.NewHashFileStoreAccessor,
			NewDrainerWorker:                objectstoredrainer.NewDrainWorker,
			SelectFileHash:                  internalobjectstore.SelectFileHash,
			NewWorker:                       objectstoredrainer.NewWorker,
			Clock:                           config.Clock,
			Logger:                          internallogger.GetLogger("juju.worker.objectstoredrainer"),
		}),

		objectStoreName: ifDatabaseUpgradeComplete(objectstore.Manifold(objectstore.ManifoldConfig{
			TraceName:                  controllerTraceName,
			ObjectStoreServicesName:    objectStoreServicesName,
			LeaseManagerName:           leaseManagerName,
			S3ClientName:               objectStoreS3CallerName,
			APIRemoteCallerName:        apiRemoteCallerName,
			RootDirReader:              config.StartupValueProvider,
			ControllerNodeID:           config.ControllerID,
			Clock:                      config.Clock,
			Logger:                     internallogger.GetLogger("juju.worker.objectstore"),
			NewObjectStoreWorker:       internalobjectstore.ObjectStoreFactory,
			GetControllerConfigService: objectstore.GetControllerConfigService,
			GetMetadataService:         objectstore.GetMetadataService,
			GetObjectStoreService:      objectstore.GetObjectStoreService,
			IsBootstrapController:      internalbootstrap.IsBootstrapController,
		})),

		// The objectstore facade is a thin wrapper around the objectstore
		// worker. It guards against any objectstore operations while the
		// draining is in progress.
		objectStoreFacadeName: objectstorefacade.Manifold(objectstorefacade.ManifoldConfig{
			ObjectStoreName: objectStoreName,
			FortressName:    objectStoreFortressName,
			NewWorker:       objectstorefacade.NewWorker,
			Logger:          internallogger.GetLogger("juju.worker.objectstorefacade"),
		}),

		objectStoreServicesName: objectstoreservices.Manifold(objectstoreservices.ManifoldConfig{
			ChangeStreamName:             changeStreamName,
			Clock:                        config.Clock,
			Logger:                       internallogger.GetLogger("juju.worker.objectstoreservices"),
			NewWorker:                    objectstoreservices.NewWorker,
			NewObjectStoreServices:       objectstoreservices.NewObjectStoreServices,
			NewObjectStoreServicesGetter: objectstoreservices.NewObjectStoreServicesGetter,
		}),

		objectStoreS3CallerName: ifDatabaseUpgradeComplete(objectstores3caller.Manifold(objectstores3caller.ManifoldConfig{
			HTTPClientName:          httpClientName,
			ObjectStoreServicesName: objectStoreServicesName,
			NewClient:               objectstores3caller.NewS3Client,
			Logger:                  internallogger.GetLogger("juju.worker.s3caller"),
			GetObjectStoreService:   objectstores3caller.GetObjectStoreService,
			NewWorker:               objectstores3caller.NewWorker,
		})),

		// Provider tracker manifold is not dependent on the
		// ifDatabaseUpgradeComplete gate. The provider tracker data must not
		// change between patch/build versions and should be available to all
		// workers from the start. This includes the controller and read-only
		// model data that the provider tracker worker is responsible for.
		//
		// Migration away to a major/minor version is the correct way to move
		// a model for upgrade scenarios.
		providerTrackerName: providertracker.MultiTrackerManifold(providertracker.ManifoldConfig{
			ProviderServiceFactoriesName: providerDomainServicesName,
			LogSinkName:                  logSinkName,
			NewWorker:                    providertracker.NewWorker,
			NewTrackerWorker:             providertracker.NewTrackerWorker,
			NewEphemeralProvider:         providertracker.NewEphemeralProvider,
			GetProviderServicesGetter:    providertracker.GetProviderServicesGetter,
			GetIAASProvider: providertracker.IAASGetProvider(func(ctx context.Context, args environs.OpenParams, invalidator environs.CredentialInvalidator) (environs.Environ, error) {
				return config.NewEnvironFunc(ctx, args, invalidator)
			}),
			// GetCAASProvider uses caas.New directly because the controller
			// ManifoldsConfig does not currently accept a CAAS broker
			// constructor, but providertracker still validates that both
			// providers are non-nil.
			GetCAASProvider: providertracker.CAASGetProvider(caas.New),
			Logger:          internallogger.GetLogger("juju.worker.providertracker"),
			Clock:           config.Clock,
		}),

		storageRegistryName: storageregistry.Manifold(storageregistry.ManifoldConfig{
			ProviderFactoryName:      providerTrackerName,
			NewStorageRegistryWorker: storageregistry.NewTrackedWorker,
			Clock:                    config.Clock,
			Logger:                   internallogger.GetLogger("juju.worker.storageregistry"),
		}),

		httpClientName: httpclient.Manifold(httpclient.ManifoldConfig{
			NewHTTPClient: func(namespace corehttp.Purpose, opts ...internalhttp.Option) *internalhttp.Client {
				switch namespace {
				case corehttp.CharmhubPurpose:
					logger := internallogger.GetLogger("juju.charmhub", corelogger.CHARMHUB)
					opts = append(
						opts,
						internalhttp.WithLogger(logger),
						internalhttp.WithRequestRetrier(charmhub.DefaultRetryPolicy()),
					)

				case corehttp.S3Purpose:
					logger := internallogger.GetLogger("juju.objectstore.s3", corelogger.OBJECTSTORE)
					opts = append(opts, internalhttp.WithLogger(logger))

				case corehttp.SSHImporterPurpose:
					logger := internallogger.GetLogger("juju.ssh.importer", corelogger.SSHIMPORTER)
					opts = append(opts, internalhttp.WithLogger(logger))

				case corehttp.MacaroonPurpose:
					logger := internallogger.GetLogger("juju.macaroon", corelogger.MACAROON)
					opts = append(opts, internalhttp.WithLogger(logger))

				case corehttp.SimpleStreamPurpose:
					logger := internallogger.GetLogger("juju.simplestream", corelogger.SIMPLESTREAM)
					opts = append(opts, internalhttp.WithLogger(logger))
				}

				return internalhttp.NewClient(opts...)
			},
			NewHTTPClientWorker:  httpclient.NewTrackedWorker,
			PrometheusRegisterer: config.PrometheusRegisterer,
			NewMetricsCollector:  httpclient.NewMetricsCollector,
			Clock:                config.Clock,
			Logger:               internallogger.GetLogger("juju.worker.httpclient"),
		}),

		apiRemoteCallerName: apiremotecaller.Manifold(apiremotecaller.ManifoldConfig{
			ObjectStoreServicesName: objectStoreServicesName,
			APIInfo:                 config.StartupValueProvider,
			Origin:                  config.ControllerAgentTag,
			Clock:                   config.Clock,
			Logger:                  internallogger.GetLogger("juju.worker.apiremotecaller"),
			NewWorker:               apiremotecaller.NewWorker,
		}),

		controllerPresenceName: controllerpresence.Manifold(controllerpresence.ManifoldConfig{
			APIRemoteCallerName:         apiRemoteCallerName,
			DomainServicesName:          domainServicesName,
			GetDomainServices:           controllerpresence.GetDomainServices,
			GetControllerDomainServices: controllerpresence.GetControllerDomainServices,
			NewWorker:                   controllerpresence.NewWorker,
			Logger:                      internallogger.GetLogger("juju.worker.controllerpresence"),
			Clock:                       config.Clock,
		}),

		apiRemoteRelationCallerName: ifControllerUpgradeComplete(apiremoterelationcaller.Manifold(apiremoterelationcaller.ManifoldConfig{
			DomainServicesName:          domainServicesName,
			NewWorker:                   apiremoterelationcaller.NewWorker,
			NewAPIInfoGetter:            apiremoterelationcaller.NewAPIInfoGetter,
			NewConnectionGetter:         apiremoterelationcaller.NewConnectionGetter,
			GetDomainServicesGetterFunc: apiremoterelationcaller.GetDomainServicesGetter,
			Logger:                      internallogger.GetLogger("juju.worker.apiremoterelationcaller"),
			Clock:                       config.Clock,
		})),

		jwtParserName: jwtparser.Manifold(jwtparser.ManifoldConfig{
			GetControllerConfigService: jwtparser.GetControllerConfigService,
			DomainServicesName:         domainServicesName,
		}),

		apiAddressSetterName: ifPrimaryController(apiaddresssetter.Manifold(apiaddresssetter.ManifoldConfig{
			DomainServicesName:          domainServicesName,
			GetDomainServices:           apiaddresssetter.GetDomainServices,
			GetControllerDomainServices: apiaddresssetter.GetControllerDomainServices,
			NewWorker:                   apiaddresssetter.New,
			Logger:                      internallogger.GetLogger("juju.worker.apiaddresssetter"),
		})),

		undertakerName: undertaker.Manifold(undertaker.ManifoldConfig{
			DBAccessorName:            dbAccessorName,
			DomainServicesName:        domainServicesName,
			NewWorker:                 undertaker.NewWorker,
			GetControllerModelService: undertaker.GetControllerModelService,
			GetRemovalServiceGetter:   undertaker.GetRemovalServiceGetter,
			Logger:                    internallogger.GetLogger("juju.worker.undertaker"),
			Clock:                     config.Clock,
		}),

		watcherRegistryName: watcherregistry.Manifold(watcherregistry.ManifoldConfig{
			NewWorker: watcherregistry.NewWorker,
			Clock:     config.Clock,
			Logger:    internallogger.GetLogger("juju.worker.watcherregistry"),
		}),
	}
}

// IAASManifolds returns the IAAS-specific controller manifolds merged with the
// shared controller manifolds.
func IAASManifolds(config ManifoldsConfig) dependency.Manifolds {
	return mergeManifolds(config, dependency.Manifolds{
		// Bootstrap worker is responsible for setting up the initial
		// controller.
		bootstrapName: ifDatabaseUpgradeComplete(bootstrap.Manifold(NewIAASBootstrapManifoldConfig(config))),

		certificateUpdaterName: ifFullyUpgraded(certupdater.Manifold(certupdater.ManifoldConfig{
			AuthorityName:               certificateWatcherName,
			DomainServicesName:          domainServicesName,
			GetControllerDomainServices: certupdater.GetControllerDomainServices,
			NewWorker:                   certupdater.NewCertificateUpdater,
			Logger:                      internallogger.GetLogger("juju.worker.certupdater"),
		})),

		// DBAccessor provides access to the Dqlite database. The
		// controller currently uses the IAAS node manager.
		dbAccessorName: dbaccessor.Manifold(NewIAASDBAccessorManifoldConfig(config)),

		// The upgrader is a leaf worker that returns a specific error
		// type recognised by the controller agent, causing other workers
		// to be stopped and the agent to be restarted running the new
		// tools.
		// TODO: remove disabledManifold wrapper once upgrader manifold is reworked
		// for standalone controller.
		upgraderName: ifControllerUpgradeComplete(disabledManifold(upgrader.Manifold(upgrader.ManifoldConfig{
			// AgentName:            agentName,
			// APICallerName:        apiCallerName,
			UpgradeStepsGateName: upgradeStepsGateName,
			UpgradeCheckGateName: upgradeCheckGateName,
			PreviousAgentVersion: config.PreviousAgentVersion,
			Logger:               internallogger.GetLogger("juju.worker.upgrader"),
			Clock:                config.Clock,
		}))),

		// The upgradestepscontroller worker runs soon after the
		// controller agent starts and runs any steps required to
		// upgrade to the running jujud version. Once upgrade steps have
		// run, the upgradesteps gate is unlocked and the worker exits.
		// TODO: remove disabledManifold wrapper once upgrade-steps controller
		// manifold is reworked for standalone controller.
		upgradeControllerStepsName: ifControllerUpgradeComplete(disabledManifold(upgradestepscontroller.Manifold(upgradestepscontroller.ManifoldConfig{
			// AgentName:            agentName,
			// APICallerName:        apiCallerName,
			DomainServicesName:   domainServicesName,
			UpgradeStepsGateName: upgradeStepsGateName,
			PreUpgradeSteps:      config.PreUpgradeSteps(model.IAAS),
			UpgradeSteps:         config.UpgradeSteps,
			NewAgentStatusSetter: config.NewAgentStatusSetter,
			NewControllerWorker:  upgradestepscontroller.NewControllerWorker,
			GetUpgradeService:    upgradestepscontroller.GetUpgradeService,
			Logger:               internallogger.GetLogger("juju.worker.upgradestepscontroller"),
			Clock:                config.Clock,
		}))),
	})
}

// CAASManifolds returns the CAAS-specific controller manifolds merged with the
// shared controller manifolds.
func CAASManifolds(config ManifoldsConfig) dependency.Manifolds {
	return mergeManifolds(config, dependency.Manifolds{
		bootstrapName: ifDatabaseUpgradeComplete(bootstrap.Manifold(NewCAASBootstrapManifoldConfig(config))),

		certificateUpdaterName: ifFullyUpgraded(certupdater.Manifold(certupdater.ManifoldConfig{
			AuthorityName:               certificateWatcherName,
			DomainServicesName:          domainServicesName,
			GetControllerDomainServices: certupdater.GetControllerDomainServices,
			NewWorker:                   certupdater.NewCertificateUpdater,
			Logger:                      internallogger.GetLogger("juju.worker.certupdater"),
		})),

		dbAccessorName: dbaccessor.Manifold(NewCAASDBAccessorManifoldConfig(config)),

		// TODO: remove disabledManifold wrapper once upgrader manifold is reworked
		// for standalone controller.
		upgraderName: ifControllerUpgradeComplete(disabledManifold(upgrader.Manifold(upgrader.ManifoldConfig{
			// AgentName:            agentName,
			// APICallerName:        apiCallerName,
			UpgradeStepsGateName: upgradeStepsGateName,
			UpgradeCheckGateName: upgradeCheckGateName,
			PreviousAgentVersion: config.PreviousAgentVersion,
			Logger:               internallogger.GetLogger("juju.worker.upgrader"),
			Clock:                config.Clock,
		}))),
		// TODO: remove disabledManifold wrapper once upgrade-steps controller
		// manifold is reworked for standalone controller.
		upgradeControllerStepsName: ifControllerUpgradeComplete(disabledManifold(upgradestepscontroller.Manifold(upgradestepscontroller.ManifoldConfig{
			// AgentName:            agentName,
			// APICallerName:        apiCallerName,
			DomainServicesName:   domainServicesName,
			UpgradeStepsGateName: upgradeStepsGateName,
			PreUpgradeSteps:      config.PreUpgradeSteps(model.IAAS),
			UpgradeSteps:         config.UpgradeSteps,
			NewAgentStatusSetter: config.NewAgentStatusSetter,
			NewControllerWorker:  upgradestepscontroller.NewControllerWorker,
			GetUpgradeService:    upgradestepscontroller.GetUpgradeService,
			Logger:               internallogger.GetLogger("juju.worker.upgradestepscontroller"),
			Clock:                config.Clock,
		}))),
	})
}

// NewIAASBootstrapManifoldConfig returns the IAAS-specific bootstrap config.
func NewIAASBootstrapManifoldConfig(config ManifoldsConfig) bootstrap.ManifoldConfig {
	return bootstrap.ManifoldConfig{
		ObjectStoreName:              objectStoreFacadeName,
		DomainServicesName:           domainServicesName,
		HTTPClientName:               httpClientName,
		BootstrapGateName:            isBootstrapGateName,
		ProviderFactoryName:          providerTrackerName,
		DataDir:                      config.DataDir,
		APIPort:                      config.APIPort,
		AgentPassword:                config.AgentPassword,
		RequiresBootstrap:            bootstrap.RequiresBootstrap,
		PopulateControllerCharm:      bootstrap.PopulateIAASControllerCharm,
		StatusHistory:                domain.NewStatusHistory(internallogger.GetLogger("juju.services"), config.Clock),
		Logger:                       internallogger.GetLogger("juju.worker.bootstrap"),
		Clock:                        config.Clock,
		AgentBinaryUploader:          bootstrap.IAASAgentBinaryUploader,
		ControllerCharmDeployer:      bootstrap.IAASControllerCharmUploader,
		ControllerUnitPassword:       bootstrap.IAASControllerUnitPassword,
		BootstrapAddressFinderGetter: bootstrap.IAASAddressFinder,
		AgentFinalizer:               bootstrap.IAASAgentFinalizer,
	}
}

// NewCAASBootstrapManifoldConfig returns the CAAS-specific bootstrap config.
func NewCAASBootstrapManifoldConfig(config ManifoldsConfig) bootstrap.ManifoldConfig {
	return bootstrap.ManifoldConfig{
		ObjectStoreName:              objectStoreFacadeName,
		DomainServicesName:           domainServicesName,
		HTTPClientName:               httpClientName,
		BootstrapGateName:            isBootstrapGateName,
		ProviderFactoryName:          providerTrackerName,
		DataDir:                      config.DataDir,
		APIPort:                      config.APIPort,
		AgentPassword:                config.AgentPassword,
		RequiresBootstrap:            bootstrap.RequiresBootstrap,
		PopulateControllerCharm:      bootstrap.PopulateCAASControllerCharm,
		StatusHistory:                domain.NewStatusHistory(internallogger.GetLogger("juju.services"), config.Clock),
		Logger:                       internallogger.GetLogger("juju.worker.bootstrap"),
		Clock:                        config.Clock,
		AgentBinaryUploader:          bootstrap.CAASAgentBinaryUploader,
		ControllerCharmDeployer:      bootstrap.CAASControllerCharmUploader,
		ControllerUnitPassword:       bootstrap.CAASControllerUnitPassword,
		BootstrapAddressFinderGetter: bootstrap.CAASAddressFinder,
		AgentFinalizer:               bootstrap.CAASAgentFinalizer,
	}
}

// NewIAASDBAccessorManifoldConfig returns the IAAS-specific db-accessor config.
func NewIAASDBAccessorManifoldConfig(config ManifoldsConfig) dbaccessor.ManifoldConfig {
	return dbaccessor.ManifoldConfig{
		QueryLoggerName:           queryLoggerName,
		ControllerAgentConfigName: controllerAgentConfigName,
		ControllerStartupValues:   config.StartupValueProvider,
		Logger:                    internallogger.GetLogger("juju.worker.dbaccessor"),
		PrometheusRegisterer:      config.PrometheusRegisterer,
		NewApp:                    dbaccessor.NewApp,
		NewDBWorker:               config.NewDBWorkerFunc,
		NewMetricsCollector:       dbaccessor.NewMetricsCollector,
		NewNodeManager:            dbaccessor.IAASNodeManager,
	}
}

// NewCAASDBAccessorManifoldConfig returns the CAAS-specific db-accessor config.
func NewCAASDBAccessorManifoldConfig(config ManifoldsConfig) dbaccessor.ManifoldConfig {
	return dbaccessor.ManifoldConfig{
		QueryLoggerName:           queryLoggerName,
		ControllerAgentConfigName: controllerAgentConfigName,
		ControllerStartupValues:   config.StartupValueProvider,
		Logger:                    internallogger.GetLogger("juju.worker.dbaccessor"),
		PrometheusRegisterer:      config.PrometheusRegisterer,
		NewApp:                    dbaccessor.NewApp,
		NewDBWorker:               config.NewDBWorkerFunc,
		NewMetricsCollector:       dbaccessor.NewMetricsCollector,
		NewNodeManager:            dbaccessor.CAASNodeManager,
	}
}

func mergeManifolds(config ManifoldsConfig, manifolds dependency.Manifolds) dependency.Manifolds {
	result := commonManifolds(config)
	maps.Copy(result, manifolds)
	return result
}

// disabledManifold wraps an existing manifold definition so that its call site
// and configuration are preserved in the source while the manifold itself is
// replaced with a no-op worker. This keeps the wiring visible and makes it
// easy to re-enable manifolds once their dependencies (e.g. agent, api-caller)
// are reworked in a later phase of the standalone controller project. Remove
// disabledManifold and unwrap the call sites when each manifold is brought
// back in scope.
func disabledManifold(_ dependency.Manifold) dependency.Manifold {
	return dependency.Manifold{
		Start: func(_ context.Context, _ dependency.Getter) (worker.Worker, error) {
			return jworker.NewSimpleWorker(func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			}), nil
		},
	}
}

// ifBootstrapComplete gates against the bootstrap worker completing.
// This ensures that all blobs (agent binaries and controller charm) are
// available before the controller agent starts.
var ifBootstrapComplete = engine.Housing{
	Flags: []string{
		isBootstrapFlagName,
	},
}.Decorate

var ifFullyUpgraded = engine.Housing{
	Flags: []string{
		upgradeStepsFlagName,
		upgradeCheckFlagName,
	},
}.Decorate

var ifControllerUpgradeComplete = engine.Housing{
	Flags: []string{
		controllerUpgradeFlagName,
	},
}.Decorate

var ifNotMigrating = engine.Housing{
	Flags: []string{
		migrationInactiveFlagName,
	},
	Occupy: migrationFortressName,
}.Decorate

var ifPrimaryController = engine.Housing{
	Flags: []string{
		isPrimaryControllerFlagName,
	},
}.Decorate

var ifDatabaseUpgradeComplete = engine.Housing{
	Flags: []string{
		upgradeDatabaseFlagName,
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
	controllerlogger.LoggingOverrideReader
	identityfilewriter.SystemIdentityReader
	logrouter.LokiConfigProvider
}

const (
	terminationName    = "termination-signal-handler"
	flightRecorderName = "flight-recorder"

	bootstrapName       = "bootstrap"
	isBootstrapGateName = "is-bootstrap-gate"
	isBootstrapFlagName = "is-bootstrap-flag"

	controllerUpgradeGateName = "controller-upgrade-gate"
	controllerUpgradeFlagName = "controller-upgrade-flag"

	upgradeDatabaseName     = "upgrade-database-runner"
	upgradeDatabaseGateName = "upgrade-database-gate"
	upgradeDatabaseFlagName = "upgrade-database-flag"

	upgraderName               = "upgrader"
	upgradeControllerStepsName = "upgrade-controller-steps-runner"
	upgradeStepsGateName       = "upgrade-steps-gate"
	upgradeStepsFlagName       = "upgrade-steps-flag"
	upgradeCheckGateName       = "upgrade-check-gate"
	upgradeCheckFlagName       = "upgrade-check-flag"
	upgradeDomainServicesName  = "upgrade-services"

	migrationFortressName     = "migration-fortress"
	migrationInactiveFlagName = "migration-inactive-flag"
	migrationMinionName       = "migration-minion"

	apiAddressSetterName               = "api-address-setter"
	apiServerName                      = "api-server"
	apiRemoteCallerName                = "api-remote-caller"
	apiRemoteRelationCallerName        = "api-remote-relation-caller"
	auditConfigUpdaterName             = "audit-config-updater"
	certificateUpdaterName             = "certificate-updater"
	certificateWatcherName             = "certificate-watcher"
	changeStreamName                   = "change-stream"
	changeStreamPrunerName             = "change-stream-pruner"
	controllerAgentConfigName          = "controller-agent-config"
	controllerPresenceName             = "controller-presence"
	controlSocketName                  = "control-socket"
	dbAccessorName                     = "db-accessor"
	domainServicesName                 = "domain-services"
	externalControllerUpdaterName      = "external-controller-updater"
	fileNotifyWatcherName              = "file-notify-watcher"
	httpClientName                     = "http-client"
	httpServerArgsName                 = "http-server-args"
	httpServerName                     = "http-server"
	identityFileWriterName             = "ssh-identity-writer"
	isPrimaryControllerFlagName        = "is-primary-controller-flag"
	jwtParserName                      = "jwt-parser"
	leaseExpiryName                    = "lease-expiry"
	leaseManagerName                   = "lease-manager"
	loggingControllerConfigUpdaterName = "logging-controller-config-updater"
	logRouterName                      = "log-router"
	logRouterReloadBridgeName          = "log-router-reload-bridge"
	logSinkName                        = "log-sink"
	lokiConfigUpdaterName              = "loki-config-updater"
	modelWorkerManagerName             = "model-worker-manager"
	objectStoreName                    = "object-store"
	objectStoreS3CallerName            = "object-store-s3-caller"
	objectStoreServicesName            = "object-store-services"
	objectStoreFortressName            = "object-store-fortress"
	objectStoreFacadeName              = "object-store-facade"
	objectStoreDrainerName             = "object-store-drainer"
	providerDomainServicesName         = "provider-services"
	providerTrackerName                = "provider-tracker"
	queryLoggerName                    = "query-logger"
	secretBackendRotateName            = "secret-backend-rotate"
	sshServerName                      = "ssh-server"
	storageRegistryName                = "storage-registry"
	controllerTraceName                = "controller-trace"
	traceServicesName                  = "trace-services"
	undertakerName                     = "undertaker"
	watcherRegistryName                = "watcher-registry"
)
