// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor2932 returns database upgrade steps for Juju 2.9.32
func stateStepsFor2932() []Step {
	return []Step{
		&upgradeStep{
			description: "add last poll time to charmhub resources",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().FixCharmhubLastPolltime()
			},
		},
	}
}
