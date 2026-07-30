// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor3628 returns upgrade steps for Juju 3.6.28 that manipulate
// state directly.
func stateStepsFor3628() []Step {
	return []Step{
		&upgradeStep{
			description: "drop unused virtual host keys collection",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().DropVirtualHostKeysCollection()
			},
		},
	}
}
