// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor3628 returns upgrade steps for Juju 3.6.28 that manipulate
// state directly.
func stateStepsFor3628() []Step {
	return []Step{
		&upgradeStep{
			description: "remove orphaned ssh proxy collections, cleanup docs and controller config",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().RemoveSSHProxyArtefacts()
			},
		},
	}
}
