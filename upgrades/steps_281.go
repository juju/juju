// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor281 returns upgrade steps for Juju 2.8.1.
func stateStepsFor281() []Step {
	return []Step{
		&upgradeStep{
			description: `remove "unsupported" link-layer device data`,
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().RemoveUnsupportedLinkLayer()
			},
		},
	}
}
