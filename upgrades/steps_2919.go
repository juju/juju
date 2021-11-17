// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor2919 returns upgrade steps for juju 2.9.19
func stateStepsFor2919() []Step {
	return []Step{
		&upgradeStep{
			description: `migrate legacy cross model tokens`,
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().MigrateLegacyCrossModelTokens()
			},
		},
	}
}
