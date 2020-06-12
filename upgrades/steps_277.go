// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor277 returns upgrade steps for Juju 2.7.7.
func stateStepsFor277() []Step {
	return []Step{
		// Re-run the same step from 2.7.0 upgrade to pick up a fix in the upgrade step.
		&upgradeStep{
			description: "replace space name in endpointBindingDoc bindings with an space ID",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().ReplaceSpaceNameWithIDEndpointBindings()
			},
		},
	}
}
