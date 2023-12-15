// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

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
