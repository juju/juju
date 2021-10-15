// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor2917 returns upgrade steps for juju 2.9.17
func stateStepsFor2917() []Step {
	return []Step{
		&upgradeStep{
			description: `drop assumes keys from charm collection`,
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().DropLegacyAssumesSectionsFromCharmMetadata()
			},
		},
	}
}
