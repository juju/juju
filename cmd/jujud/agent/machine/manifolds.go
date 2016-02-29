// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apideployer "github.com/juju/juju/api/deployer"
	"github.com/juju/juju/state"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/apiaddressupdater"
	"github.com/juju/juju/worker/apicaller"
	"github.com/juju/juju/worker/authenticationworker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/deployer"
	"github.com/juju/juju/worker/diskmanager"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/identityfilewriter"
	"github.com/juju/juju/worker/logger"
	"github.com/juju/juju/worker/logsender"
	"github.com/juju/juju/worker/machiner"
	"github.com/juju/juju/worker/proxyupdater"
	"github.com/juju/juju/worker/reboot"
	"github.com/juju/juju/worker/resumer"
	"github.com/juju/juju/worker/storageprovisioner"
	"github.com/juju/juju/worker/terminationworker"
	"github.com/juju/juju/worker/upgrader"
	"github.com/juju/juju/worker/upgradesteps"
	"github.com/juju/juju/worker/upgradewaiter"
	"github.com/juju/juju/worker/util"
	"github.com/juju/utils/clock"
)

// ManifoldsConfig allows specialisation of the result of Manifolds.
type ManifoldsConfig struct {
	// Agent contains the agent that will be wrapped and made available to
	// its dependencies via a dependency.Engine.
	Agent coreagent.Agent

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

	// OpenStateForUpgrade is a function the upgradesteps worker can
	// use to establish a connection to state.
	OpenStateForUpgrade func() (*state.State, func(), error)

	// WriteUninstallFile is a function the uninstaller manifold uses
	// to write the agent uninstall file.
	WriteUninstallFile func() error

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
	return dependency.Manifolds{
		// The agent manifold references the enclosing agent, and is the
		// foundation stone on which most other manifolds ultimately depend.
		agentName: agent.Manifold(config.Agent),

		// The termination worker returns ErrTerminateAgent if a
		// termination signal is received by the process it's running
		// in. It has no inputs and its only output is the error it
		// returns.
		terminationName: terminationworker.Manifold(),

		// The api caller is a thin concurrent wrapper around a connection
		// to some API server. It's used by many other manifolds, which all
		// select their own desired facades. It will be interesting to see
		// how this works when we consolidate the agents; might be best to
		// handle the auth changes server-side..?
		apiCallerName: apicaller.Manifold(apicaller.ManifoldConfig{
			AgentName: agentName,
		}),

		// The upgrade steps gate is used to coordinate workers which
		// shouldn't do anything until the upgrade-steps worker has
		// finished running any required upgrade steps.
		upgradeStepsGateName: gate.ManifoldEx(config.UpgradeStepsLock),

		// The upgrade check gate is used to coordinate workers which
		// shouldn't do anything until the upgrader worker has
		// completed it's first check for a new tools version to
		// upgrade to.
		upgradeCheckGateName: gate.ManifoldEx(config.UpgradeCheckLock),

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

		// The uninstaller manifold checks if the machine is dead. If
		// it is it writes the agent uninstall file and returns
		// ErrTerminateAgent which causes the agent to remove itself.
		uninstallerName: uninstallerManifold(uninstallerManifoldConfig{
			AgentName:          agentName,
			APICallerName:      apiCallerName,
			WriteUninstallFile: config.WriteUninstallFile,
		}),

		// The serving-info-setter manifold sets grabs the state
		// serving info from the API connection and writes it to the
		// agent config.
		servingInfoSetterName: ServingInfoSetterManifold(ServingInfoSetterConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
		}),

		// The upgradewaiter manifold aggregates the
		// upgrade-steps-gate and upgrade-check-gate manifolds into
		// one boolean output. It makes it easy to create manifolds
		// which must only run after these upgrade events have
		// occured.
		upgradeWaiterName: upgradewaiter.Manifold(upgradewaiter.ManifoldConfig{
			UpgradeStepsWaiterName: upgradeStepsGateName,
			UpgradeCheckWaiterName: upgradeCheckGateName,
		}),

		// The apiworkers manifold starts workers which rely on the
		// machine agent's API connection but have not been converted
		// to work directly under the dependency engine. It waits for
		// upgrades to be finished before starting these workers.
		apiWorkersName: APIWorkersManifold(APIWorkersConfig{
			APICallerName:     apiCallerName,
			UpgradeWaiterName: upgradeWaiterName,
			StartAPIWorkers:   config.StartAPIWorkers,
		}),

		// The reboot manifold manages a worker which will reboot the
		// machine when requested. It needs an API connection and
		// waits for upgrades to be complete.
		rebootName: reboot.Manifold(reboot.ManifoldConfig{
			AgentName:         agentName,
			APICallerName:     apiCallerName,
			UpgradeWaiterName: upgradeWaiterName,
		}),

		// The logging config updater is a leaf worker that indirectly
		// controls the messages sent via the log sender or rsyslog,
		// according to changes in environment config. We should only need
		// one of these in a consolidated agent.
		loggingConfigUpdaterName: logger.Manifold(logger.ManifoldConfig{
			AgentName:         agentName,
			APICallerName:     apiCallerName,
			UpgradeWaiterName: upgradeWaiterName,
		}),

		// The diskmanager worker periodically lists block devices on the
		// machine it runs on. This worker will be run on all Juju-managed
		// machines (one per machine agent).
		diskmanagerName: diskmanager.Manifold(diskmanager.ManifoldConfig{
			AgentName:         agentName,
			APICallerName:     apiCallerName,
			UpgradeWaiterName: upgradeWaiterName,
		}),

		// The proxy config updater is a leaf worker that sets http/https/apt/etc
		// proxy settings.
		proxyConfigUpdater: proxyupdater.Manifold(proxyupdater.ManifoldConfig{
			AgentName:         agentName,
			APICallerName:     apiCallerName,
			UpgradeWaiterName: upgradeWaiterName,
		}),

		// The api address updater is a leaf worker that rewrites agent config
		// as the state server addresses change. We should only need one of
		// these in a consolidated agent.
		apiAddressUpdaterName: apiaddressupdater.Manifold(apiaddressupdater.ManifoldConfig{
			AgentName:         agentName,
			APICallerName:     apiCallerName,
			UpgradeWaiterName: upgradeWaiterName,
		}),

		// The machiner Worker will wait for the identified machine to become
		// Dying and make it Dead; or until the machine becomes Dead by other
		// means.
		machinerName: machiner.Manifold(machiner.ManifoldConfig{
			PostUpgradeManifoldConfig: util.PostUpgradeManifoldConfig{
				AgentName:         agentName,
				APICallerName:     apiCallerName,
				UpgradeWaiterName: upgradeWaiterName,
			},
			WriteUninstallFile: config.WriteUninstallFile,
		}),

		// The log sender is a leaf worker that sends log messages to some
		// API server, when configured so to do. We should only need one of
		// these in a consolidated agent.
		logSenderName: logsender.Manifold(logsender.ManifoldConfig{
			LogSource: config.LogSource,
			PostUpgradeManifoldConfig: util.PostUpgradeManifoldConfig{
				AgentName:         agentName,
				APICallerName:     apiCallerName,
				UpgradeWaiterName: upgradeWaiterName,
			},
		}),

		// The deployer worker is responsible for deploying and recalling unit
		// agents, according to changes in a set of state units; and for the
		// final removal of its agents' units from state when they are no
		// longer needed.
		deployerName: deployer.Manifold(deployer.ManifoldConfig{
			NewDeployContext: config.NewDeployContext,
			PostUpgradeManifoldConfig: util.PostUpgradeManifoldConfig{
				AgentName:         agentName,
				APICallerName:     apiCallerName,
				UpgradeWaiterName: upgradeWaiterName,
			},
		}),

		authenticationworkerName: authenticationworker.Manifold(authenticationworker.ManifoldConfig{
			AgentName:         agentName,
			APICallerName:     apiCallerName,
			UpgradeWaiterName: upgradeWaiterName,
		}),

		// The storageProvisioner worker manages provisioning
		// (deprovisioning), and attachment (detachment) of first-class
		// volumes and filesystems.
		storageprovisionerName: storageprovisioner.Manifold(storageprovisioner.ManifoldConfig{
			PostUpgradeManifoldConfig: util.PostUpgradeManifoldConfig{
				AgentName:         agentName,
				APICallerName:     apiCallerName,
				UpgradeWaiterName: upgradeWaiterName},
			Clock: config.Clock,
		}),

		resumerName: resumer.Manifold(resumer.ManifoldConfig{
			AgentName:         agentName,
			APICallerName:     apiCallerName,
			UpgradeWaiterName: upgradeWaiterName,
		}),

		identityFileWriterName: identityfilewriter.Manifold(identityfilewriter.ManifoldConfig{
			AgentName:         agentName,
			APICallerName:     apiCallerName,
			UpgradeWaiterName: upgradeWaiterName,
		}),
	}
}

const (
	agentName                = "agent"
	terminationName          = "termination"
	apiCallerName            = "api-caller"
	apiInfoGateName          = "api-info-gate"
	upgradeStepsGateName     = "upgrade-steps-gate"
	upgradeCheckGateName     = "upgrade-check-gate"
	upgraderName             = "upgrader"
	upgradeStepsName         = "upgradesteps"
	upgradeWaiterName        = "upgradewaiter"
	uninstallerName          = "uninstaller"
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
)
