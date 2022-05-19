// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor2930 returns database upgrade steps for Juju 2.9.30
func stateStepsFor2930() []Step {
	return []Step{
		&upgradeStep{
			description: "remove channels from local charm origins",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().RemoveLocalCharmOriginChannels()
			},
		},
	}
}
