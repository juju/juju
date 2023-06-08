// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

func stepsFor321() []Step {
	return []Step{}
}

func stateStepsFor321() []Step {
	return []Step{
		&upgradeStep{
			description: "ensure initial refCount for external secret backends",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().EnsureInitalRefCountForExternalSecretBackends()
			},
		},
	}
}
