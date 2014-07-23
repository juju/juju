// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stepsFor121 returns upgrade steps to upgrade to a Juju 1.21 deployment.
func stepsFor121() []Step {
	return []Step{
		&upgradeStep{
			description: "rename the user LastConnection field to LastLogin",
			targets:     []Target{DatabaseMaster},
			run:         migrateLastConnectionToLastLogin,
		},
	}
}
