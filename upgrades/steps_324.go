// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

func stepsFor324() []Step {
	return []Step{}
}

func stateStepsFor324() []Step {
	return []Step{
		&upgradeStep{
			description: "ensure application charm origins have revisions",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().EnsureApplicationCharmOriginsNormalised()
			},
		},
	}
}
