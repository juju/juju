// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor237 returns upgrade steps for Juju 2.3.7 that manipulate state directly.
func stateStepsFor237() []Step {
	return []Step{
		&upgradeStep{
			description: "ensure container-image-stream isn't set in applications",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().RemoveContainerImageStreamFromNonModelSettings()
			},
		},
	}
}
