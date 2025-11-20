// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor3613 returns upgrade steps for Juju 3.6.13 that manipulate state directly.
func stateStepsFor3613() []Step {
	return []Step{
		&upgradeStep{
			description: "populate application storage unique ID",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().PopulateApplicationStorageUniqueID()
			},
		},
	}
}
