// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor2918 returns database upgrade steps for Juju 2.9.18
func stateStepsFor2918() []Step {
	return []Step{
		&upgradeStep{
			description: "remove link-layer devices without machines",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().RemoveOrphanedLinkLayerDevices()
			},
		},
		// This is a repetition of the same step run for the 2.8.6 upgrade.
		// It is here due to the fix in 2.8.18 of a bug that was still
		// causing this issue to occur.
		&upgradeStep{
			description: "remove unused link-layer device provider IDs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().RemoveUnusedLinkLayerDeviceProviderIDs()
			},
		},
	}
}
