// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor24 returns upgrade steps for Juju 2.4.0 that manipulate state directly.
func stateStepsFor24() []Step {
	return []Step{
		&upgradeStep{
			description: "move or drop the old audit log collection",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().MoveOldAuditLog()
			},
		},
		&upgradeStep{
			description: "copy controller info Mongo space to controller config HA space if valid",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().CopyMongoSpaceToHASpaceConfig()
			},
		},
	}
}
