// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor2934 returns database upgrade steps for Juju 2.9.34
func stateStepsFor2934() []Step {
	return []Step{
		&upgradeStep{
			description: "add latest as charm-origin channel track if not specified",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().CharmOriginChannelMustHaveTrack()
			},
		},
	}
}
