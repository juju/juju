// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor2810 returns database upgrade steps for Juju 2.8.10.
func stateStepsFor2810() []Step {
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
