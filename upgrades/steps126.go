// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
)

// stepsFor126 returns upgrade steps for Juju 1.26.
func stepsFor126() []Step {
	return []Step{}
}

// stateStepsFor126 returns upgrade steps for Juju 1.26 that manipulate state directly.
func stateStepsFor126() []Step {
	return []Step{
		&upgradeStep{
			description: "add the version field to all settings docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.MigrateSettingsSchema(context.State())
			},
		},
		&upgradeStep{
			description: "add status to filesystem",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddFilesystemStatus(context.State())
			},
		},
		&upgradeStep{
			description: "upgrade environment config",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				// TODO(axw) updateEnvironConfig should be
				// called for all upgrades, to decouple this
				// package from provider-specific upgrades.
				st := context.State()
				return upgradeEnvironConfig(st, st, environs.GlobalProviderRegistry())
			},
		},
		&upgradeStep{
			description: "update machine preferred addresses",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddPreferredAddressesToMachines(context.State())
			},
		},
		&upgradeStep{
			description: "add default endpoint bindings to services",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddDefaultEndpointBindingsToServices(context.State())
			},
		},
	}
}
