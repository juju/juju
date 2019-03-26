// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor254 returns the upgrade steps for Juju 2.5.4 that manipulates
// state directly.
func stateStepsFor254() []Step {
	return []Step{
		&upgradeStep{
			description: "ensure default modification status is set for machines",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().EnsureDefaultModificationStatus()
			},
		},
		&upgradeStep{
			description: "ensure device constraints exists for applications",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().EnsureApplicationDeviceConstraints()
			},
		},
	}
}
