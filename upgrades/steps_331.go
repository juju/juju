// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stepsFor331 is a stub for how to write non-state upgrade tests. These are
// rarely necessary. They have been used in the past for upgrade steps where
// changes to workload machines are necessary. E.g. renaming the directory
// where agent binaries are placed in /var/lib/juju.
func stepsFor331() []Step {
	return []Step{}
}

func stateStepsFor331() []Step {
	return []Step{
		&upgradeStep{
			description: "convert application offer token keys",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().ConvertApplicationOfferTokenKeys()
			},
		},
	}
}
