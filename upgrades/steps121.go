// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/juju/state"
)

// stepsFor121 returns upgrade steps to upgrade to a Juju 1.21 deployment.
func stepsFor121() []Step {
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
			description: "migrate tools into environment storage",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return migrateToolsStorage(context.State(), context.AgentConfig())
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
