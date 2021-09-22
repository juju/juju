// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor2915 returns upgrade steps for juju 2.9.15
func stateStepsFor2915() []Step {
	return []Step{
		&upgradeStep{
			description: `remove orphaned cross model proxies`,
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().RemoveOrphanedCrossModelProxies()
			},
		},
	}
}
