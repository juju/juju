// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor23 returns upgrade steps for Juju 2.3.0 that manipulate state directly.
func stateStepsFor23() []Step {
	return []Step{
		&upgradeStep{
			description: "add a 'type' field to model documents",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddModelType()
			},
		},
	}
}
