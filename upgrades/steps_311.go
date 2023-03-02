// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

func stepsFor311() []Step {
	return []Step{}
}

func stateStepsFor311() []Step {
	return []Step{
		&upgradeStep{
			description: "remove orphaned secret permissions",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().RemoveOrphanedSecretPermissions()
			},
		},
		&upgradeStep{
			description: "copies the opened ports for an application for all its units",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().MigrateApplicationOpenedPortsToUnitScope()
			},
		},
	}
}
