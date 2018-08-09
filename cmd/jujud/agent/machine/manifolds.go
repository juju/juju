// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"net/http"
	"runtime"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/proxy"
	"github.com/juju/pubsub"
	"github.com/juju/utils/clock"
	utilsfeatureflag "github.com/juju/utils/featureflag"
	"github.com/juju/utils/voyeur"
	"github.com/juju/version"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/crosscontroller"
	apideployer "github.com/juju/juju/api/deployer"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/container/lxd"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
	proxyconfig "github.com/juju/juju/utils/proxy"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/apiaddressupdater"
	"github.com/juju/juju/worker/apicaller"
	"github.com/juju/juju/worker/apiconfigwatcher"
	"github.com/juju/juju/worker/apiserver"
	"github.com/juju/juju/worker/apiservercertwatcher"
	"github.com/juju/juju/worker/auditconfigupdater"
	"github.com/juju/juju/worker/authenticationworker"
	"github.com/juju/juju/worker/centralhub"
	"github.com/juju/juju/worker/certupdater"
	"github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/credentialvalidator"
	"github.com/juju/juju/worker/dblogpruner"
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
	"github.com/juju/juju/worker/identityfilewriter"
	"github.com/juju/juju/worker/logger"
	"github.com/juju/juju/worker/logsender"
	"github.com/juju/juju/worker/machineactions"
	"github.com/juju/juju/worker/machiner"
	"github.com/juju/juju/worker/migrationflag"
	"github.com/juju/juju/worker/migrationminion"
	"github.com/juju/juju/worker/modelworkermanager"
	"github.com/juju/juju/worker/peergrouper"
	prworker "github.com/juju/juju/worker/presence"
	"github.com/juju/juju/worker/proxyupdater"
	psworker "github.com/juju/juju/worker/pubsub"
	"github.com/juju/juju/worker/raft"
	"github.com/juju/juju/worker/raft/raftbackstop"
	"github.com/juju/juju/worker/raft/raftclusterer"
	"github.com/juju/juju/worker/raft/raftflag"
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
)

// ManifoldsConfig allows specialisation of the result of Manifolds.
type ManifoldsConfig struct {

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

	// OpenController is function used by the controller manifold to
	// create a *state.Controller.
	OpenController func(coreagent.Config) (*state.Controller, error)

	// OpenState is function used by the state manifold to create a
	// *state.State.
	OpenState func(coreagent.Config) (*state.State, error)

	// OpenStateForUpgrade is a function the upgradesteps worker can
	// use to establish a connection to state.
	OpenStateForUpgrade func() (*state.State, error)

	// StartAPIWorkers is passed to the apiworkers manifold. It starts
	// workers which rely on an API connection (which have not yet
	// been converted to work directly with the dependency engine).
	StartAPIWorkers func(api.Connection) (worker.Worker, error)

	// PreUpgradeSteps is a function that is used by the upgradesteps
	// worker to ensure that conditions are OK for an upgrade to
	// proceed.
	PreUpgradeSteps func(*state.State, coreagent.Config, bool, bool) error

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
	NewModelWorker func(modelUUID string, modelType state.ModelType) (worker.Worker, error)

	// ControllerSupportsSpaces is a function that reports whether or
	// not the controller model, represented by the given *state.State,
	// supports network spaces.
	ControllerSupportsSpaces func(*state.State) (bool, error)

	// MachineLock is a central source for acquiring the machine lock.
	// This is used by a number of workers to ensure serialisation of actions
	// across the machine.
	MachineLock machinelock.Lock
}

// Manifolds returns a set of co-configured manifolds covering the
// various responsibilities of a machine agent.
//
// Thou Shalt Not Use String Literals In This Function. Or Else.
func Manifolds(config ManifoldsConfig) dependency.Manifolds {

	// connectFilter exists:
	//  1) to let us retry api connections immediately on password change,
	//     rather than causing the dependency engine to wait for a while;
	//  2) to ensure that certain connection failures correctly trigger
	//     complete agent removal. (It's not safe to let any agent other
	//     than the machine mess around with SetCanUninstall).
	connectFilter := func(err error) error {
		cause := errors.Cause(err)
		if cause == apicaller.ErrConnectImpossible {
			err2 := coreagent.SetCanUninstall(config.Agent)
			if err2 != nil {
				return errors.Trace(err2)
			}
			return jworker.ErrTerminateAgent
		} else if cause == apicaller.ErrChangedPassword {
			return dependency.ErrBounce
		}
		return err
	}
	var externalUpdateProxyFunc func(proxy.Settings) error
	if runtime.GOOS == "linux" {
		externalUpdateProxyFunc = lxd.ConfigureLXDProxies
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

	agentConfig := config.Agent.CurrentConfig()
	machineTag := agentConfig.Tag().(names.MachineTag)
	controllerTag := agentConfig.Controller()

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
			Hub: config.CentralHub,
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
			OpenState:              config.OpenState,
			PrometheusRegisterer:   config.PrometheusRegisterer,
			SetStatePool:           config.SetStatePool,
		}),

		// The api-config-watcher manifold monitors the API server
		// addresses in the agent config and bounces when they
		// change. It's required as part of model migrations.
		apiConfigWatcherName: apiconfigwatcher.Manifold(apiconfigwatcher.ManifoldConfig{
			AgentName:          agentName,
			AgentConfigChanged: config.AgentConfigChanged,
		}),

		// The certificate-watcher manifold monitors the API server
		// certificate in the agent config for changes, and parses
		// and offers the result to other manifolds. This is only
		// run by state servers.
		certificateWatcherName: ifController(apiservercertwatcher.Manifold(apiservercertwatcher.ManifoldConfig{
			AgentName:          agentName,
			AgentConfigChanged: config.AgentConfigChanged,
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
			APIOpen:           api.Open,
			ValidateMigration: config.ValidateMigration,
			NewFacade:         migrationminion.NewFacade,
			NewWorker:         migrationminion.NewWorker,
		}),

		// We run a global clock updater for every controller machine.
		//
		// The global clock updater is primary for detecting and
		// preventing concurrent updates, to ensure global time is
		// monotonic and increases at a rate no faster than real time.
		globalClockUpdaterName: globalclockupdater.Manifold(globalclockupdater.ManifoldConfig{
			ClockName:      clockName,
			StateName:      stateName,
			NewWorker:      globalclockupdater.NewWorker,
			UpdateInterval: globalClockUpdaterUpdateInterval,
			BackoffDelay:   globalClockUpdaterBackoffDelay,
		}),

		// Each controller machine runs a singular worker which will
		// attempt to claim responsibility for running certain workers
		// that must not be run concurrently by multiple agents.
		isPrimaryControllerFlagName: ifController(singular.Manifold(singular.ManifoldConfig{
			ClockName:     clockName,
			APICallerName: apiCallerName,
			Duration:      config.ControllerLeaseDuration,
			Claimant:      machineTag,
			Entity:        controllerTag,
			NewFacade:     singular.NewFacade,
			NewWorker:     singular.NewWorker,
		})),

		// The serving-info-setter manifold sets grabs the state
		// serving info from the API connection and writes it to the
		// agent config.
		servingInfoSetterName: ifNotMigrating(ServingInfoSetterManifold(ServingInfoSetterConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
		})),

		// The apiworkers manifold starts workers which rely on the
		// machine agent's API connection but have not been converted
		// to work directly under the dependency engine. It waits for
		// upgrades to be finished before starting these workers.
		apiWorkersName: ifNotMigrating(APIWorkersManifold(APIWorkersConfig{
			APICallerName:   apiCallerName,
			StartAPIWorkers: config.StartAPIWorkers,
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

		// The logging config updater is a leaf worker that indirectly
		// controls the messages sent via the log sender or rsyslog,
		// according to changes in environment config. We should only need
		// one of these in a consolidated agent.
		loggingConfigUpdaterName: ifNotMigrating(logger.Manifold(logger.ManifoldConfig{
			AgentName:       agentName,
			APICallerName:   apiCallerName,
			UpdateAgentFunc: config.UpdateLoggerConfig,
		})),

		// The diskmanager worker periodically lists block devices on the
		// machine it runs on. This worker will be run on all Juju-managed
		// machines (one per machine agent).
		diskManagerName: ifNotMigrating(diskmanager.Manifold(diskmanager.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
		})),

		// The proxy config updater is a leaf worker that sets http/https/apt/etc
		// proxy settings.
		proxyConfigUpdater: ifNotMigrating(proxyupdater.Manifold(proxyupdater.ManifoldConfig{
			AgentName:       agentName,
			APICallerName:   apiCallerName,
			Logger:          loggo.GetLogger("juju.worker.proxyupdater"),
			WorkerFunc:      proxyupdater.NewWorker,
			ExternalUpdate:  externalUpdateProxyFunc,
			InProcessUpdate: proxyconfig.DefaultConfig.Set,
			RunFunc:         proxyupdater.RunWithStdIn,
		})),

		// The api address updater is a leaf worker that rewrites agent config
		// as the state server addresses change. We should only need one of
		// these in a consolidated agent.
		apiAddressUpdaterName: ifNotMigrating(apiaddressupdater.Manifold(apiaddressupdater.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
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

		// The deployer worker is primary for deploying and recalling unit
		// agents, according to changes in a set of state units; and for the
		// final removal of its agents' units from state when they are no
		// longer needed.
		deployerName: ifNotMigrating(deployer.Manifold(deployer.ManifoldConfig{
			NewDeployContext: config.NewDeployContext,
			AgentName:        agentName,
			APICallerName:    apiCallerName,
		})),

		authenticationWorkerName: ifNotMigrating(authenticationworker.Manifold(authenticationworker.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
		})),

		// The storageProvisioner worker manages provisioning
		// (deprovisioning), and attachment (detachment) of first-class
		// volumes and filesystems.
		storageProvisionerName: ifNotMigrating(ifCredentialValid(storageprovisioner.MachineManifold(storageprovisioner.MachineManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			Clock:         config.Clock,
			NewCredentialValidatorFacade: common.NewCredentialInvalidatorFacade,
		}))),

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

		toolsVersionCheckerName: ifNotMigrating(toolsversionchecker.Manifold(toolsversionchecker.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
		})),

		machineActionName: ifNotMigrating(machineactions.Manifold(machineactions.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			NewFacade:     machineactions.NewFacade,
			NewWorker:     machineactions.NewMachineActionsWorker,
		})),

		hostKeyReporterName: ifNotMigrating(hostkeyreporter.Manifold(hostkeyreporter.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			RootDir:       config.RootDir,
			NewFacade:     hostkeyreporter.NewFacade,
			NewWorker:     hostkeyreporter.NewWorker,
		})),

		externalControllerUpdaterName: ifNotMigrating(ifPrimaryController(externalcontrollerupdater.Manifold(
			externalcontrollerupdater.ManifoldConfig{
				APICallerName:                      apiCallerName,
				NewExternalControllerWatcherClient: newExternalControllerWatcherClient,
			},
		))),

		logPrunerName: ifNotMigrating(ifPrimaryController(dblogpruner.Manifold(
			dblogpruner.ManifoldConfig{
				ClockName:     clockName,
				StateName:     stateName,
				PruneInterval: config.LogPruneInterval,
				NewWorker:     dblogpruner.NewWorker,
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

		httpServerName: httpserver.Manifold(httpserver.ManifoldConfig{
			AgentName:             agentName,
			CertWatcherName:       certificateWatcherName,
			ClockName:             clockName,
			StateName:             stateName,
			PrometheusRegisterer:  config.PrometheusRegisterer,
			NewTLSConfig:          httpserver.NewTLSConfig,
			NewStateAuthenticator: httpserver.NewStateAuthenticator,
			NewWorker:             httpserver.NewWorkerShim,
		}),

		apiServerName: apiserver.Manifold(apiserver.ManifoldConfig{
			AgentName:                         agentName,
			AuthenticatorName:                 httpServerName,
			ClockName:                         clockName,
			StateName:                         stateName,
			MuxName:                           httpServerName,
			UpgradeGateName:                   upgradeStepsGateName,
			RestoreStatusName:                 restoreWatcherName,
			AuditConfigUpdaterName:            auditConfigUpdaterName,
			PrometheusRegisterer:              config.PrometheusRegisterer,
			RegisterIntrospectionHTTPHandlers: config.RegisterIntrospectionHTTPHandlers,
			Hub:       config.CentralHub,
			Presence:  config.PresenceRecorder,
			NewWorker: apiserver.NewWorker,
		}),

		modelWorkerManagerName: ifFullyUpgraded(modelworkermanager.Manifold(modelworkermanager.ManifoldConfig{
			StateName:      stateName,
			NewWorker:      modelworkermanager.New,
			NewModelWorker: config.NewModelWorker,
		})),

		peergrouperName: ifFullyUpgraded(peergrouper.Manifold(peergrouper.ManifoldConfig{
			AgentName:                agentName,
			ClockName:                clockName,
			StateName:                stateName,
			Hub:                      config.CentralHub,
			NewWorker:                peergrouper.New,
			ControllerSupportsSpaces: config.ControllerSupportsSpaces,
		})),

		restoreWatcherName: restorewatcher.Manifold(restorewatcher.ManifoldConfig{
			StateName: stateName,
			NewWorker: restorewatcher.NewWorker,
		}),

		certificateUpdaterName: ifFullyUpgraded(certupdater.Manifold(certupdater.ManifoldConfig{
			AgentName:                agentName,
			StateName:                stateName,
			NewWorker:                certupdater.NewCertificateUpdater,
			NewMachineAddressWatcher: certupdater.NewMachineAddressWatcher,
		})),

		auditConfigUpdaterName: ifController(auditconfigupdater.Manifold(auditconfigupdater.ManifoldConfig{
			AgentName: agentName,
			StateName: stateName,
			NewWorker: auditconfigupdater.New,
		})),

		raftEnabledName: ifController(featureflag.Manifold(featureflag.ManifoldConfig{
			StateName: stateName,
			FlagName:  feature.DisableRaft,
			Invert:    true,
			Logger:    loggo.GetLogger("juju.worker.raft.raftenabled"),
			NewWorker: featureflag.NewWorker,
		})),

		// All the other raft workers hang off the raft transport, so
		// it's the only one that needs to be gated by the enabled flag.
		raftTransportName: ifRaftEnabled(ifFullyUpgraded(rafttransport.Manifold(rafttransport.ManifoldConfig{
			ClockName:         clockName,
			AgentName:         agentName,
			AuthenticatorName: httpServerName,
			HubName:           centralHubName,
			MuxName:           httpServerName,
			DialConn:          rafttransport.DialConn,
			NewWorker:         rafttransport.NewWorker,
			Path:              "/raft",
		}))),

		raftName: raft.Manifold(raft.ManifoldConfig{
			ClockName:     clockName,
			AgentName:     agentName,
			TransportName: raftTransportName,
			FSM:           &raft.SimpleFSM{},
			Logger:        loggo.GetLogger("juju.worker.raft"),
			NewWorker:     raft.NewWorker,
		}),

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

		validCredentialFlagName: credentialvalidator.Manifold(credentialvalidator.ManifoldConfig{
			APICallerName: apiCallerName,
			NewFacade:     credentialvalidator.NewFacade,
			NewWorker:     credentialvalidator.NewWorker,
		}),

		// TODO: (hml) 2018-07-17
		// Remove when upgrade-series feature flag removed.
		//upgradeSeriesEnabledName: featureflag.Manifold(featureflag.ManifoldConfig{
		//	StateName: stateName,
		//	FlagName:  feature.UpgradeSeries,
		//	Invert:    false,
		//	Logger:    loggo.GetLogger("juju.worker.upgradeseries.enabled"),
		//	NewWorker: featureflag.NewWorker,
		//}),
	}
	//  is there an ifNotCAAS?
	if utilsfeatureflag.Enabled(feature.UpgradeSeries) {
		manifolds[upgradeSeriesWorkerName] = ifNotMigrating(upgradeseries.Manifold(upgradeseries.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			Logger:        loggo.GetLogger("juju.worker.upgradeseries"),
			NewFacade:     upgradeseries.NewFacade,
			NewWorker:     upgradeseries.NewWorker,
		}))
	}

	return manifolds
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

var ifRaftEnabled = engine.Housing{
	Flags: []string{
		raftEnabledName,
	},
}.Decorate

var ifUpgradeSeriesEnabled = engine.Housing{
	Flags: []string{
		upgradeSeriesEnabledName,
	},
}.Decorate

var ifCredentialValid = engine.Housing{
	Flags: []string{
		validCredentialFlagName,
	},
}.Decorate

const (
	agentName              = "agent"
	terminationName        = "termination-signal-handler"
	stateConfigWatcherName = "state-config-watcher"
	controllerName         = "controller"
	stateName              = "state"
	apiCallerName          = "api-caller"
	apiConfigWatcherName   = "api-config-watcher"
	centralHubName         = "central-hub"
	presenceName           = "presence"
	pubSubName             = "pubsub-forwarder"
	clockName              = "clock"

	upgraderName         = "upgrader"
	upgradeStepsName     = "upgrade-steps-runner"
	upgradeStepsGateName = "upgrade-steps-gate"
	upgradeStepsFlagName = "upgrade-steps-flag"
	upgradeCheckGateName = "upgrade-check-gate"
	upgradeCheckFlagName = "upgrade-check-flag"

	migrationFortressName     = "migration-fortress"
	migrationInactiveFlagName = "migration-inactive-flag"
	migrationMinionName       = "migration-minion"

	servingInfoSetterName         = "serving-info-setter"
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
	isPrimaryControllerFlagName   = "is-primary-controller-flag"
	isControllerFlagName          = "is-controller-flag"
	logPrunerName                 = "log-pruner"
	txnPrunerName                 = "transaction-pruner"
	certificateWatcherName        = "certificate-watcher"
	modelWorkerManagerName        = "model-worker-manager"
	peergrouperName               = "peer-grouper"
	restoreWatcherName            = "restore-watcher"
	certificateUpdaterName        = "certificate-updater"
	auditConfigUpdaterName        = "audit-config-updater"

	upgradeSeriesEnabledName = "upgrade-series-enabled"
	upgradeSeriesWorkerName  = "upgrade-series"

	httpServerName = "http-server"
	apiServerName  = "api-server"

	raftTransportName = "raft-transport"
	raftName          = "raft"
	raftClustererName = "raft-clusterer"
	raftFlagName      = "raft-leader-flag"
	raftEnabledName   = "raft-enabled-flag"
	raftBackstopName  = "raft-backstop"

	validCredentialFlagName = "valid-credential-flag"
)
