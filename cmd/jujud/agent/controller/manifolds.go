// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package controller provides the manifolds for the jujud controller.
package controller

import (
	"context"
	"net/http"
	"path"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	"github.com/prometheus/client_golang/prometheus"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller/crosscontroller"
	"github.com/juju/juju/api/macaroon"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/flightrecorder"
	corehttp "github.com/juju/juju/core/http"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	coretrace "github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/environs"
	internalbootstrap "github.com/juju/juju/internal/bootstrap"
	"github.com/juju/juju/internal/charmhub"
	internalhttp "github.com/juju/juju/internal/http"
	internallease "github.com/juju/juju/internal/lease"
	internallogger "github.com/juju/juju/internal/logger"
	internalobjectstore "github.com/juju/juju/internal/objectstore"
	"github.com/juju/juju/internal/s3client"
	"github.com/juju/juju/internal/simplestreams"
	sshimporter "github.com/juju/juju/internal/ssh/importer"
	"github.com/juju/juju/internal/upgrades"
	"github.com/juju/juju/internal/upgradesteps"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/agent"
	"github.com/juju/juju/internal/worker/agentconfigupdater"
	"github.com/juju/juju/internal/worker/apiaddresssetter"
	"github.com/juju/juju/internal/worker/apicaller"
	"github.com/juju/juju/internal/worker/apiconfigwatcher"
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
	"github.com/juju/juju/internal/worker/logger"
	"github.com/juju/juju/internal/worker/logsink"
	"github.com/juju/juju/internal/worker/migrationflag"
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

	// Agent contains the agent that will be wrapped and made available
	// to its dependencies via a dependency.Engine.
	Agent coreagent.Agent

	// AgentConfigChanged is set whenever the controller agent's config
	// is updated.
	AgentConfigChanged *voyeur.Value

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

	// TransactionPruneInterval defines how frequently mgo/txn
	// transactions are pruned from the database.
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

// Manifolds returns a set of co-configured manifolds covering
// the various responsibilities of the controller agent.
func Manifolds(config ManifoldsConfig) dependency.Manifolds {
	// connectFilter exists:
	//  1) to let us retry api connections immediately on password change,
	//     rather than causing the dependency engine to wait for a while;
	//  2) to decide how to deal with fatal, non-recoverable errors
	//     e.g. apicaller.ErrConnectImpossible.
	connectFilter := func(err error) error {
		cause := errors.Cause(err)
		if errors.Is(cause, apicaller.ErrConnectImpossible) {
			return jworker.ErrTerminateAgent
		} else if errors.Is(cause, apicaller.ErrChangedPassword) {
			return dependency.ErrBounce
		}
		return err
	}

	newExternalControllerWatcherClient := func(ctx context.Context, apiInfo *api.Info) (
		externalcontrollerupdater.ExternalControllerWatcherClientCloser, string, error,
	) {
		conn, err := apicaller.NewExternalControllerConnection(ctx, apiInfo)
		if err != nil {
			return nil, "", errors.Trace(err)
		}
		return crosscontroller.NewClient(conn), conn.IPAddr(), nil
	}

	agentConfig := config.Agent.CurrentConfig()
	agentTag := agentConfig.Tag()
	controllerTag := agentConfig.Controller()

	return dependency.Manifolds{
		// The agent manifold references the enclosing agent, and is the
		// foundation stone on which most other manifolds ultimately
		// depend.
		agentName: agent.Manifold(config.Agent),

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

		// clock is retained because several manifolds (http-server-args,
		// api-server, lease-expiry, lease-manager) reference it by name.
		clockName: clockManifold(config.Clock),

		flightRecorderName: workerflightrecorder.Manifold(config.FlightRecorder),

		// Controller agent config manifold watches the controller agent
		// config and bounces if it changes.
		controllerAgentConfigName: controlleragentconfig.Manifold(controlleragentconfig.ManifoldConfig{
			AgentName:         agentName,
			Clock:             config.Clock,
			Logger:            internallogger.GetLogger("juju.worker.controlleragentconfig"),
			NewSocketListener: controlleragentconfig.NewSocketListener,
			SocketName:        path.Join(agentConfig.DataDir(), "configchange.socket"),
		}),

		// The api-config-watcher manifold monitors the API server
		// addresses in the agent config and bounces when they change.
		// It's required as part of model migrations.
		apiConfigWatcherName: apiconfigwatcher.Manifold(apiconfigwatcher.ManifoldConfig{
			AgentName:          agentName,
			AgentConfigChanged: config.AgentConfigChanged,
			Logger:             internallogger.GetLogger("juju.worker.apiconfigwatcher"),
		}),

		// The certificate-watcher manifold monitors the API server
		// certificate in the agent config for changes, and parses and
		// offers the result to other manifolds.
		certificateWatcherName: apiservercertwatcher.Manifold(apiservercertwatcher.ManifoldConfig{
			AgentName: agentName,
		}),

		// The api caller is a thin concurrent wrapper around a connection
		// to some API server. It's used by many other manifolds, which
		// all select their own desired facades.
		apiCallerName: apicaller.Manifold(apicaller.ManifoldConfig{
			AgentName:            agentName,
			APIConfigWatcherName: apiConfigWatcherName,
			APIOpen:              api.Open,
			NewConnection:        apicaller.ScaryConnect,
			Filter:               connectFilter,
			Logger:               internallogger.GetLogger("juju.worker.apicaller"),
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
		upgradeDatabaseName: upgradedatabase.Manifold(upgradedatabase.ManifoldConfig{
			AgentName:           agentName,
			UpgradeDBGateName:   upgradeDatabaseGateName,
			UpgradeServicesName: upgradeDomainServicesName,
			DBAccessorName:      dbAccessorName,
			NewWorker:           upgradedatabase.NewUpgradeDatabaseWorker,
			Logger:              internallogger.GetLogger("juju.worker.upgradedatabase"),
			Clock:               config.Clock,
			UpgradeSteps:        upgradedatabase.UpgradeSteps,
		}),

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
		migrationInactiveFlagName: migrationflag.Manifold(migrationflag.ManifoldConfig{
			APICallerName: apiCallerName,
			Check:         migrationflag.IsTerminal,
			NewFacade:     migrationflag.NewFacade,
			NewWorker:     migrationflag.NewWorker,
		}),
		migrationMinionName: ifControllerUpgradeComplete(migrationminion.Manifold(migrationminion.ManifoldConfig{
			AgentName:         agentName,
			APICallerName:     apiCallerName,
			FortressName:      migrationFortressName,
			Clock:             config.Clock,
			APIOpen:           api.Open,
			ValidateMigration: config.ValidateMigration,
			NewFacade:         migrationminion.NewFacade,
			NewWorker:         migrationminion.NewWorker,
			Logger:            internallogger.GetLogger("juju.worker.migrationminion", corelogger.MIGRATION),
		})),

		// The primary controller flag manifold will attempt to claim
		// responsibility for running certain workers that must not be
		// run concurrently by multiple controller agents.
		isPrimaryControllerFlagName: singular.Manifold(singular.ManifoldConfig{
			AgentName:        agentName,
			LeaseManagerName: leaseManagerName,
			Clock:            config.Clock,
			Duration:         config.ControllerLeaseDuration,
			Claimant:         agentTag,
			Entity:           controllerTag,
			NewWorker:        singular.NewFlagWorker,
		}),

		// The logging config updater controls the messages sent via the
		// log sender, according to changes in environment config.
		loggingConfigUpdaterName: ifNotMigrating(logger.Manifold(logger.ManifoldConfig{
			AgentName:       agentName,
			APICallerName:   apiCallerName,
			LoggerContext:   internallogger.DefaultContext(),
			Logger:          internallogger.GetLogger("juju.worker.logger"),
			UpdateAgentFunc: config.UpdateLoggerConfig,
		})),

		identityFileWriterName: ifNotMigrating(identityfilewriter.Manifold(identityfilewriter.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
		})),

		externalControllerUpdaterName: ifNotMigrating(ifPrimaryController(externalcontrollerupdater.Manifold(
			externalcontrollerupdater.ManifoldConfig{
				APICallerName:                      apiCallerName,
				NewExternalControllerWatcherClient: newExternalControllerWatcherClient,
			},
		))),

		traceName: trace.Manifold(trace.ManifoldConfig{
			AgentName:       agentName,
			Clock:           config.Clock,
			Logger:          internallogger.GetLogger("juju.worker.trace"),
			NewTracerWorker: trace.NewTracerWorker,
			Kind:            coretrace.KindController,
		}),

		httpServerArgsName: ifBootstrapComplete(httpserverargs.Manifold(httpserverargs.ManifoldConfig{
			ClockName:             clockName,
			DomainServicesName:    domainServicesName,
			NewStateAuthenticator: httpserverargs.NewStateAuthenticator,
		})),

		httpServerName: httpserver.Manifold(httpserver.ManifoldConfig{
			AuthorityName:       certificateWatcherName,
			DomainServicesName:  domainServicesName,
			MuxName:             httpServerArgsName,
			APIServerName:       apiServerName,
			AgentName:           config.AgentName,
			Clock:               config.Clock,
			MuxShutdownWait:     config.MuxShutdownWait,
			LogDir:              agentConfig.LogDir(),
			Logger:              internallogger.GetLogger("juju.worker.httpserver"),
			GetControllerConfig: httpserver.GetControllerConfig,
			NewTLSConfig:        httpserver.NewTLSConfig,
			NewWorker:           httpserver.NewWorkerShim,
		}),

		logSinkName: logsink.Manifold(logsink.ManifoldConfig{
			AgentTag:       agentTag,
			Clock:          config.Clock,
			NewWorker:      logsink.NewWorker,
			NewModelLogger: logsink.NewModelLogger,
			LogSink:        config.LogSink,
		}),

		apiServerName: apiserver.Manifold(apiserver.ManifoldConfig{
			AgentName:              agentName,
			AuthenticatorName:      httpServerArgsName,
			ClockName:              clockName,
			LogSinkName:            logSinkName,
			MuxName:                httpServerArgsName,
			LeaseManagerName:       leaseManagerName,
			UpgradeGateName:        upgradeStepsGateName,
			AuditConfigUpdaterName: auditConfigUpdaterName,
			HTTPClientName:         httpClientName,
			TraceName:              traceName,
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
			LogDir:                      agentConfig.LogDir(),
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
			LogDir: agentConfig.LogDir(),
			Clock:  config.Clock,
			Logger: internallogger.GetLogger("juju.worker.querylogger"),
		}),

		fileNotifyWatcherName: filenotifywatcher.Manifold(filenotifywatcher.ManifoldConfig{
			Clock:             config.Clock,
			Logger:            internallogger.GetLogger("juju.worker.filenotifywatcher"),
			NewWatcher:        filenotifywatcher.NewWatcher,
			NewINotifyWatcher: filenotifywatcher.NewINotifyWatcher,
		}),

		changeStreamName: changestream.Manifold(changestream.ManifoldConfig{
			AgentName:            agentName,
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
			AgentName:                  agentName,
			DomainServicesName:         domainServicesName,
			NewWorker:                  auditconfigupdater.NewWorker,
			GetControllerConfigService: auditconfigupdater.GetControllerConfigService,
		})),

		// The lease expiry worker constantly deletes leases with an
		// expiry time in the past.
		leaseExpiryName: ifPrimaryController(leaseexpiry.Manifold(leaseexpiry.ManifoldConfig{
			ClockName:      clockName,
			DBAccessorName: dbAccessorName,
			TraceName:      traceName,
			Logger:         internallogger.GetLogger("juju.worker.leaseexpiry"),
			NewWorker:      leaseexpiry.NewWorker,
			NewStore:       leaseexpiry.NewStore,
		})),

		// The global lease manager tracks lease information in the
		// Dqlite database.
		leaseManagerName: leasemanager.Manifold(leasemanager.ManifoldConfig{
			AgentName:            agentName,
			ClockName:            clockName,
			DBAccessorName:       dbAccessorName,
			TraceName:            traceName,
			Logger:               internallogger.GetLogger("juju.worker.lease"),
			LogDir:               agentConfig.LogDir(),
			PrometheusRegisterer: config.PrometheusRegisterer,
			NewWorker:            leasemanager.NewWorker,
			NewStore:             leasemanager.NewStore,
			NewSecretaryFinder:   internallease.NewSecretaryFinder,
		}),

		secretBackendRotateName: ifNotMigrating(ifPrimaryController(secretbackendrotate.Manifold(
			secretbackendrotate.ManifoldConfig{
				APICallerName: apiCallerName,
				Logger:        internallogger.GetLogger("juju.worker.secretbackendsrotate"),
			},
		))),

		// The controlsocket worker runs on the controller.
		controlSocketName: ifDatabaseUpgradeComplete(controlsocket.Manifold(controlsocket.ManifoldConfig{
			DomainServicesName:              domainServicesName,
			ObjectStoreServicesName:         objectStoreServicesName,
			Logger:                          internallogger.GetLogger("juju.worker.controlsocket"),
			NewWorker:                       controlsocket.NewWorker,
			NewSocketListener:               controlsocket.NewSocketListener,
			SocketName:                      path.Join(agentConfig.DataDir(), "control.socket"),
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
			AgentName:                       agentName,
			S3ClientName:                    objectStoreS3CallerName,
			ObjectStoreName:                 objectStoreName,
			ObjectStoreServicesName:         objectStoreServicesName,
			FortressName:                    objectStoreFortressName,
			GetControllerService:            objectstoredrainer.GetControllerService,
			GeObjectStoreServices:           objectstoredrainer.GeObjectStoreServicesGetter,
			GetControllerObjectStoreService: objectstoredrainer.GetControllerObjectStoreService,
			GetGuardService:                 objectstoredrainer.GetGuardService,
			GetControllerConfigService:      objectstoredrainer.GetControllerConfigService,
			NewHashFileSystemAccessor:       objectstoredrainer.NewHashFileStoreAccessor,
			NewDrainerWorker:                objectstoredrainer.NewDrainWorker,
			SelectFileHash:                  internalobjectstore.SelectFileHash,
			NewWorker:                       objectstoredrainer.NewWorker,
			Logger:                          internallogger.GetLogger("juju.worker.objectstoredrainer"),
			Clock:                           config.Clock,
		}),

		objectStoreName: ifDatabaseUpgradeComplete(objectstore.Manifold(objectstore.ManifoldConfig{
			AgentName:                  agentName,
			TraceName:                  traceName,
			ObjectStoreServicesName:    objectStoreServicesName,
			LeaseManagerName:           leaseManagerName,
			S3ClientName:               objectStoreS3CallerName,
			APIRemoteCallerName:        apiRemoteCallerName,
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
		// ifDatabaseUpgradeComplete gate. The provider tracker data must
		// not change between patch/build versions and should be available
		// to all workers from the start.
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
			// GetCAASProvider uses caas.New directly: no NewCAASBrokerFunc
			// in ManifoldsConfig since the controller is always IAAS-like,
			// but providertracker validates both providers non-nil.
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
					charmhubLogger := internallogger.GetLogger("juju.charmhub", corelogger.CHARMHUB)
					return charmhub.DefaultHTTPClient(charmhubLogger)

				case corehttp.S3Purpose:
					s3Logger := internallogger.GetLogger("juju.objectstore.s3", corelogger.OBJECTSTORE)
					return s3client.DefaultHTTPClient(s3Logger)

				case corehttp.SSHImporterPurpose:
					sshImporterLogger := internallogger.GetLogger("juju.ssh.importer", corelogger.SSHIMPORTER)
					return sshimporter.DefaultHTTPClient(sshImporterLogger)

				case corehttp.MacaroonPurpose:
					macaroonLogger := internallogger.GetLogger("juju.macaroon", corelogger.MACAROON)
					return macaroon.DefaultHTTPClient(macaroonLogger)

				case corehttp.SimpleStreamPurpose:
					simplestreamLogger := internallogger.GetLogger("juju.simplestream", corelogger.SIMPLESTREAM)
					return simplestreams.DefaultHTTPClient(simplestreamLogger)

				default:
					return internalhttp.NewClient(opts...)
				}
			},
			NewHTTPClientWorker: httpclient.NewTrackedWorker,
			Clock:               config.Clock,
			Logger:              internallogger.GetLogger("juju.worker.httpclient"),
		}),

		apiRemoteCallerName: apiremotecaller.Manifold(apiremotecaller.ManifoldConfig{
			AgentName:               agentName,
			ObjectStoreServicesName: objectStoreServicesName,
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

		// Bootstrap worker is responsible for setting up the initial
		// controller.
		bootstrapName: ifDatabaseUpgradeComplete(bootstrap.Manifold(bootstrap.ManifoldConfig{
			AgentName:               agentName,
			ObjectStoreName:         objectStoreFacadeName,
			DomainServicesName:      domainServicesName,
			HTTPClientName:          httpClientName,
			BootstrapGateName:       isBootstrapGateName,
			ProviderFactoryName:     providerTrackerName,
			RequiresBootstrap:       bootstrap.RequiresBootstrap,
			PopulateControllerCharm: bootstrap.PopulateIAASControllerCharm,
			StatusHistory:           domain.NewStatusHistory(internallogger.GetLogger("juju.services"), config.Clock),
			Logger:                  internallogger.GetLogger("juju.worker.bootstrap"),
			Clock:                   config.Clock,

			AgentBinaryUploader:          bootstrap.IAASAgentBinaryUploader,
			ControllerCharmDeployer:      bootstrap.IAASControllerCharmUploader,
			ControllerUnitPassword:       bootstrap.IAASControllerUnitPassword,
			BootstrapAddressFinderGetter: bootstrap.IAASAddressFinder,
			AgentFinalizer:               bootstrap.IAASAgentFinalizer,
		})),

		agentConfigUpdaterName: ifNotMigrating(agentconfigupdater.Manifold(agentconfigupdater.ManifoldConfig{
			AgentName:                     agentName,
			APICallerName:                 apiCallerName,
			DomainServicesName:            domainServicesName,
			TraceName:                     traceName,
			GetControllerDomainServicesFn: agentconfigupdater.GetControllerDomainServices,
			IsControllerAgentFn:           agentconfigupdater.IAASIsControllerAgent,
			Logger:                        internallogger.GetLogger("juju.worker.agentconfigupdater"),
		})),

		certificateUpdaterName: ifFullyUpgraded(certupdater.Manifold(certupdater.ManifoldConfig{
			AuthorityName:               certificateWatcherName,
			DomainServicesName:          domainServicesName,
			GetControllerDomainServices: certupdater.GetControllerDomainServices,
			NewWorker:                   certupdater.NewCertificateUpdater,
			Logger:                      internallogger.GetLogger("juju.worker.certupdater"),
		})),

		// DBAccessor provides access to the Dqlite database. The
		// controller always uses the IAAS node manager.
		dbAccessorName: dbaccessor.Manifold(dbaccessor.ManifoldConfig{
			AgentName:                 agentName,
			QueryLoggerName:           queryLoggerName,
			ControllerAgentConfigName: controllerAgentConfigName,
			Clock:                     config.Clock,
			Logger:                    internallogger.GetLogger("juju.worker.dbaccessor"),
			LogDir:                    agentConfig.LogDir(),
			PrometheusRegisterer:      config.PrometheusRegisterer,
			NewApp:                    dbaccessor.NewApp,
			NewDBWorker:               config.NewDBWorkerFunc,
			NewMetricsCollector:       dbaccessor.NewMetricsCollector,
			NewNodeManager:            dbaccessor.IAASNodeManager,
		}),

		// The upgrader is a leaf worker that returns a specific error
		// type recognised by the controller agent, causing other workers
		// to be stopped and the agent to be restarted running the new
		// tools.
		upgraderName: ifControllerUpgradeComplete(upgrader.Manifold(upgrader.ManifoldConfig{
			AgentName:            agentName,
			APICallerName:        apiCallerName,
			UpgradeStepsGateName: upgradeStepsGateName,
			UpgradeCheckGateName: upgradeCheckGateName,
			PreviousAgentVersion: config.PreviousAgentVersion,
			Logger:               internallogger.GetLogger("juju.worker.upgrader"),
			Clock:                config.Clock,
		})),

		// The upgradestepscontroller worker runs soon after the
		// controller agent starts and runs any steps required to
		// upgrade to the running jujud version. Once upgrade steps have
		// run, the upgradesteps gate is unlocked and the worker exits.
		upgradeControllerStepsName: ifControllerUpgradeComplete(upgradestepscontroller.Manifold(upgradestepscontroller.ManifoldConfig{
			AgentName:            agentName,
			APICallerName:        apiCallerName,
			DomainServicesName:   domainServicesName,
			UpgradeStepsGateName: upgradeStepsGateName,
			PreUpgradeSteps:      config.PreUpgradeSteps(model.IAAS),
			UpgradeSteps:         config.UpgradeSteps,
			NewAgentStatusSetter: config.NewAgentStatusSetter,
			NewControllerWorker:  upgradestepscontroller.NewControllerWorker,
			GetUpgradeService:    upgradestepscontroller.GetUpgradeService,
			Logger:               internallogger.GetLogger("juju.worker.upgradestepscontroller"),
			Clock:                config.Clock,
		})),
	}
}

func clockManifold(clk clock.Clock) dependency.Manifold {
	return dependency.Manifold{
		Start: func(_ context.Context, _ dependency.Getter) (worker.Worker, error) {
			return engine.NewValueWorker(clk)
		},
		Output: engine.ValueWorkerOutput,
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

const (
	agentName              = "agent"
	agentConfigUpdaterName = "agent-config-updater"
	terminationName        = "termination-signal-handler"
	apiCallerName          = "api-caller"
	apiConfigWatcherName   = "api-config-watcher"
	clockName              = "clock"
	flightRecorderName     = "flight-recorder"

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

	apiAddressSetterName          = "api-address-setter"
	apiServerName                 = "api-server"
	apiRemoteCallerName           = "api-remote-caller"
	apiRemoteRelationCallerName   = "api-remote-relation-caller"
	auditConfigUpdaterName        = "audit-config-updater"
	certificateUpdaterName        = "certificate-updater"
	certificateWatcherName        = "certificate-watcher"
	changeStreamName              = "change-stream"
	changeStreamPrunerName        = "change-stream-pruner"
	controllerAgentConfigName     = "controller-agent-config"
	controllerPresenceName        = "controller-presence"
	controlSocketName             = "control-socket"
	dbAccessorName                = "db-accessor"
	domainServicesName            = "domain-services"
	externalControllerUpdaterName = "external-controller-updater"
	fileNotifyWatcherName         = "file-notify-watcher"
	httpClientName                = "http-client"
	httpServerArgsName            = "http-server-args"
	httpServerName                = "http-server"
	identityFileWriterName        = "ssh-identity-writer"
	isPrimaryControllerFlagName   = "is-primary-controller-flag"
	jwtParserName                 = "jwt-parser"
	leaseExpiryName               = "lease-expiry"
	leaseManagerName              = "lease-manager"
	loggingConfigUpdaterName      = "logging-config-updater"
	logSinkName                   = "log-sink"
	modelWorkerManagerName        = "model-worker-manager"
	objectStoreName               = "object-store"
	objectStoreS3CallerName       = "object-store-s3-caller"
	objectStoreServicesName       = "object-store-services"
	objectStoreFortressName       = "object-store-fortress"
	objectStoreFacadeName         = "object-store-facade"
	objectStoreDrainerName        = "object-store-drainer"
	providerDomainServicesName    = "provider-services"
	providerTrackerName           = "provider-tracker"
	queryLoggerName               = "query-logger"
	secretBackendRotateName       = "secret-backend-rotate"
	sshServerName                 = "ssh-server"
	storageRegistryName           = "storage-registry"
	traceName                     = "trace"
	undertakerName                = "undertaker"
	watcherRegistryName           = "watcher-registry"
)
