// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor2920 returns upgrade steps for juju 2.9.20.
func stateStepsFor2920() []Step {
	return []Step{
		&upgradeStep{
			description: `clean up assignUnits for dead and removed applications`,
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().CleanupDeadAssignUnits()
			},
		},
	}
}
