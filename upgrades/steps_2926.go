// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor2926 returns database upgrade steps for Juju 2.9.26
func stateStepsFor2926() []Step {
	return []Step{
		&upgradeStep{
			description: `set container address origins to "machine"`,
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().SetContainerAddressOriginToMachine()
			},
		},
		&upgradeStep{
			description: "update charm origin to facilitate charm refresh after set-series",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().UpdateCharmOriginAfterSetSeries()
			},
		},
	}
}
