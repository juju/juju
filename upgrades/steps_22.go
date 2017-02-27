// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor22 returns upgrade steps for Juju 2.2 that manipulate state directly.
func stateStepsFor22() []Step {
	return []Step{
		&upgradeStep{
			description: "add machineid to non-detachable storage docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddNonDetachableStorageMachineId()
			},
		},
	}
}
