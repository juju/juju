// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor3612 returns upgrade steps for Juju 3.6.12 that manipulate state directly.
func stateStepsFor3612() []Step {
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
