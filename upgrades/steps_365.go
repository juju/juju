// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor365 returns upgrade steps for Juju 3.6.5 that manipulate state directly.
func stateStepsFor365() []Step {
	return []Step{
		&upgradeStep{
			description: "split migration status messages",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().SplitMigrationStatusMessages()
			},
		},
	}
}
