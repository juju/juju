// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor286 returns database upgrade steps for Juju 2.8.6.
func stateStepsFor286() []Step {
	return []Step{
		// Prior versions of Juju could end up in a state with link-layer
		// device provider IDs in the global collection that were not assigned
		// to a device.
		// This prevents the newer corrected logic for assigning these IDs,
		// because Juju thinks they are already in use.
		&upgradeStep{
			description: "remove unused link-layer device provider IDs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().RemoveUnusedLinkLayerDeviceProviderIDs()
			},
		},
	}
}
