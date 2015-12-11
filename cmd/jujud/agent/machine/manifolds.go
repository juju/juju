// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/state"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/apicaller"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/terminationworker"
	"github.com/juju/juju/worker/upgrader"
	"github.com/juju/juju/worker/upgradesteps"
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
	}
}

const (
	agentName             = "agent"
	terminationName       = "termination"
	apiCallerName         = "api-caller"
	apiInfoGateName       = "api-info-gate"
	upgradeStepsGateName  = "upgrade-steps-gate"
	upgradeCheckGateName  = "upgrade-check-gate"
	upgraderName          = "upgrader"
	upgradeStepsName      = "upgradesteps"
	uninstallerName       = "uninstaller"
	servingInfoSetterName = "serving-info-setter"
)
