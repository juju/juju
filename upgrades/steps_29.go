// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor29 returns upgrade steps for Juju 2.9.0
func stateStepsFor29() []Step {
	return []Step{
		&upgradeStep{
			description: "add charmhub-url to model config",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddCharmhubToModelConfig()
			},
		},
	}
}
