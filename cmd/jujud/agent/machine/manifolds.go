// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/errors"
	"github.com/juju/utils/voyeur"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apideployer "github.com/juju/juju/api/deployer"
	"github.com/juju/juju/cmd/jujud/agent/common"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/apiaddressupdater"
	"github.com/juju/juju/worker/apicaller"
	"github.com/juju/juju/worker/apiconfigwatcher"
	"github.com/juju/juju/worker/authenticationworker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/deployer"
	"github.com/juju/juju/worker/diskmanager"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/identityfilewriter"
	"github.com/juju/juju/worker/logger"
	"github.com/juju/juju/worker/logsender"
	"github.com/juju/juju/worker/machiner"
	"github.com/juju/juju/worker/migrationflag"
	"github.com/juju/juju/worker/migrationminion"
	"github.com/juju/juju/worker/proxyupdater"
	"github.com/juju/juju/worker/reboot"
	"github.com/juju/juju/worker/resumer"
	workerstate "github.com/juju/juju/worker/state"
	"github.com/juju/juju/worker/stateconfigwatcher"
	"github.com/juju/juju/worker/storageprovisioner"
	"github.com/juju/juju/worker/terminationworker"
	"github.com/juju/juju/worker/toolsversionchecker"
	"github.com/juju/juju/worker/upgrader"
	"github.com/juju/juju/worker/upgradesteps"
	"github.com/juju/utils/clock"
	"github.com/juju/version"
)

// ManifoldsConfig allows specialisation of the result of Manifolds.
type ManifoldsConfig struct {
	// Agent contains the agent that will be wrapped and made available to
	// its dependencies via a dependency.Engine.
	Agent coreagent.Agent

	// AgentConfigChanged is set whenever the machine agent's config
	// is updated.
	AgentConfigChanged *voyeur.Value

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

	// OpenState is function used by the state manifold to create a
	// *state.State.
	OpenState func(coreagent.Config) (*state.State, error)

	// OpenStateForUpgrade is a function the upgradesteps worker can
	// use to establish a connection to state.
	OpenStateForUpgrade func() (*state.State, error)

	// StartStateWorkers is function called by the stateworkers
	// manifold to start workers which rely on a *state.State but
	// which haven't been converted to run directly under the
	// dependency engine yet. This will go once these workers have
	// been converted.
	StartStateWorkers func(*state.State) (worker.Worker, error)

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

	// Clock is used by the storageprovisioner worker.
	Clock clock.Clock
}

// Manifolds returns a set of co-configured manifolds covering the
// various responsibilities of a machine agent.
//
// Thou Shalt Not Use String Literals In This Function. Or Else.
func Manifolds(config ManifoldsConfig) dependency.Manifolds {

	// connectFilter exists:
	//  1) to let us retry api connections immeduately on password change,
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
			return worker.ErrTerminateAgent
		} else if cause == apicaller.ErrChangedPassword {
			return dependency.ErrBounce
		}
		return err
	}

	return dependency.Manifolds{
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

		// The stateconfigwatcher manifold watches the machine agent's
		// configuration and reports if state serving info is
		// present. It will bounce itself if state serving info is
		// added or removed. It is intended as a dependency just for
		// the state manifold.
		stateConfigWatcherName: stateconfigwatcher.Manifold(stateconfigwatcher.ManifoldConfig{
			AgentName:          agentName,
			AgentConfigChanged: config.AgentConfigChanged,
		}),

		// The state manifold creates a *state.State and makes it
		// available to other manifolds. It pings the mongodb session
		// regularly and will die if pings fail.
		stateName: workerstate.Manifold(workerstate.ManifoldConfig{
			AgentName:              agentName,
			StateConfigWatcherName: stateConfigWatcherName,
			OpenState:              config.OpenState,
		}),

		// The stateworkers manifold starts workers which rely on a
		// *state.State but which haven't been converted to run
		// directly under the dependency engine yet. This manifold
		// will be removed once all such workers have been converted;
		// until then, the workers are expected to handle their own
		// checks for upgrades etc, rather than blocking this whole
		// worker on upgrade completion.
		stateWorkersName: StateWorkersManifold(StateWorkersConfig{
			StateName:         stateName,
			StartStateWorkers: config.StartStateWorkers,
		}),

		// The api-config-watcher manifold monitors the API server
		// addresses in the agent config and bounces when they
		// change. It's required as part of model migrations.
		apiConfigWatcherName: apiconfigwatcher.Manifold(apiconfigwatcher.ManifoldConfig{
			AgentName:          agentName,
			AgentConfigChanged: config.AgentConfigChanged,
		}),

		// The api caller is a thin concurrent wrapper around a connection
		// to some API server. It's used by many other manifolds, which all
		// select their own desired facades. It will be interesting to see
		// how this works when we consolidate the agents; might be best to
		// handle the auth changes server-side..?
		apiCallerName: apicaller.Manifold(apicaller.ManifoldConfig{
			AgentName:            agentName,
			APIConfigWatcherName: apiConfigWatcherName,
			APIOpen:              apicaller.APIOpen,
			NewConnection:        apicaller.ScaryConnect,
			Filter:               connectFilter,
		}),

		// The upgrade steps gate is used to coordinate workers
		// which shouldn't do anything until the upgrade-steps
		// worker has finished running any required upgrade
		// steps. The flag of similar name is used to implement
		// the isUpgraded func that keeps upgrade concerns out
		// of unrelated manifolds.
		upgradeStepsGateName: gate.ManifoldEx(config.UpgradeStepsLock),
		upgradeStepsFlagName: gate.FlagManifold(gate.FlagManifoldConfig{
			GateName:  upgradeStepsGateName,
			NewWorker: gate.NewFlagWorker,
		}),

		// The upgrade check gate is used to coordinate workers which
		// shouldn't do anything until the upgrader worker has
		// completed its first check for a new tools version to
		// upgrade to. The flag of similar name is used to implement
		// the isUpgraded func that keeps upgrade concerns out of
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
		}),

		// The log sender is a leaf worker that sends log
		// messages to some API server, when configured so to
		// do. We should only need one of these in a
		// consolidated agent.
		//
		// NOTE: the LogSource will buffer a large number of
		// messages as an upgrade runs; it currently seems
		// better to fill the buffer and send when stable,
		// optimising for stable controller upgrades rather than
		// up-to-the-moment observable normal-machine upgrades.
		logSenderName: ifUpgraded(logsender.Manifold(logsender.ManifoldConfig{
			APICallerName: apiCallerName,
			LogSource:     config.LogSource,
		})),

		// The migration fortress synchronises the migration
		// minion worker (as a Guard) with all workers that
		// might interfere with migration (basically, all other
		// workers). We defer creation of this manifold past
		// upgrade completion, so that all following workers can
		// depend on the migration fortress and not worry about
		// upgrades at all.
		migrationFortressName: ifUpgraded(fortress.Manifold()),

		// The migration minion handles the agent-side aspects
		// of model migrations.
		migrationMinionName: migrationminion.Manifold(migrationminion.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			FortressName:  migrationFortressName,
		}),

		// The migration-inactive flag is used to signal to the
		// bulk of manifolds that they are allowed to run and
		// should attempt to do so.
		migrationInactiveFlagName: migrationflag.Manifold(migrationflag.ManifoldConfig{
			APICallerName: apiCallerName,
			Check:         migrationflag.IsNone,
			NewFacade:     migrationflag.NewFacade,
			NewWorker:     migrationflag.NewWorker,
		}),

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
		})),

		// The logging config updater is a leaf worker that indirectly
		// controls the messages sent via the log sender or rsyslog,
		// according to changes in environment config. We should only need
		// one of these in a consolidated agent.
		loggingConfigUpdaterName: ifNotMigrating(logger.Manifold(logger.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
		})),

		// The diskmanager worker periodically lists block devices on the
		// machine it runs on. This worker will be run on all Juju-managed
		// machines (one per machine agent).
		diskmanagerName: ifNotMigrating(diskmanager.Manifold(diskmanager.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
		})),

		// The proxy config updater is a leaf worker that sets http/https/apt/etc
		// proxy settings.
		proxyConfigUpdater: ifNotMigrating(proxyupdater.Manifold(proxyupdater.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
		})),

		// The api address updater is a leaf worker that rewrites agent config
		// as the state server addresses change. We should only need one of
		// these in a consolidated agent.
		apiAddressUpdaterName: ifNotMigrating(apiaddressupdater.Manifold(apiaddressupdater.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
		})),

		// The machiner Worker will wait for the identified machine to become
		// Dying and make it Dead; or until the machine becomes Dead by other
		// means.
		machinerName: ifNotMigrating(machiner.Manifold(machiner.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
		})),

		// The deployer worker is responsible for deploying and recalling unit
		// agents, according to changes in a set of state units; and for the
		// final removal of its agents' units from state when they are no
		// longer needed.
		deployerName: ifNotMigrating(deployer.Manifold(deployer.ManifoldConfig{
			NewDeployContext: config.NewDeployContext,
			AgentName:        agentName,
			APICallerName:    apiCallerName,
		})),

		authenticationworkerName: ifNotMigrating(authenticationworker.Manifold(authenticationworker.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
		})),

		// The storageProvisioner worker manages provisioning
		// (deprovisioning), and attachment (detachment) of first-class
		// volumes and filesystems.
		storageprovisionerName: ifNotMigrating(storageprovisioner.MachineManifold(storageprovisioner.MachineManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			Clock:         config.Clock,
		})),

		resumerName: ifNotMigrating(resumer.Manifold(resumer.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
		})),

		identityFileWriterName: ifNotMigrating(identityfilewriter.Manifold(identityfilewriter.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
		})),

		toolsversioncheckerName: ifNotMigrating(toolsversionchecker.Manifold(toolsversionchecker.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
		})),
	}
}

var (
	ifUpgraded = common.Spec{
		Flags: []string{
			upgradeStepsFlagName,
			upgradeCheckFlagName,
		},
	}.Decorate

	ifNotMigrating = common.Spec{
		Flags: []string{
			migrationInactiveFlagName,
		},
		Occupy: migrationFortressName,
	}.Decorate
)

const (
	agentName              = "agent"
	terminationName        = "termination"
	stateConfigWatcherName = "state-config-watcher"
	stateName              = "state"
	stateWorkersName       = "stateworkers"
	apiCallerName          = "api-caller"
	upgradeStepsGateName   = "upgrade-steps-gate"
	upgradeStepsFlagName   = "upgrade-steps-flag"
	upgradeCheckGateName   = "upgrade-check-gate"
	upgradeCheckFlagName   = "upgrade-check-flag"
	upgraderName           = "upgrader"
	upgradeStepsName       = "upgradesteps"

	migrationFortressName     = "migration-fortress"
	migrationMinionName       = "migration-minion"
	migrationInactiveFlagName = "migration-inactive-flag"

	servingInfoSetterName    = "serving-info-setter"
	apiWorkersName           = "apiworkers"
	rebootName               = "reboot"
	loggingConfigUpdaterName = "logging-config-updater"
	diskmanagerName          = "disk-manager"
	proxyConfigUpdater       = "proxy-config-updater"
	apiAddressUpdaterName    = "api-address-updater"
	machinerName             = "machiner"
	logSenderName            = "log-sender"
	deployerName             = "deployer"
	authenticationworkerName = "authenticationworker"
	storageprovisionerName   = "storage-provisioner-machine"
	resumerName              = "resumer"
	identityFileWriterName   = "identity-file-writer"
	toolsversioncheckerName  = "tools-version-checker"
	apiConfigWatcherName     = "api-config-watcher"
)
