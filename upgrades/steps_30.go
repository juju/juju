// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

func stepsFor30() []Step {
	return []Step{}
}

func stateStepsFor30() []Step {
	return []Step{}
}

func stateStepsForSSTXN() []Step {
	return []Step{
		&upgradeStep{
			description: "migrate txns.log from capped collection",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().MigrateCappedTxnsLogCollection()
			},
		},
	}
}
