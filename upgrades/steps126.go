// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/utils"
	"github.com/juju/juju/version"
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
			description: "upgrade model config",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				// TODO(axw) updateModelConfig should be
				// called for all upgrades, to decouple this
				// package from provider-specific upgrades.
				st := context.State()
				return upgradeModelConfig(st, st, environs.GlobalProviderRegistry())
			},
		},
		//TODO(perrito666) make this an unconditional upgrade step.
		// it would be ideal not to have to modify this package whenever we add provider upgrade steps.
		&upgradeStep{
			description: "provider side upgrades",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				st := context.State()
				env, err := utils.GetEnviron(st)
				if err != nil {
					return errors.Annotate(err, "getting provider for upgrade")
				}
				return upgradeProviderChanges(env, st, version.Number{Major: 1, Minor: 26})
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
