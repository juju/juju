// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor3628 returns upgrade steps for Juju 3.6.28 that manipulate
// state directly.
func stateStepsFor3628() []Step {
	return []Step{
		&upgradeStep{
			description: "drop unused ssh proxy collections and cleanup docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().DropSSHProxyCollections()
			},
		},
		&upgradeStep{
			description: "remove orphaned ssh proxy controller config keys",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().RemoveSSHProxyControllerConfig()
			},
		},
	}
}
