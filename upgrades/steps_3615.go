// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor3615 returns upgrade steps for Juju 3.6.15 that manipulate state directly.
func stateStepsFor3615() []Step {
	return []Step{
		&upgradeStep{
			description: "open controller api port in state",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().OpenControllerAPIPort()
			},
		},
	}
}
