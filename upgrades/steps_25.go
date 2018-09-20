// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor25 returns upgrade steps for Juju 2.5.0 that manipulate state directly.
func stateStepsFor25() []Step {
	return []Step{
		&upgradeStep{
			description: `migrate storage records to use "hostid" field`,
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().MigrateStorageMachineIdFields()
			},
		},
		&upgradeStep{
			description: "migrate legacy leases into raft",
			targets:     []Target{Controller},
			run:         MigrateLegacyLeases,
		},
		&upgradeStep{
			description: "migrate add-model permissions",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().MigrateAddModelPermissions()
			},
		},
	}
}
