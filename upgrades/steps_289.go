// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor289 returns database upgrade steps for Juju 2.8.9.
func stateStepsFor289() []Step {
	return []Step{
		&upgradeStep{
			description: "translate k8s service types",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().TranslateK8sServiceTypes()
			},
		},
	}
}
