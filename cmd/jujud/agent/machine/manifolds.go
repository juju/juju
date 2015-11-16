// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/apicaller"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/gate2"
	"github.com/juju/juju/worker/terminationworker"
)

// ManifoldsConfig allows specialisation of the result of Manifolds.
type ManifoldsConfig struct {

	// Agent contains the agent that will be wrapped and made available to
	// its dependencies via a dependency.Engine.
	Agent coreagent.Agent
}

// Manifolds returns a set of co-configured manifolds covering the
// various responsibilities of a machine agent.
//
// Thou Shalt Not Use String Literals In This Function. Or Else.
func Manifolds(config ManifoldsConfig) dependency.Manifolds {

	upgradesDoneManifold, upgradesDoneUnlocker := gate2.Manifold()

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
			AgentName:       agentName,
			APIInfoGateName: apiInfoGateName,
		}),

		// This manifold is used to coordinate between the api caller and the
		// log sender, which share the API credentials that the API caller may
		// update. To avoid surprising races, the log sender waits for the api
		// caller to unblock this, indicating that any password dance has been
		// completed and the log-sender can now connect without confusion.
		apiInfoGateName: gate.Manifold(),

		upgradesDoneName: upgradesDoneManifold,

		// The gate unlocker gets passed directly to the manifold that unlocks it.
		upgradeStepsName: upgradeSteps.Manifold(upgradeSteps.ManifoldConfig{
			AgentName:            agentName,
			APiCallerName:        apiCallerName,
			UpgradesDoneUnlocker: upgradesDoneUnlocker,
		},

		// Some worker which needs to wait until upgrades are done
		// gets told the name of the gate2 instance. In it's start
		// func it would pull the gate2.Checker from the upgrades-done
		// manifold and call IsUnlocked on it, exiting if it's not
		// unlocked yet. It'll get restarted by the dependency engine
		// once the gate is unlocked.
		someWorker: someWorker.Manifold(someWorker.ManifoldConfig{
			...,
			UpgradesDoneName: upgradesDoneName,
		},
	}
}

const (
	agentName        = "agent"
	terminationName  = "termination"
	apiCallerName    = "api-caller"
	apiInfoGateName  = "api-info-gate"
	upgradesDoneName = "upgrades-done"
)







