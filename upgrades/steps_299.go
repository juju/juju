// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor299 returns upgrade steps for juju 2.9.9
func stateStepsFor299() []Step {
	return []Step{
		&upgradeStep{
			description: `add spawned task count to operations`,
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddSpawnedTaskCountToOperations()
			},
		},
	}
}
