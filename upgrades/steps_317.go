// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

func stepsFor317() []Step {
	return []Step{}
}

func stateStepsFor317() []Step {
	return []Step{
		&upgradeStep{
			description: "ensure application charm origins have revisions",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().EnsureApplicationCharmOriginsNormalised()
			},
		}, &upgradeStep{
			description: "fix owner consumed secret info",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().FixOwnerConsumedSecretInfo()
			},
		},
	}
}
