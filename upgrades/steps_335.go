// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

func stateStepsFor335() []Step {
	return []Step{
		&upgradeStep{
			description: "assign architectures to container's instance data",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AssignArchToContainers()
			},
		},
	}
}
