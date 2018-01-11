// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor23 returns upgrade steps for Juju 2.3.0 that manipulate state directly.
func stateStepsFor23() []Step {
	return []Step{
		&upgradeStep{
			description: "add a 'type' field to model documents",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddModelType()
			},
		},
		&upgradeStep{
			description: "migrate old leases",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().MigrateLeasesToGlobalTime()
			},
		},
	}
}

// stateStepsFor231 returns upgrade steps for Juju 2.3.1 that manipulate state directly.
func stateStepsFor231() []Step {
	return []Step{
		&upgradeStep{
			description: "add status to relations",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddRelationStatus()
			},
		},
	}
}
