// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor2939 returns database upgrade steps for Juju 2.9.39
func stateStepsFor2939() []Step {
	return []Step{
		&upgradeStep{
			description: "correct stored durations in controller config",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().CorrectControllerConfigDurations()
			},
		},
	}
}
