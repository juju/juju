// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor366 returns upgrade steps for Juju 3.6.6 that manipulate state directly.
func stateStepsFor366() []Step {
	return []Step{
		&upgradeStep{
			description: "add ssh jump host key",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddJumpHostKey()
			},
		},
	}
}
