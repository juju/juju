// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor232 returns upgrade steps for Juju 2.3.2 that manipulate state directly.
func stateStepsFor232() []Step {
	return []Step{
		&upgradeStep{
			description: "move or drop the old audit log collection",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().MoveOldAuditLog()
			},
		},
	}
}
