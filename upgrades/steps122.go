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
		},
	}
}
