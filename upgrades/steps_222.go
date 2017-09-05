// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor222 returns upgrade steps for Juju 2.2.2 that manipulate state directly.
func stateStepsFor222() []Step {
	return []Step{
		&upgradeStep{
			description: "add environ-version to model docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddModelEnvironVersion()
			},
		},
	}
}
