// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor26 returns upgrade steps for Juju 2.6 that manipulate state directly.
func stateStepsFor26() []Step {
	return []Step{
		&upgradeStep{
			description: "update k8s storage config",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().UpdateKubernetesStorageConfig()
			},
		},
		&upgradeStep{
			description: "remove instanceCharmProfileData collection",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().RemoveInstanceCharmProfileDataCollection()
			},
		},
	}
}
