// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor2933 returns database upgrade steps for Juju 2.9.33
func stateStepsFor2933() []Step {
	return []Step{
		&upgradeStep{
			description: "remove use-floating-ip=false from model config",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().RemoveUseFloatingIPConfigFalse()
			},
		},
	}
}
