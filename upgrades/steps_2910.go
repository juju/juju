// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor2910 returns upgrade steps for juju 2.9.10
func stateStepsFor2910() []Step {
	return []Step{
		&upgradeStep{
			description: `transform empty manifests to nil`,
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().TransformEmptyManifestsToNil()
			},
		},
	}
}
