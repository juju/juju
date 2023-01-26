// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor303 returns database upgrade steps for Juju 3.0.3
func stateStepsFor303() []Step {
	return []Step{
		&upgradeStep{
			description: "add charm-origin id and hash where missing, if charm already downloaded",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().CorrectCharmOriginsMultiAppSingleCharm()
			},
		},
	}
}
