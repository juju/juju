// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor2935 returns database upgrade steps for Juju 2.9.35
func stateStepsFor2935() []Step {
	return []Step{
		&upgradeStep{
			description: "remove default-series value from model-config",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().RemoveDefaultSeriesFromModelConfig()
			},
		},
	}
}
