// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor3614 returns upgrade steps for Juju 3.6.14 that manipulate state directly.
func stateStepsFor3614() []Step {
	return []Step{
		&upgradeStep{
			description: "convert scaling field to enum",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().ConvertScalingToCurrentOperationEnumField()
			},
		},
	}
}
