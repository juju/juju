// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"net/http"
	"runtime"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/proxy"
	"github.com/juju/pubsub"
	"github.com/juju/utils/voyeur"
	"github.com/juju/version"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	"github.com/prometheus/client_golang/prometheus"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/crosscontroller"
	apideployer "github.com/juju/juju/api/deployer"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	containerbroker "github.com/juju/juju/container/broker"
	"github.com/juju/juju/container/lxd"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/state"
	proxyconfig "github.com/juju/juju/utils/proxy"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/agentconfigupdater"
	"github.com/juju/juju/worker/apiaddressupdater"
	"github.com/juju/juju/worker/apicaller"
	"github.com/juju/juju/worker/apiconfigwatcher"
	"github.com/juju/juju/worker/apiserver"
	"github.com/juju/juju/worker/apiservercertwatcher"
	"github.com/juju/juju/worker/auditconfigupdater"
	"github.com/juju/juju/worker/authenticationworker"
	"github.com/juju/juju/worker/caasupgrader"
	"github.com/juju/juju/worker/centralhub"
	"github.com/juju/juju/worker/certupdater"
	"github.com/juju/juju/worker/common"
	lxdbroker "github.com/juju/juju/worker/containerbroker"
	"github.com/juju/juju/worker/controllerport"
	"github.com/juju/juju/worker/credentialvalidator"
	"github.com/juju/juju/worker/deployer"
	"github.com/juju/juju/worker/diskmanager"
	"github.com/juju/juju/worker/externalcontrollerupdater"
	"github.com/juju/juju/worker/fanconfigurer"
	"github.com/juju/juju/worker/featureflag"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/globalclockupdater"
	"github.com/juju/juju/worker/hostkeyreporter"
	"github.com/juju/juju/worker/httpserver"
	"github.com/juju/juju/worker/httpserverargs"
	"github.com/juju/juju/worker/identityfilewriter"
	"github.com/juju/juju/worker/instancemutater"
	leasemanager "github.com/juju/juju/worker/lease/manifold"
	"github.com/juju/juju/worker/logger"
	"github.com/juju/juju/worker/logsender"
	"github.com/juju/juju/worker/machineactions"
	"github.com/juju/juju/worker/machiner"
	"github.com/juju/juju/worker/migrationflag"
	"github.com/juju/juju/worker/migrationminion"
	"github.com/juju/juju/worker/modelcache"
	"github.com/juju/juju/worker/modelworkermanager"
	"github.com/juju/juju/worker/multiwatcher"
	"github.com/juju/juju/worker/peergrouper"
	prworker "github.com/juju/juju/worker/presence"
	"github.com/juju/juju/worker/proxyupdater"
	psworker "github.com/juju/juju/worker/pubsub"
	"github.com/juju/juju/worker/raft"
	"github.com/juju/juju/worker/raft/raftbackstop"
	"github.com/juju/juju/worker/raft/raftclusterer"
	"github.com/juju/juju/worker/raft/raftflag"
	"github.com/juju/juju/worker/raft/raftforwarder"
	"github.com/juju/juju/worker/raft/rafttransport"
	"github.com/juju/juju/worker/reboot"
	"github.com/juju/juju/worker/restorewatcher"
	"github.com/juju/juju/worker/resumer"
	"github.com/juju/juju/worker/singular"
	workerstate "github.com/juju/juju/worker/state"
	"github.com/juju/juju/worker/stateconfigwatcher"
	"github.com/juju/juju/worker/storageprovisioner"
	"github.com/juju/juju/worker/terminationworker"
	"github.com/juju/juju/worker/toolsversionchecker"
	"github.com/juju/juju/worker/txnpruner"
	"github.com/juju/juju/worker/upgradedatabase"
	"github.com/juju/juju/worker/upgrader"
	"github.com/juju/juju/worker/upgradeseries"
	"github.com/juju/juju/worker/upgradesteps"
)

const (
	// globalClockUpdaterUpdateInterval is the interval between
	// global clock updates.
	globalClockUpdaterUpdateInterval = 1 * time.Second

	// globalClockUpdaterBackoffDelay is the amount of time to
	// delay when a concurrent global clock update is detected.
	globalClockUpdaterBackoffDelay = 10 * time.Second

	// leaseRequestTopic is the pubsub topic that lease FSM updates
	// will be published on.
	leaseRequestTopic = "lease.request"
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

	// OpenController is function used by the controller manifold to
	// create a *state.Controller.
	OpenController func(coreagent.Config) (*state.Controller, error)

	// OpenStatePool is function used by the state manifold to create a
	// *state.StatePool.
	OpenStatePool func(coreagent.Config) (*state.StatePool, error)

	// OpenStateForUpgrade is a function the upgradesteps worker can
	// use to establish a connection to state.
	OpenStateForUpgrade func() (*state.StatePool, error)

	// StartAPIWorkers is passed to the apiworkers manifold. It starts
	// workers which rely on an API connection (which have not yet
	// been converted to work directly with the dependency engine).
	StartAPIWorkers func(api.Connection) (worker.Worker, error)

	// PreUpgradeSteps is a function that is used by the upgradesteps
	// worker to ensure that conditions are OK for an upgrade to
	// proceed.
	PreUpgradeSteps func(*state.StatePool, coreagent.Config, bool, bool, bool) error

	// LogSource defines the channel type used to send log message
	// structs within the machine agent.
	LogSource logsender.LogRecordCh

	// newDeployContext gives the tests the opportunity to create a deployer.Context
	// that can be used for testing so as to avoid (1) deploying units to the system
	// running the tests and (2) get access to the *State used internally, so that
	// tests can be run without waiting for the 5s watcher refresh time to which we would
	// otherwise be restricted.
	NewDeployContext func(st *apideployer.State, agentConfig coreagent.Config) deployer.Context

	// Clock supplies timekeeping services to various workers.
	Clock clock.Clock

	// ValidateMigration is called by the migrationminion during the
	// migration process to check that the agent will be ok when
	// connected to the new target controller.
	ValidateMigration func(base.APICaller) error

	// PrometheusRegisterer is a prometheus.Registerer that may be used
	// by workers to register Prometheus metric collectors.
	PrometheusRegisterer prometheus.Registerer

	// CentralHub is the primary hub that exists in the apiserver.
	CentralHub *pubsub.StructuredHub

	// PubSubReporter is the introspection reporter for the pubsub forwarding
	// worker.
	PubSubReporter psworker.Reporter

	// PresenceRecorder
	PresenceRecorder presence.Recorder

	// UpdateLoggerConfig is a function that will save the specified
	// config value as the logging config in the agent.conf file.
	UpdateLoggerConfig func(string) error

	// UpdateControllerAPIPort is a function that will save the updated
	// controller api port in the agent.conf file.
	UpdateControllerAPIPort func(int) error

	// NewAgentStatusSetter provides upgradesteps.StatusSetter.
	NewAgentStatusSetter func(apiConn api.Connection) (upgradesteps.StatusSetter, error)

	// ControllerLeaseDuration defines for how long this agent will ask
	// for controller administration rights.
	ControllerLeaseDuration time.Duration

	// LogPruneInterval defines how frequently logs are pruned from
	// the database.
	LogPruneInterval time.Duration

	// TransactionPruneInterval defines how frequently mgo/txn transactions
	// are pruned from the database.
	TransactionPruneInterval time.Duration

	// SetStatePool is used by the state worker for informing the agent of
	// the StatePool that it creates, so we can pass it to the introspection
	// worker running outside of the dependency engine.
	SetStatePool func(*state.StatePool)

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

	// NewContainerBrokerFunc is a function opens a CAAS provider.
	NewContainerBrokerFunc caas.NewContainerBrokerFunc

	// NewBrokerFunc is a function opens a instance broker (LXD/KVM)
	NewBrokerFunc containerbroker.NewBrokerFunc

	// IsCaasConfig is true if this config is for a caas agent.
	IsCaasConfig bool
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
	//     e.g apicaller.ErrConnectImpossible.
	connectFilter := func(err error) error {
		cause := errors.Cause(err)
		if cause == apicaller.ErrConnectImpossible {
			return jworker.ErrTerminateAgent
		} else if cause == apicaller.ErrChangedPassword {
			return dependency.ErrBounce
		}
		return err
	}

	newExternalControllerWatcherClient := func(apiInfo *api.Info) (
		externalcontrollerupdater.ExternalControllerWatcherClientCloser, error,
	) {
		conn, err := apicaller.NewExternalControllerConnection(apiInfo)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return crosscontroller.NewClient(conn), nil
	}

	var externalUpdateProxyFunc func(proxy.Settings) error
	if runtime.GOOS == "linux" && !config.IsCaasConfig {
		externalUpdateProxyFunc = lxd.ConfigureLXDProxies
	}

	agentConfig := config.Agent.CurrentConfig()
	agentTag := agentConfig.Tag()
	controllerTag := agentConfig.Controller()

	leaseFSM := raftlease.NewFSM()

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

		// Each machine agent has a flag manifold/worker which
		// reports whether or not the agent is a controller.
		isControllerFlagName: isControllerFlagManifold(),

		// The stateconfigwatcher manifold watches the machine agent's
		// configuration and reports if state serving info is
		// present. It will bounce itself if state serving info is
		// added or removed. It is intended as a dependency just for
		// the state manifold.
		stateConfigWatcherName: stateconfigwatcher.Manifold(stateconfigwatcher.ManifoldConfig{
			AgentName:          agentName,
			AgentConfigChanged: config.AgentConfigChanged,
		}),

		// The centralhub manifold watches the state config to make sure it
		// only starts for machines that are api servers. Currently the hub is
		// passed in as config, but when the apiserver and peergrouper are
		// updated to use the dependency engine, the centralhub manifold
		// should also take the agentName so the worker can get the machine ID
		// for the creation of the hub.
		centralHubName: centralhub.Manifold(centralhub.ManifoldConfig{
			StateConfigWatcherName: stateConfigWatcherName,
			Hub:                    config.CentralHub,
		}),

		// The pubsub manifold gets the APIInfo from the agent config,
		// and uses this as a basis to talk to the other API servers.
		// The worker subscribes to the messages sent by the peergrouper
		// that defines the set of machines that are the API servers.
		// All non-local messages that originate from the machine that
		// is running the worker get forwarded to the other API servers.
		// This worker does not run in non-API server machines through
		// the hub dependency, as that is only available if the machine
		// is an API server.
		pubSubName: psworker.Manifold(psworker.ManifoldConfig{
			AgentName:      agentName,
			CentralHubName: centralHubName,
			Clock:          config.Clock,
			Logger:         loggo.GetLogger("juju.worker.pubsub"),
			NewWorker:      psworker.NewWorker,
			Reporter:       config.PubSubReporter,
		}),

		// The presence manifold listens to pubsub messages about the pubsub
		// forwarding connections and api connection and disconnections to
		// establish a view on which agents are "alive".
		presenceName: prworker.Manifold(prworker.ManifoldConfig{
			AgentName:              agentName,
			CentralHubName:         centralHubName,
			StateConfigWatcherName: stateConfigWatcherName,
			Recorder:               config.PresenceRecorder,
			Logger:                 loggo.GetLogger("juju.worker.presence"),
			NewWorker:              prworker.NewWorker,
		}),

		/* TODO(menn0) - this is currently unused, pending further
		 * refactoring in the state package.

			// The controller manifold creates a *state.Controller and
			// makes it available to other manifolds. It pings the MongoDB
			// session regularly and will die if pings fail.
			controllerName: workercontroller.Manifold(workercontroller.ManifoldConfig{
				AgentName:              agentName,
				StateConfigWatcherName: stateConfigWatcherName,
				OpenController:         config.OpenController,
			}),
		*/

		// The state manifold creates a *state.State and makes it
		// available to other manifolds. It pings the mongodb session
		// regularly and will die if pings fail.
		stateName: workerstate.Manifold(workerstate.ManifoldConfig{
			AgentName:              agentName,
			StateConfigWatcherName: stateConfigWatcherName,
			OpenStatePool:          config.OpenStatePool,
			SetStatePool:           config.SetStatePool,
		}),

		// The multiwatcher manifold watches all the changes in the database
		// through the AllWatcherBacking and manages notifying the multiwatchers.
		multiwatcherName: ifDatabaseUpgradeComplete(ifController(multiwatcher.Manifold(multiwatcher.ManifoldConfig{
			StateName:            stateName,
			Logger:               loggo.GetLogger("juju.worker.multiwatcher"),
			PrometheusRegisterer: config.PrometheusRegisterer,
			NewWorker:            multiwatcher.NewWorkerShim,
			NewAllWatcher:        state.NewAllWatcherBacking,
		}))),

		// The model cache initialized gate is used to make sure the api server
		// isn't created before the model cache has been initialized with the
		// initial state of the world.
		modelCacheInitializedGateName: ifController(gate.Manifold()),
		modelCacheInitializedFlagName: ifController(gate.FlagManifold(gate.FlagManifoldConfig{
			GateName:  modelCacheInitializedGateName,
			NewWorker: gate.NewFlagWorker,
		})),

		// The modelcache manifold creates a cache.Controller and keeps
		// it up to date using an all model watcher. The controller is then
		// used by the apiserver.
		modelCacheName: ifDatabaseUpgradeComplete(ifController(modelcache.Manifold(modelcache.ManifoldConfig{
			StateName:            stateName,
			CentralHubName:       centralHubName,
			MultiwatcherName:     multiwatcherName,
			InitializedGateName:  modelCacheInitializedGateName,
			Logger:               loggo.GetLogger("juju.worker.modelcache"),
			PrometheusRegisterer: config.PrometheusRegisterer,
			NewWorker:            modelcache.NewWorker,
		}))),

		// The api-config-watcher manifold monitors the API server
		// addresses in the agent config and bounces when they
		// change. It's required as part of model migrations.
		apiConfigWatcherName: apiconfigwatcher.Manifold(apiconfigwatcher.ManifoldConfig{
			AgentName:          agentName,
			AgentConfigChanged: config.AgentConfigChanged,
			Logger:             loggo.GetLogger("juju.worker.apiconfigwatcher"),
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
			Logger:               loggo.GetLogger("juju.worker.apicaller"),
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
			AgentName:         agentName,
			UpgradeDBGateName: upgradeDatabaseGateName,
			OpenState:         config.OpenStateForUpgrade,
			Logger:            loggo.GetLogger("juju.worker.upgradedatabase"),
			Clock:             config.Clock,
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

		// The upgradesteps worker runs soon after the machine agent
		// starts and runs any steps required to upgrade to the
		// running jujud version. Once upgrade steps have run, the
		// upgradesteps gate is unlocked and the worker exits.
		upgradeStepsName: upgradesteps.Manifold(upgradesteps.ManifoldConfig{
			AgentName:            agentName,
			APICallerName:        apiCallerName,
			UpgradeStepsGateName: upgradeStepsGateName,
			OpenStateForUpgrade:  config.OpenStateForUpgrade,
			PreUpgradeSteps:      config.PreUpgradeSteps,
			NewAgentStatusSetter: config.NewAgentStatusSetter,
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
			Logger:            loggo.GetLogger("juju.worker.migrationminion"),
		}),

		// We also run another clock updater to feed time updates into
		// the lease FSM.
		leaseClockUpdaterName: globalclockupdater.Manifold(globalclockupdater.ManifoldConfig{
			Clock:            config.Clock,
			LeaseManagerName: leaseManagerName,
			RaftName:         raftForwarderName,
			NewWorker:        globalclockupdater.NewWorker,
			UpdateInterval:   globalClockUpdaterUpdateInterval,
			BackoffDelay:     globalClockUpdaterBackoffDelay,
			Logger:           loggo.GetLogger("juju.worker.globalclockupdater.raft"),
		}),

		// Each controller machine runs a singular worker which will
		// attempt to claim responsibility for running certain workers
		// that must not be run concurrently by multiple agents.
		isPrimaryControllerFlagName: ifController(singular.Manifold(singular.ManifoldConfig{
			Clock:         config.Clock,
			APICallerName: apiCallerName,
			Duration:      config.ControllerLeaseDuration,
			Claimant:      agentTag,
			Entity:        controllerTag,
			NewFacade:     singular.NewFacade,
			NewWorker:     singular.NewWorker,
		})),

		// The agent-config-updater manifold sets the state serving info from
		// the API connection and writes it to the agent config.
		agentConfigUpdaterName: ifNotMigrating(agentconfigupdater.Manifold(agentconfigupdater.ManifoldConfig{
			AgentName:      agentName,
			APICallerName:  apiCallerName,
			CentralHubName: centralHubName,
			Logger:         loggo.GetLogger("juju.worker.agentconfigupdater"),
		})),

		// The apiworkers manifold starts workers which rely on the
		// machine agent's API connection but have not been converted
		// to work directly under the dependency engine. It waits for
		// upgrades to be finished before starting these workers.
		apiWorkersName: ifNotMigrating(APIWorkersManifold(APIWorkersConfig{
			APICallerName:   apiCallerName,
			StartAPIWorkers: config.StartAPIWorkers,
		})),

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

		resumerName: ifNotMigrating(resumer.Manifold(resumer.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			Clock:         config.Clock,
			Interval:      time.Minute,
			NewFacade:     resumer.NewFacade,
			NewWorker:     resumer.NewWorker,
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

		txnPrunerName: ifNotMigrating(ifPrimaryController(txnpruner.Manifold(
			txnpruner.ManifoldConfig{
				ClockName:     clockName,
				StateName:     stateName,
				PruneInterval: config.TransactionPruneInterval,
				NewWorker:     txnpruner.New,
			},
		))),

		httpServerArgsName: httpserverargs.Manifold(httpserverargs.ManifoldConfig{
			ClockName:             clockName,
			ControllerPortName:    controllerPortName,
			StateName:             stateName,
			NewStateAuthenticator: httpserverargs.NewStateAuthenticator,
		}),

		// TODO Juju 3.0: the controller port worker is only needed while
		// the controller port is a mutable controller config value.
		// When we hit 3.0 we should make controller-port a required
		// and immutable value.
		controllerPortName: controllerport.Manifold(controllerport.ManifoldConfig{
			AgentName:               agentName,
			HubName:                 centralHubName,
			StateName:               stateName,
			Logger:                  loggo.GetLogger("juju.worker.controllerport"),
			UpdateControllerAPIPort: config.UpdateControllerAPIPort,
			GetControllerConfig:     controllerport.GetControllerConfig,
			NewWorker:               controllerport.NewWorker,
		}),

		httpServerName: httpserver.Manifold(httpserver.ManifoldConfig{
			AuthorityName:        certificateWatcherName,
			HubName:              centralHubName,
			StateName:            stateName,
			MuxName:              httpServerArgsName,
			APIServerName:        apiServerName,
			RaftTransportName:    raftTransportName,
			PrometheusRegisterer: config.PrometheusRegisterer,
			AgentName:            config.AgentName,
			Clock:                config.Clock,
			MuxShutdownWait:      config.MuxShutdownWait,
			LogDir:               agentConfig.LogDir(),
			GetControllerConfig:  httpserver.GetControllerConfig,
			NewTLSConfig:         httpserver.NewTLSConfig,
			NewWorker:            httpserver.NewWorkerShim,
		}),

		apiServerName: ifModelCacheInitialized(apiserver.Manifold(apiserver.ManifoldConfig{
			AgentName:              agentName,
			AuthenticatorName:      httpServerArgsName,
			ClockName:              clockName,
			StateName:              stateName,
			ModelCacheName:         modelCacheName,
			MultiwatcherName:       multiwatcherName,
			MuxName:                httpServerArgsName,
			LeaseManagerName:       leaseManagerName,
			UpgradeGateName:        upgradeStepsGateName,
			RestoreStatusName:      restoreWatcherName,
			AuditConfigUpdaterName: auditConfigUpdaterName,
			// Synthetic dependency - if raft-transport bounces we
			// need to bounce api-server too, otherwise http-server
			// can't shutdown properly.
			RaftTransportName: raftTransportName,

			PrometheusRegisterer:              config.PrometheusRegisterer,
			RegisterIntrospectionHTTPHandlers: config.RegisterIntrospectionHTTPHandlers,
			Hub:                               config.CentralHub,
			Presence:                          config.PresenceRecorder,
			NewWorker:                         apiserver.NewWorker,
			NewMetricsCollector:               apiserver.NewMetricsCollector,
		})),

		modelWorkerManagerName: ifFullyUpgraded(modelworkermanager.Manifold(modelworkermanager.ManifoldConfig{
			AgentName:      agentName,
			AuthorityName:  certificateWatcherName,
			StateName:      stateName,
			Clock:          config.Clock,
			MuxName:        httpServerArgsName,
			NewWorker:      modelworkermanager.New,
			NewModelWorker: config.NewModelWorker,
			Logger:         loggo.GetLogger("juju.workers.modelworkermanager"),
		})),

		peergrouperName: ifFullyUpgraded(peergrouper.Manifold(peergrouper.ManifoldConfig{
			AgentName:            agentName,
			ClockName:            clockName,
			ControllerPortName:   controllerPortName,
			StateName:            stateName,
			Hub:                  config.CentralHub,
			PrometheusRegisterer: config.PrometheusRegisterer,
			NewWorker:            peergrouper.New,
		})),

		restoreWatcherName: restorewatcher.Manifold(restorewatcher.ManifoldConfig{
			StateName: stateName,
			NewWorker: restorewatcher.NewWorker,
		}),

		auditConfigUpdaterName: ifController(auditconfigupdater.Manifold(auditconfigupdater.ManifoldConfig{
			AgentName: agentName,
			StateName: stateName,
			NewWorker: auditconfigupdater.New,
		})),

		raftTransportName: ifController(rafttransport.Manifold(rafttransport.ManifoldConfig{
			ClockName:         clockName,
			AgentName:         agentName,
			AuthenticatorName: httpServerArgsName,
			HubName:           centralHubName,
			MuxName:           httpServerArgsName,
			DialConn:          rafttransport.DialConn,
			NewWorker:         rafttransport.NewWorker,
			Path:              "/raft",
		})),

		raftName: ifFullyUpgraded(raft.Manifold(raft.ManifoldConfig{
			ClockName:            clockName,
			AgentName:            agentName,
			TransportName:        raftTransportName,
			FSM:                  leaseFSM,
			Logger:               loggo.GetLogger("juju.worker.raft"),
			PrometheusRegisterer: config.PrometheusRegisterer,
			NewWorker:            raft.NewWorker,
		})),

		raftFlagName: raftflag.Manifold(raftflag.ManifoldConfig{
			RaftName:  raftName,
			NewWorker: raftflag.NewWorker,
		}),

		// The raft clusterer can only run on the raft leader, since
		// it makes configuration updates based on changes in API
		// server details.
		raftClustererName: ifRaftLeader(raftclusterer.Manifold(raftclusterer.ManifoldConfig{
			RaftName:       raftName,
			CentralHubName: centralHubName,
			NewWorker:      raftclusterer.NewWorker,
		})),

		raftBackstopName: raftbackstop.Manifold(raftbackstop.ManifoldConfig{
			RaftName:       raftName,
			CentralHubName: centralHubName,
			AgentName:      agentName,
			Logger:         loggo.GetLogger("juju.worker.raft.raftbackstop"),
			NewWorker:      raftbackstop.NewWorker,
		}),

		// The raft forwarder accepts FSM commands from the hub and
		// applies them to the raft leader.
		raftForwarderName: ifRaftLeader(raftforwarder.Manifold(raftforwarder.ManifoldConfig{
			AgentName:            agentName,
			RaftName:             raftName,
			StateName:            stateName,
			CentralHubName:       centralHubName,
			RequestTopic:         leaseRequestTopic,
			Logger:               loggo.GetLogger("juju.worker.raft.raftforwarder"),
			PrometheusRegisterer: config.PrometheusRegisterer,
			NewWorker:            raftforwarder.NewWorker,
			NewTarget:            raftforwarder.NewTarget,
		})),

		// The global lease manager tracks lease information in the raft
		// cluster rather than in mongo.
		leaseManagerName: ifController(leasemanager.Manifold(leasemanager.ManifoldConfig{
			AgentName:            agentName,
			ClockName:            clockName,
			CentralHubName:       centralHubName,
			StateName:            stateName,
			FSM:                  leaseFSM,
			RequestTopic:         leaseRequestTopic,
			Logger:               loggo.GetLogger("juju.worker.lease.raft"),
			LogDir:               agentConfig.LogDir(),
			PrometheusRegisterer: config.PrometheusRegisterer,
			NewWorker:            leasemanager.NewWorker,
			NewStore:             leasemanager.NewStore,
		})),

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

		certificateUpdaterName: ifFullyUpgraded(certupdater.Manifold(certupdater.ManifoldConfig{
			AgentName:                agentName,
			AuthorityName:            certificateWatcherName,
			StateName:                stateName,
			NewWorker:                certupdater.NewCertificateUpdater,
			NewMachineAddressWatcher: certupdater.NewMachineAddressWatcher,
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
		})),

		// TODO(legacy-leases): remove this.
		legacyLeasesFlagName: ifController(featureflag.Manifold(featureflag.ManifoldConfig{
			StateName: stateName,
			FlagName:  "legacy-leases-always-off",
			Logger:    loggo.GetLogger("juju.worker.legacyleasesenabled"),
			NewWorker: featureflag.NewWorker,
		})),

		// We run clock updaters for every controller machine to
		// ensure the lease clock is updated monotonically and at a
		// rate no faster than real time.
		//
		// If the legacy-leases feature flag is set the global clock
		// updater updates the lease clock in the database.  .
		globalClockUpdaterName: ifLegacyLeasesEnabled(globalclockupdater.Manifold(globalclockupdater.ManifoldConfig{
			Clock:          config.Clock,
			StateName:      stateName,
			NewWorker:      globalclockupdater.NewWorker,
			UpdateInterval: globalClockUpdaterUpdateInterval,
			BackoffDelay:   globalClockUpdaterBackoffDelay,
			Logger:         loggo.GetLogger("juju.worker.globalclockupdater.mongo"),
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
		}),

		upgradeSeriesWorkerName: ifNotMigrating(upgradeseries.Manifold(upgradeseries.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			Logger:        loggo.GetLogger("juju.worker.upgradeseries"),
			NewFacade:     upgradeseries.NewFacade,
			NewWorker:     upgradeseries.NewWorker,
		})),

		// The deployer worker is primary for deploying and recalling unit
		// agents, according to changes in a set of state units; and for the
		// final removal of its agents' units from state when they are no
		// longer needed.
		deployerName: ifNotMigrating(deployer.Manifold(deployer.ManifoldConfig{
			NewDeployContext: config.NewDeployContext,
			AgentName:        agentName,
			APICallerName:    apiCallerName,
		})),

		// The reboot manifold manages a worker which will reboot the
		// machine when requested. It needs an API connection and
		// waits for upgrades to be complete.
		rebootName: ifNotMigrating(reboot.Manifold(reboot.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			MachineLock:   config.MachineLock,
			Clock:         config.Clock,
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
			Logger:        loggo.GetLogger("juju.worker.instancemutater"),
			NewClient:     instancemutater.NewClient,
			NewWorker:     instancemutater.NewContainerWorker,
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
		Start: func(_ dependency.Context) (worker.Worker, error) {
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

var ifRaftLeader = engine.Housing{
	Flags: []string{
		raftFlagName,
	},
}.Decorate

var ifCredentialValid = engine.Housing{
	Flags: []string{
		validCredentialFlagName,
	},
}.Decorate

var ifLegacyLeasesEnabled = engine.Housing{
	Flags: []string{
		legacyLeasesFlagName,
	},
}.Decorate

var ifModelCacheInitialized = engine.Housing{
	Flags: []string{
		modelCacheInitializedFlagName,
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
	controllerPortName     = "controller-port"
	stateName              = "state"
	apiCallerName          = "api-caller"
	apiConfigWatcherName   = "api-config-watcher"
	centralHubName         = "central-hub"
	presenceName           = "presence"
	pubSubName             = "pubsub-forwarder"
	clockName              = "clock"

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

	apiWorkersName                = "unconverted-api-workers"
	rebootName                    = "reboot-executor"
	loggingConfigUpdaterName      = "logging-config-updater"
	diskManagerName               = "disk-manager"
	proxyConfigUpdater            = "proxy-config-updater"
	apiAddressUpdaterName         = "api-address-updater"
	machinerName                  = "machiner"
	logSenderName                 = "log-sender"
	deployerName                  = "unit-agent-deployer"
	authenticationWorkerName      = "ssh-authkeys-updater"
	storageProvisionerName        = "storage-provisioner"
	resumerName                   = "mgo-txn-resumer"
	identityFileWriterName        = "ssh-identity-writer"
	toolsVersionCheckerName       = "tools-version-checker"
	machineActionName             = "machine-action-runner"
	hostKeyReporterName           = "host-key-reporter"
	fanConfigurerName             = "fan-configurer"
	externalControllerUpdaterName = "external-controller-updater"
	globalClockUpdaterName        = "global-clock-updater"
	leaseClockUpdaterName         = "lease-clock-updater"
	isPrimaryControllerFlagName   = "is-primary-controller-flag"
	isControllerFlagName          = "is-controller-flag"
	instanceMutaterName           = "instance-mutater"
	txnPrunerName                 = "transaction-pruner"
	certificateWatcherName        = "certificate-watcher"
	modelCacheName                = "model-cache"
	modelCacheInitializedFlagName = "model-cache-initialized-flag"
	modelCacheInitializedGateName = "model-cache-initialized-gate"
	modelWorkerManagerName        = "model-worker-manager"
	multiwatcherName              = "multiwatcher"
	peergrouperName               = "peer-grouper"
	restoreWatcherName            = "restore-watcher"
	certificateUpdaterName        = "certificate-updater"
	auditConfigUpdaterName        = "audit-config-updater"
	leaseManagerName              = "lease-manager"
	legacyLeasesFlagName          = "legacy-leases-flag"

	upgradeSeriesWorkerName = "upgrade-series"

	httpServerName     = "http-server"
	httpServerArgsName = "http-server-args"
	apiServerName      = "api-server"

	raftTransportName = "raft-transport"
	raftName          = "raft"
	raftClustererName = "raft-clusterer"
	raftFlagName      = "raft-leader-flag"
	raftBackstopName  = "raft-backstop"
	raftForwarderName = "raft-forwarder"

	validCredentialFlagName = "valid-credential-flag"

	brokerTrackerName = "broker-tracker"
)
