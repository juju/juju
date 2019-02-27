// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor253 returns upgrade steps for Juju 2.5.3 that manipulate state directly.
func stateStepsFor253() []Step {
	return []Step{
		&upgradeStep{
			description: "update inherited controller config global key",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().UpdateInheritedControllerConfig()
			},
		},
	}
}
