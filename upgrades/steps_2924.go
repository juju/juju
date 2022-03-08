// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor2924 returns database upgrade steps for Juju 2.9.24
func stateStepsFor2924() []Step {
	return []Step{
		&upgradeStep{
			description: "remove invalid charm placeholders",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().RemoveInvalidCharmPlaceholders()
			},
		},
	}
}
