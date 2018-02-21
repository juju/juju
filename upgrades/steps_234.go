// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor234 returns upgrade steps for Juju 2.3.4 that manipulate state directly.
func stateStepsFor234() []Step {
	return []Step{
		&upgradeStep{
			description: "delete cloud image metadata cache",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().DeleteCloudImageMetadata()
			},
		},
	}
}
