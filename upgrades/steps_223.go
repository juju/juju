// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor223 returns upgrade steps for Juju 2.2.3 that manipulate state directly.
func stateStepsFor223() []Step {
	return []Step{
		&upgradeStep{
			description: "add max-action-age and max-action-size config settings",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddActionPruneSettings()
			},
		},
	}
}
