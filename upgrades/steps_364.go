// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor364 returns upgrade steps for Juju 3.6.4 that manipulate state directly.
func stateStepsFor364() []Step {
	return []Step{
		&upgradeStep{
			description: "add virtual host keys",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddVirtualHostKeys()
			},
		},
	}
}
