// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor236 returns upgrade steps for Juju 2.3.6 that manipulate state directly.
func stateStepsFor236() []Step {
	return []Step{
		&upgradeStep{
			description: "ensure container-image-stream config defaults to released",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().EnsureContainerImageStreamDefault()
			},
		},
	}
}
