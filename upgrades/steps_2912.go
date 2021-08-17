// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor2912 returns upgrade steps for juju 2.9.12
func stateStepsFor2912() []Step {
	return []Step{
		&upgradeStep{
			description: `ensure correct charm-origin risk`,
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().EnsureCharmOriginRisk()
			},
		},
	}
}
