// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/juju/state"
)

// stepsFor121a1 returns upgrade steps to upgrade to a Juju 1.21alpha1 deployment.
func stepsFor121a1() []Step {
	return []Step{
		&upgradeStep{
			description: "rename the user LastConnection field to LastLogin",
			targets:     []Target{DatabaseMaster},
			run:         migrateLastConnectionToLastLogin,
		},
		&upgradeStep{
			description: "add environment uuid to state server doc",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddEnvironmentUUIDToStateServerDoc(context.State())
			},
		},
		&upgradeStep{
			description: "add all users in state as environment users",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddStateUsersAsEnvironUsers(context.State())
			},
		},
		&upgradeStep{
			description: "migrate charm archives into environment storage",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return migrateCharmStorage(context.State(), context.AgentConfig())
			},
		},
		&upgradeStep{
			description: "migrate custom image metadata into environment storage",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return migrateCustomImageMetadata(context.State(), context.AgentConfig())
			},
		},
		&upgradeStep{
			description: "set environment owner and server uuid",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.SetOwnerAndServerUUIDForEnvironment(context.State())
			},
		},
	}
}

// stepsFor121a2 returns upgrade steps to upgrade to a Juju 1.21alpha2 deployment.
func stepsFor121a2() []Step {
	return []Step{
		&upgradeStep{
			description: "prepend the environment UUID to the ID of all service docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddEnvUUIDToServicesID(context.State())
			},
		},
	}
}
