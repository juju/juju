// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor2925 returns database upgrade steps for Juju 2.9.25
func stateStepsFor2925() []Step {
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
