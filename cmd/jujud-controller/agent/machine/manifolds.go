// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"context"
	"net/http"
	"path"
	"runtime"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/proxy"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	"github.com/prometheus/client_golang/prometheus"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller/crosscontroller"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cmd/jujud-controller/util"
	corehttp "github.com/juju/juju/core/http"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	coretrace "github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/environs"
	internalbootstrap "github.com/juju/juju/internal/bootstrap"
	"github.com/juju/juju/internal/charmhub"
	containerbroker "github.com/juju/juju/internal/container/broker"
	"github.com/juju/juju/internal/container/lxd"
	internalhttp "github.com/juju/juju/internal/http"
	internallease "github.com/juju/juju/internal/lease"
	internallogger "github.com/juju/juju/internal/logger"
	internalobjectstore "github.com/juju/juju/internal/objectstore"
	proxyconfig "github.com/juju/juju/internal/proxy/config"
	"github.com/juju/juju/internal/s3client"
	sshimporter "github.com/juju/juju/internal/ssh/importer"
	"github.com/juju/juju/internal/upgrades"
	jupgradesteps "github.com/juju/juju/internal/upgradesteps"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/agent"
	"github.com/juju/juju/internal/worker/agentconfigupdater"
	"github.com/juju/juju/internal/worker/apiaddresssetter"
	"github.com/juju/juju/internal/worker/apiaddressupdater"
	"github.com/juju/juju/internal/worker/apicaller"
	"github.com/juju/juju/internal/worker/apiconfigwatcher"
	"github.com/juju/juju/internal/worker/apiremotecaller"
	"github.com/juju/juju/internal/worker/apiserver"
	"github.com/juju/juju/internal/worker/apiservercertwatcher"
	"github.com/juju/juju/internal/worker/auditconfigupdater"
	"github.com/juju/juju/internal/worker/authenticationworker"
	"github.com/juju/juju/internal/worker/bootstrap"
	"github.com/juju/juju/internal/worker/caasupgrader"
	"github.com/juju/juju/internal/worker/certupdater"
	"github.com/juju/juju/internal/worker/changestream"
	"github.com/juju/juju/internal/worker/changestreampruner"
	lxdbroker "github.com/juju/juju/internal/worker/containerbroker"
	"github.com/juju/juju/internal/worker/containerprovisioner"
	"github.com/juju/juju/internal/worker/controlleragentconfig"
	"github.com/juju/juju/internal/worker/controlsocket"
	"github.com/juju/juju/internal/worker/credentialvalidator"
	"github.com/juju/juju/internal/worker/dbaccessor"
	"github.com/juju/juju/internal/worker/deployer"
	"github.com/juju/juju/internal/worker/diskmanager"
	workerdomainservices "github.com/juju/juju/internal/worker/domainservices"
	"github.com/juju/juju/internal/worker/externalcontrollerupdater"
	"github.com/juju/juju/internal/worker/filenotifywatcher"
	"github.com/juju/juju/internal/worker/fortress"
	"github.com/juju/juju/internal/worker/gate"
	"github.com/juju/juju/internal/worker/hostkeyreporter"
	"github.com/juju/juju/internal/worker/httpclient"
	"github.com/juju/juju/internal/worker/httpserver"
	"github.com/juju/juju/internal/worker/httpserverargs"
	"github.com/juju/juju/internal/worker/identityfilewriter"
	"github.com/juju/juju/internal/worker/jwtparser"
	leasemanager "github.com/juju/juju/internal/worker/lease"
	"github.com/juju/juju/internal/worker/leaseexpiry"
	"github.com/juju/juju/internal/worker/logger"
	"github.com/juju/juju/internal/worker/logsink"
	"github.com/juju/juju/internal/worker/machineactions"
	"github.com/juju/juju/internal/worker/machineconverter"
	"github.com/juju/juju/internal/worker/machiner"
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
	"github.com/juju/juju/internal/worker/proxyupdater"
	"github.com/juju/juju/internal/worker/querylogger"
	"github.com/juju/juju/internal/worker/reboot"
	"github.com/juju/juju/internal/worker/secretbackendrotate"
	"github.com/juju/juju/internal/worker/singular"
	"github.com/juju/juju/internal/worker/sshserver"
	"github.com/juju/juju/internal/worker/stateconfigwatcher"
	"github.com/juju/juju/internal/worker/storageprovisioner"
	"github.com/juju/juju/internal/worker/storageregistry"
	"github.com/juju/juju/internal/worker/terminationworker"
	"github.com/juju/juju/internal/worker/toolsversionchecker"
	"github.com/juju/juju/internal/worker/trace"
	"github.com/juju/juju/internal/worker/undertaker"
	"github.com/juju/juju/internal/worker/upgradedatabase"
	"github.com/juju/juju/internal/worker/upgrader"
	"github.com/juju/juju/internal/worker/upgradesteps"
	"github.com/juju/juju/internal/worker/upgradestepsmachine"
	"github.com/juju/juju/internal/worker/watcherregistry"
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
	PreviousAgentVersion semversion.Number

	// BootstrapLock is passed to the bootstrap gate to coordinate
	// workers that shouldn't do anything until the bootstrap worker
	// is done.
	BootstrapLock gate.Lock

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

	// NewDBWorkerFunc returns a tracked db worker.
	NewDBWorkerFunc dbaccessor.NewDBWorkerFunc

	// PreUpgradeSteps is a function that is used by the upgradesteps
	// worker to ensure that conditions are OK for an upgrade to
	// proceed.
	PreUpgradeSteps func(model.ModelType) upgrades.PreUpgradeStepsFunc

	// UpgradeSteps is a function that is used by the upgradesteps
	// worker to perform the upgrade steps.
	UpgradeSteps upgrades.UpgradeStepsFunc

	// LogSink defines an interface for writing log records to a log sink.
	LogSink corelogger.LogSink

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

	// IsCaasConfig is true if this config is for a caas agent.
	IsCaasConfig bool

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

	newExternalControllerWatcherClient := func(ctx context.Context, apiInfo *api.Info) (
		externalcontrollerupdater.ExternalControllerWatcherClientCloser, string, error,
	) {
		conn, err := apicaller.NewExternalControllerConnection(ctx, apiInfo)
		if err != nil {
			return nil, "", errors.Trace(err)
		}
		return crosscontroller.NewClient(conn), conn.IPAddr(), nil
	}

	var externalUpdateProxyFunc func(proxy.Settings) error
	if runtime.GOOS == "linux" && !config.IsCaasConfig {
		externalUpdateProxyFunc = lxd.ConfigureLXDProxies
	}

	agentConfig := config.Agent.CurrentConfig()
	agentTag := agentConfig.Tag()
	controllerTag := agentConfig.Controller()

	manifolds := dependency.Manifolds{
		// The agent manifold references the enclosing agent, and is the
		// foundation stone on which most other manifolds ultimately depend.
		agentName: agent.Manifold(config.Agent),

		// The upgrade database gate is used to coordinate workers that should
		// not do anything until the upgrade-database worker has finished
		// running any required database upgrade steps.
		isBootstrapGateName: gate.ManifoldEx(config.BootstrapLock),
		isBootstrapFlagName: gate.FlagManifold(gate.FlagManifoldConfig{
			GateName:  isBootstrapGateName,
			NewWorker: gate.NewFlagWorker,
		}),

		// The termination worker returns ErrTerminateAgent if a
		// termination signal is received by the process it's running
		// in. It has no inputs and its only output is the error it
		// returns. It depends on the uninstall file having been
		// written *by the manual provider* at install time; it would
		// be Very Wrong Indeed to use SetCanUninstall in conjunction
		// with this code.
		terminationName: terminationworker.Manifold(),

		clockName: clockManifold(config.Clock),

		// Each machine agent has a flag manifold/worker which
		// reports whether or not the agent is a controller.
		isControllerFlagName: util.IsControllerFlagManifold(stateConfigWatcherName, true),

		// Controller agent config manifold watches the controller
		// agent config and bounces if it changes.
		controllerAgentConfigName: ifController(controlleragentconfig.Manifold(controlleragentconfig.ManifoldConfig{
			AgentName:         agentName,
			Clock:             config.Clock,
			Logger:            internallogger.GetLogger("juju.worker.controlleragentconfig"),
			NewSocketListener: controlleragentconfig.NewSocketListener,
			SocketName:        path.Join(agentConfig.DataDir(), "configchange.socket"),
		})),

		// The stateconfigwatcher manifold watches the machine agent's
		// configuration and reports if state serving info is
		// present. It will bounce itself if state serving info is
		// added or removed. It is intended as a dependency just for
		// the state manifold.
		stateConfigWatcherName: stateconfigwatcher.Manifold(stateconfigwatcher.ManifoldConfig{
			AgentName:          agentName,
			AgentConfigChanged: config.AgentConfigChanged,
		}),

		// The api-config-watcher manifold monitors the API server
		// addresses in the agent config and bounces when they
		// change. It's required as part of model migrations.
		apiConfigWatcherName: apiconfigwatcher.Manifold(apiconfigwatcher.ManifoldConfig{
			AgentName:          agentName,
			AgentConfigChanged: config.AgentConfigChanged,
			Logger:             internallogger.GetLogger("juju.worker.apiconfigwatcher"),
		}),

		// The certificate-watcher manifold monitors the API server
		// certificate in the agent config for changes, and parses
		// and offers the result to other manifolds. This is only
		// run by state servers.
		certificateWatcherName: ifController(apiservercertwatcher.Manifold(apiservercertwatcher.ManifoldConfig{
			AgentName: agentName,
		})),

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

		// The upgrade database gate is used to coordinate workers that should
		// not do anything until the upgrade-database worker has finished
		// running any required database upgrade steps.
		upgradeDatabaseGateName: ifController(gate.ManifoldEx(config.UpgradeDBLock)),
		upgradeDatabaseFlagName: ifController(gate.FlagManifold(gate.FlagManifoldConfig{
			GateName:  upgradeDatabaseGateName,
			NewWorker: gate.NewFlagWorker,
		})),

		// The upgrade-database worker runs soon after the machine agent starts
		// and runs any steps required to upgrade to the database to the
		// current version. Once upgrade steps have run, the upgrade-database
		// gate is unlocked and the worker exits.
		upgradeDatabaseName: ifController(upgradedatabase.Manifold(upgradedatabase.ManifoldConfig{
			AgentName:          agentName,
			UpgradeDBGateName:  upgradeDatabaseGateName,
			DBAccessorName:     dbAccessorName,
			DomainServicesName: domainServicesName,
			NewWorker:          upgradedatabase.NewUpgradeDatabaseWorker,
			Logger:             internallogger.GetLogger("juju.worker.upgradedatabase"),
			Clock:              config.Clock,
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
			Logger:            internallogger.GetLogger("juju.worker.migrationminion", corelogger.MIGRATION),
		}),

		// Each controller machine runs a singular worker which will
		// attempt to claim responsibility for running certain workers
		// that must not be run concurrently by multiple agents.
		isPrimaryControllerFlagName: ifController(singular.Manifold(singular.ManifoldConfig{
			AgentName:        agentName,
			LeaseManagerName: leaseManagerName,
			Clock:            config.Clock,
			Duration:         config.ControllerLeaseDuration,
			Claimant:         agentTag,
			Entity:           controllerTag,
			NewWorker:        singular.NewFlagWorker,
		})),

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
			AuthorityName:        certificateWatcherName,
			DomainServicesName:   domainServicesName,
			MuxName:              httpServerArgsName,
			APIServerName:        apiServerName,
			PrometheusRegisterer: config.PrometheusRegisterer,
			AgentName:            config.AgentName,
			Clock:                config.Clock,
			MuxShutdownWait:      config.MuxShutdownWait,
			LogDir:               agentConfig.LogDir(),
			Logger:               internallogger.GetLogger("juju.worker.httpserver"),
			GetControllerConfig:  httpserver.GetControllerConfig,
			NewTLSConfig:         httpserver.NewTLSConfig,
			NewWorker:            httpserver.NewWorkerShim,
		}),

		logSinkName: ifDatabaseUpgradeComplete(logsink.Manifold(logsink.ManifoldConfig{
			AgentTag:       agentTag,
			Clock:          config.Clock,
			NewWorker:      logsink.NewWorker,
			NewModelLogger: logsink.NewModelLogger,
			LogSink:        config.LogSink,
		})),

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

			// Note that although there is a transient dependency on dbaccessor
			// via changestream, the direct dependency supplies the capability
			// to remove databases corresponding to destroyed/migrated models.
			DomainServicesName: domainServicesName,
			ChangeStreamName:   changeStreamName,
			DBAccessorName:     dbAccessorName,

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
			ProviderServiceFactoriesName: providerDomainServicesName,
			NewWorker:                    modelworkermanager.New,
			NewModelWorker:               config.NewModelWorker,
			ModelMetrics:                 config.DependencyEngineMetrics,
			Logger:                       internallogger.GetLogger("juju.workers.modelworkermanager"),
			GetProviderServicesGetter:    modelworkermanager.GetProviderServicesGetter,
			GetControllerConfig:          modelworkermanager.GetControllerConfig,
		})),

		domainServicesName: workerdomainservices.Manifold(workerdomainservices.ManifoldConfig{
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

		queryLoggerName: ifController(querylogger.Manifold(querylogger.ManifoldConfig{
			LogDir: agentConfig.LogDir(),
			Clock:  config.Clock,
			Logger: internallogger.GetLogger("juju.worker.querylogger"),
		})),

		fileNotifyWatcherName: ifController(filenotifywatcher.Manifold(filenotifywatcher.ManifoldConfig{
			Clock:             config.Clock,
			Logger:            internallogger.GetLogger("juju.worker.filenotifywatcher"),
			NewWatcher:        filenotifywatcher.NewWatcher,
			NewINotifyWatcher: filenotifywatcher.NewINotifyWatcher,
		})),

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
			DBAccessor: dbAccessorName,
			Clock:      config.Clock,
			Logger:     internallogger.GetLogger("juju.worker.changestreampruner"),
			NewWorker:  changestreampruner.NewWorker,
		})),

		auditConfigUpdaterName: ifDatabaseUpgradeComplete(auditconfigupdater.Manifold(auditconfigupdater.ManifoldConfig{
			AgentName:                  agentName,
			DomainServicesName:         domainServicesName,
			NewWorker:                  auditconfigupdater.NewWorker,
			GetControllerConfigService: auditconfigupdater.GetControllerConfigService,
		})),

		// The lease expiry worker constantly deletes
		// leases with an expiry time in the past.
		leaseExpiryName: ifPrimaryController(leaseexpiry.Manifold(leaseexpiry.ManifoldConfig{
			ClockName:      clockName,
			DBAccessorName: dbAccessorName,
			TraceName:      traceName,
			Logger:         internallogger.GetLogger("juju.worker.leaseexpiry"),
			NewWorker:      leaseexpiry.NewWorker,
			NewStore:       leaseexpiry.NewStore,
		})),

		// The global lease manager tracks lease information in the Dqlite database.
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

		// The proxy config updater is a leaf worker that sets http/https/apt/etc
		// proxy settings.
		proxyConfigUpdater: ifNotMigrating(proxyupdater.Manifold(proxyupdater.ManifoldConfig{
			AgentName:           agentName,
			APICallerName:       apiCallerName,
			Logger:              internallogger.GetLogger("juju.worker.proxyupdater"),
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
			Logger:        internallogger.GetLogger("juju.worker.credentialvalidator"),
		}),

		secretBackendRotateName: ifNotMigrating(ifPrimaryController(secretbackendrotate.Manifold(
			secretbackendrotate.ManifoldConfig{
				APICallerName: apiCallerName,
				Logger:        internallogger.GetLogger("juju.worker.secretbackendsrotate"),
			},
		))),

		// The controlsocket worker runs on the controller machine.
		controlSocketName: ifDatabaseUpgradeComplete(controlsocket.Manifold(controlsocket.ManifoldConfig{
			DomainServicesName: domainServicesName,
			Logger:             internallogger.GetLogger("juju.worker.controlsocket"),
			NewWorker:          controlsocket.NewWorker,
			NewSocketListener:  controlsocket.NewSocketListener,
			SocketName:         path.Join(agentConfig.DataDir(), "control.socket"),
		})),

		// The ssh server worker runs on the controller machine.
		sshServerName: ifController(sshserver.Manifold(sshserver.ManifoldConfig{
			DomainServicesName:         domainServicesName,
			Logger:                     internallogger.GetLogger("juju.worker.sshserver"),
			NewServerWrapperWorker:     sshserver.NewServerWrapperWorker,
			NewServerWorker:            sshserver.NewServerWorker,
			GetControllerConfigService: sshserver.GetControllerConfigService,
		})),

		// The objectstore draining workers collaborate to run draining of blobs
		// between underlying object stores (s3 compatible). They are used to
		// drain; and to create a mechanism for running other workers so they
		// can't accidentally interfere with a draining in progress. Such a
		// manifold should depend on the objectstore facade, which will guard
		// against any objectstore operations while the draining is in progress.
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
			Logger:                       internallogger.GetLogger("juju.worker.objectstoreservices"),
			NewWorker:                    objectstoreservices.NewWorker,
			NewObjectStoreServices:       objectstoreservices.NewObjectStoreServices,
			NewObjectStoreServicesGetter: objectstoreservices.NewObjectStoreServicesGetter,
		}),

		objectStoreS3CallerName: ifDatabaseUpgradeComplete(objectstores3caller.Manifold(objectstores3caller.ManifoldConfig{
			HTTPClientName:             httpClientName,
			ObjectStoreServicesName:    objectStoreServicesName,
			NewClient:                  objectstores3caller.NewS3Client,
			Logger:                     internallogger.GetLogger("juju.worker.s3caller"),
			Clock:                      config.Clock,
			GetControllerConfigService: objectstores3caller.GetControllerConfigService,
			NewWorker:                  objectstores3caller.NewWorker,
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
			NewWorker:                    providertracker.NewWorker,
			NewTrackerWorker:             providertracker.NewTrackerWorker,
			NewEphemeralProvider:         providertracker.NewEphemeralProvider,
			GetProviderServicesGetter:    providertracker.GetProviderServicesGetter,
			GetIAASProvider: providertracker.IAASGetProvider(func(ctx context.Context, args environs.OpenParams, invalidator environs.CredentialInvalidator) (environs.Environ, error) {
				return config.NewEnvironFunc(ctx, args, invalidator)
			}),
			GetCAASProvider: providertracker.CAASGetProvider(func(ctx context.Context, args environs.OpenParams, invalidator environs.CredentialInvalidator) (caas.Broker, error) {
				return config.NewCAASBrokerFunc(ctx, args, invalidator)
			}),
			Logger: internallogger.GetLogger("juju.worker.providertracker"),
			Clock:  config.Clock,
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

				default:
					return internalhttp.NewClient(opts...)
				}
			},
			NewHTTPClientWorker: httpclient.NewTrackedWorker,
			Clock:               config.Clock,
			Logger:              internallogger.GetLogger("juju.worker.httpclient"),
		}),

		apiRemoteCallerName: ifController(apiremotecaller.Manifold(apiremotecaller.ManifoldConfig{
			AgentName:               agentName,
			ObjectStoreServicesName: objectStoreServicesName,
			Clock:                   config.Clock,
			Logger:                  internallogger.GetLogger("juju.worker.apiremotecaller"),
			NewWorker:               apiremotecaller.NewWorker,
		})),

		jwtParserName: ifController(jwtparser.Manifold(jwtparser.ManifoldConfig{
			GetControllerConfigService: jwtparser.GetControllerConfigService,
			DomainServicesName:         domainServicesName,
		})),

		apiAddressSetterName: ifPrimaryController(apiaddresssetter.Manifold(apiaddresssetter.ManifoldConfig{
			DomainServicesName:          domainServicesName,
			GetDomainServices:           apiaddresssetter.GetDomainServices,
			GetControllerDomainServices: apiaddresssetter.GetControllerDomainServices,
			NewWorker:                   apiaddresssetter.New,
			Logger:                      internallogger.GetLogger("juju.worker.apiaddresssetter"),
		})),

		undertakerName: ifController(undertaker.Manifold(undertaker.ManifoldConfig{
			DBAccessorName:            dbAccessorName,
			DomainServicesName:        domainServicesName,
			NewWorker:                 undertaker.NewWorker,
			GetControllerModelService: undertaker.GetControllerModelService,
			GetRemovalServiceGetter:   undertaker.GetRemovalServiceGetter,
			Logger:                    internallogger.GetLogger("juju.worker.undertaker"),
			Clock:                     config.Clock,
		})),

		watcherRegistryName: ifController(watcherregistry.Manifold(watcherregistry.ManifoldConfig{
			NewWorker: watcherregistry.NewWorker,
			Clock:     config.Clock,
			Logger:    internallogger.GetLogger("juju.worker.watcherregistry"),
		})),
	}

	return manifolds
}

// IAASManifolds returns a set of co-configured manifolds covering the
// various responsibilities of a IAAS machine agent.
func IAASManifolds(config ManifoldsConfig) dependency.Manifolds {
	agentConfig := config.Agent.CurrentConfig()

	manifolds := dependency.Manifolds{
		// Bootstrap worker is responsible for setting up the initial machine.
		bootstrapName: ifDatabaseUpgradeComplete(bootstrap.Manifold(bootstrap.ManifoldConfig{
			AgentName:               agentName,
			ObjectStoreName:         objectStoreFacadeName,
			DomainServicesName:      domainServicesName,
			HTTPClientName:          httpClientName,
			BootstrapGateName:       isBootstrapGateName,
			ProviderFactoryName:     providerTrackerName,
			StorageRegistryName:     storageRegistryName,
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

		toolsVersionCheckerName: ifNotMigrating(toolsversionchecker.Manifold(toolsversionchecker.ManifoldConfig{
			AgentName:          agentName,
			DomainServicesName: domainServicesName,
			GetModelUUID:       toolsversionchecker.GetModelUUID,
			GetDomainServices:  toolsversionchecker.GetModelDomainServices,
			NewWorker:          toolsversionchecker.New,
			Logger:             internallogger.GetLogger("juju.worker.toolsversionchecker"),
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

		certificateUpdaterName: ifFullyUpgraded(certupdater.Manifold(certupdater.ManifoldConfig{
			AuthorityName:               certificateWatcherName,
			DomainServicesName:          domainServicesName,
			GetControllerDomainServices: certupdater.GetControllerDomainServices,
			NewWorker:                   certupdater.NewCertificateUpdater,
			Logger:                      internallogger.GetLogger("juju.worker.certupdater"),
		})),

		// The machiner Worker will wait for the identified machine to become
		// Dying and make it Dead; or until the machine becomes Dead by other
		// means.
		machinerName: ifNotMigrating(machiner.Manifold(machiner.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
		})),

		// DBAccessor is a manifold that provides a DBAccessor worker
		// that can be used to access the database.
		dbAccessorName: ifController(dbaccessor.Manifold(dbaccessor.ManifoldConfig{
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
			Logger:        internallogger.GetLogger("juju.worker.apiaddressupdater"),
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

		// The upgradesteps worker runs soon after the machine agent
		// starts and runs any steps required to upgrade to the
		// running jujud version. Once upgrade steps have run, the
		// upgradesteps gate is unlocked and the worker exits.
		upgradeStepsName: upgradesteps.Manifold(upgradesteps.ManifoldConfig{
			AgentName:            agentName,
			APICallerName:        apiCallerName,
			DomainServicesName:   domainServicesName,
			UpgradeStepsGateName: upgradeStepsGateName,
			PreUpgradeSteps:      config.PreUpgradeSteps(model.IAAS),
			UpgradeSteps:         config.UpgradeSteps,
			NewAgentStatusSetter: config.NewAgentStatusSetter,
			NewMachineWorker:     upgradestepsmachine.NewMachineWorker,
			NewControllerWorker:  upgradesteps.NewControllerWorker,
			Logger:               internallogger.GetLogger("juju.worker.upgradesteps"),
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
			Logger:        internallogger.GetLogger("juju.worker.deployer"),

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
		// isNotControllerFlagName is only used for the machineconverter,
		isNotControllerFlagName: util.IsControllerFlagManifold(stateConfigWatcherName, false),
		machineConverterName: ifNotController(ifNotMigrating(machineconverter.Manifold(machineconverter.ManifoldConfig{
			AgentName:        agentName,
			APICallerName:    apiCallerName,
			Logger:           internallogger.GetLogger("juju.worker.machineconverter"),
			NewMachineClient: machineconverter.NewMachineClient,
			NewAgentClient:   machineconverter.NewAgentClient,
			NewConverter:     machineconverter.NewConverter,
		}))),

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
	agentConfig := config.Agent.CurrentConfig()

	return mergeManifolds(config, dependency.Manifolds{
		// Bootstrap worker is responsible for setting up the initial machine.
		bootstrapName: ifDatabaseUpgradeComplete(bootstrap.Manifold(bootstrap.ManifoldConfig{
			AgentName:               agentName,
			ObjectStoreName:         objectStoreFacadeName,
			DomainServicesName:      domainServicesName,
			HTTPClientName:          httpClientName,
			BootstrapGateName:       isBootstrapGateName,
			ProviderFactoryName:     providerTrackerName,
			StorageRegistryName:     storageRegistryName,
			RequiresBootstrap:       bootstrap.RequiresBootstrap,
			PopulateControllerCharm: bootstrap.PopulateCAASControllerCharm,
			StatusHistory:           domain.NewStatusHistory(internallogger.GetLogger("juju.services"), config.Clock),
			Logger:                  internallogger.GetLogger("juju.worker.bootstrap"),
			Clock:                   config.Clock,

			AgentBinaryUploader:          bootstrap.CAASAgentBinaryUploader,
			ControllerCharmDeployer:      bootstrap.CAASControllerCharmUploader,
			ControllerUnitPassword:       bootstrap.CAASControllerUnitPassword,
			BootstrapAddressFinderGetter: bootstrap.CAASAddressFinder,
			AgentFinalizer:               bootstrap.CAASAgentFinalizer,
		})),

		agentConfigUpdaterName: ifNotMigrating(agentconfigupdater.Manifold(agentconfigupdater.ManifoldConfig{
			AgentName:                     agentName,
			APICallerName:                 apiCallerName,
			DomainServicesName:            domainServicesName,
			TraceName:                     traceName,
			GetControllerDomainServicesFn: agentconfigupdater.GetControllerDomainServices,
			IsControllerAgentFn:           agentconfigupdater.CAASIsControllerAgent,
			Logger:                        internallogger.GetLogger("juju.worker.agentconfigupdater"),
		})),

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
		upgradeStepsName: upgradesteps.Manifold(upgradesteps.ManifoldConfig{
			AgentName:            agentName,
			APICallerName:        apiCallerName,
			DomainServicesName:   domainServicesName,
			UpgradeStepsGateName: upgradeStepsGateName,
			PreUpgradeSteps:      config.PreUpgradeSteps(model.CAAS),
			UpgradeSteps:         config.UpgradeSteps,
			NewAgentStatusSetter: config.NewAgentStatusSetter,
			NewMachineWorker:     upgradestepsmachine.NewMachineWorker,
			NewControllerWorker:  upgradesteps.NewControllerWorker,
			Logger:               internallogger.GetLogger("juju.worker.upgradesteps"),
			Clock:                config.Clock,
		}),

		// DBAccessor is a manifold that provides a DBAccessor worker
		// that can be used to access the database.
		dbAccessorName: ifController(dbaccessor.Manifold(dbaccessor.ManifoldConfig{
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
			NewNodeManager:            dbaccessor.CAASNodeManager,
		})),
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

// ifBootstrapComplete gates against the bootstrap worker completing.
// This ensures that all blobs (agent binaries and controller charm) are
// available before the machine agent starts.
// We currently use this to provide a happier experience for the user
// when bootstrapping a controller, before immediately going into HA. If the
// underlying object store storage is slow, then retrying for the agent binary
// against the controller can lead to slower HA deployment. It might be worth
// revisiting this in the future, so we release the gate as soon as the binaries
// are being uploaded.
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

var ifController = engine.Housing{
	Flags: []string{
		isControllerFlagName,
	},
}.Decorate

var ifNotController = engine.Housing{
	Flags: []string{
		isNotControllerFlagName,
	},
}.Decorate

var ifCredentialValid = engine.Housing{
	Flags: []string{
		validCredentialFlagName,
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
	stateConfigWatcherName = "state-config-watcher"
	apiCallerName          = "api-caller"
	apiConfigWatcherName   = "api-config-watcher"
	clockName              = "clock"

	bootstrapName       = "bootstrap"
	isBootstrapGateName = "is-bootstrap-gate"
	isBootstrapFlagName = "is-bootstrap-flag"

	upgradeDatabaseName     = "upgrade-database-runner"
	upgradeDatabaseGateName = "upgrade-database-gate"
	upgradeDatabaseFlagName = "upgrade-database-flag"

	upgraderName         = "upgrader"
	upgradeStepsName     = "upgrade-steps-runner"
	upgradeStepsGateName = "upgrade-steps-gate"
	upgradeStepsFlagName = "upgrade-steps-flag"
	upgradeCheckGateName = "upgrade-check-gate"
	upgradeCheckFlagName = "upgrade-check-flag"

	migrationFortressName     = "migration-fortress"
	migrationInactiveFlagName = "migration-inactive-flag"
	migrationMinionName       = "migration-minion"

	apiAddressSetterName          = "api-address-setter"
	apiAddressUpdaterName         = "api-address-updater"
	apiServerName                 = "api-server"
	apiRemoteCallerName           = "api-remote-caller"
	auditConfigUpdaterName        = "audit-config-updater"
	authenticationWorkerName      = "ssh-authkeys-updater"
	brokerTrackerName             = "broker-tracker"
	certificateUpdaterName        = "certificate-updater"
	certificateWatcherName        = "certificate-watcher"
	changeStreamName              = "change-stream"
	changeStreamPrunerName        = "change-stream-pruner"
	controllerAgentConfigName     = "controller-agent-config"
	controlSocketName             = "control-socket"
	dbAccessorName                = "db-accessor"
	deployerName                  = "deployer"
	diskManagerName               = "disk-manager"
	domainServicesName            = "domain-services"
	externalControllerUpdaterName = "external-controller-updater"
	fileNotifyWatcherName         = "file-notify-watcher"
	hostKeyReporterName           = "host-key-reporter"
	httpClientName                = "http-client"
	httpServerArgsName            = "http-server-args"
	httpServerName                = "http-server"
	identityFileWriterName        = "ssh-identity-writer"
	isControllerFlagName          = "is-controller-flag"
	isNotControllerFlagName       = "is-not-controller-flag"
	isPrimaryControllerFlagName   = "is-primary-controller-flag"
	jwtParserName                 = "jwt-parser"
	leaseExpiryName               = "lease-expiry"
	leaseManagerName              = "lease-manager"
	loggingConfigUpdaterName      = "logging-config-updater"
	logSinkName                   = "log-sink"
	lxdContainerProvisioner       = "lxd-container-provisioner"
	machineActionName             = "machine-action-runner"
	machinerName                  = "machiner"
	modelWorkerManagerName        = "model-worker-manager"
	objectStoreName               = "object-store"
	objectStoreS3CallerName       = "object-store-s3-caller"
	objectStoreServicesName       = "object-store-services"
	objectStoreFortressName       = "object-store-fortress"
	objectStoreFacadeName         = "object-store-facade"
	objectStoreDrainingFlagName   = "object-store-draining-flag"
	objectStoreDrainerName        = "object-store-drainer"
	providerDomainServicesName    = "provider-services"
	providerTrackerName           = "provider-tracker"
	proxyConfigUpdater            = "proxy-config-updater"
	queryLoggerName               = "query-logger"
	rebootName                    = "reboot-executor"
	secretBackendRotateName       = "secret-backend-rotate"
	sshServerName                 = "ssh-server"
	machineConverterName          = "machine-converter"
	storageProvisionerName        = "storage-provisioner"
	storageRegistryName           = "storage-registry"
	toolsVersionCheckerName       = "tools-version-checker"
	traceName                     = "trace"
	validCredentialFlagName       = "valid-credential-flag"
	undertakerName                = "undertaker"
	machineSetupName              = "machine-setup"
	watcherRegistryName           = "watcher-registry"
)
