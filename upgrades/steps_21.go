// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor21 returns upgrade steps for Juju 2.1 that manipulate state directly.
func stateStepsFor21() []Step {
	return []Step{
		&upgradeStep{
			description: "add attempt to migration docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddMigrationAttempt()
			},
		},
		&upgradeStep{
			description: "add sequences to track used local charm revisions",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddLocalCharmSequences()
			},
		},
		&upgradeStep{
			description: "update lxd cloud/credentials",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return updateLXDCloudCredentials(context.State())
			},
		},
	}
}
