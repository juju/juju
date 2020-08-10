// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor282 returns upgrade steps for Juju 2.8.2.
func stateStepsFor282() []Step {
	return []Step{
		// The charm metadata parser used in juju 2.7 (and before)
		// would inject a limit of 1 for each requirer/peer endpoint in
		// the charm metadata when no limit was specified. The limit
		// was ignored prior to juju 2.8 so this upgrade step allows us
		// to reset the limit to prevent errors when attempting to add
		// new relations.
		//
		// Fixes LP1887095.
		&upgradeStep{
			description: "reset default limit to 0 for existing charm metadata",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().ResetDefaultRelationLimitInCharmMetadata()
			},
		},
	}
}
