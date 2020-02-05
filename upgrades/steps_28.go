// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor272 returns upgrade steps for Juju 2.8.0.
func stateStepsFor28() []Step {
	return []Step{
		&upgradeStep{
			description: "increment tasks sequence by 1",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().IncrementTasksSequence()
			},
		},
	}
}
