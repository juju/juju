// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor221 returns upgrade steps for Juju 2.2.1 that manipulate state directly.
func stateStepsFor221() []Step {
	return []Step{
		&upgradeStep{
			description: "add update-status hook config settings",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddUpdateStatusHookSettings()
			},
		},
		&upgradeStep{
			description: "correct relation unit counts for subordinates",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().CorrectRelationUnitCounts()
			},
		},
	}
}
