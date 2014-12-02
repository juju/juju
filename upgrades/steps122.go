// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/juju/state"
)

// stateStepsFor122 returns upgrade steps form Juju 1.22 that manipulate state directly.
func stateStepsFor122() []Step {
	return []Step{
		&upgradeStep{
			description: "prepend the environment UUID to the ID of all settings docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddEnvUUIDToSettings(context.State())
			},
		},
		&upgradeStep{
			description: "prepend the environment UUID to the ID of all settingsRefs docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddEnvUUIDToSettingsRefs(context.State())
			},
		},
		&upgradeStep{
			description: "prepend the environment UUID to the ID of all networks docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddEnvUUIDToNetworks(context.State())
			},
		},
		&upgradeStep{
			description: "prepend the environment UUID to the ID of all requestedNetworks docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddEnvUUIDToRequestedNetworks(context.State())
			},
		},
		&upgradeStep{
			description: "prepend the environment UUID to the ID of all networkInterfaces docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddEnvUUIDToNetworkInterfaces(context.State())
			},
		}, &upgradeStep{
			description: "prepend the environment UUID to the ID of all statuses docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddEnvUUIDToStatuses(context.State())
			},
		}, &upgradeStep{
			description: "prepend the environment UUID to the ID of all annotations docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddEnvUUIDToAnnotations(context.State())
			},
		}, &upgradeStep{
			description: "prepend the environment UUID to the ID of all constraints docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddEnvUUIDToConstraints(context.State())
			},
		}, &upgradeStep{
			description: "update system identity in state",
			targets:     []Target{DatabaseMaster},
			run:         ensureSystemSSHKeyRedux,
		},
	}
}

// stepsFor122 returns upgrade steps form Juju 1.22 that only need the API.
func stepsFor122() []Step {
	return []Step{
		&upgradeStep{
			description: "update the authorized keys for the system identity",
			targets:     []Target{DatabaseMaster},
			run:         updateAuthorizedKeysForSystemIdentity,
		},
	}
}
