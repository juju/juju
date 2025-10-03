// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor3611 returns upgrade steps for Juju 3.6.11 that manipulate state directly.
func stateStepsFor3611() []Step {
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
