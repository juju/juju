// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor281 returns upgrade steps for Juju 2.8.1.
func stateStepsFor281() []Step {
	return []Step{
		// This step occurs for the 2.8.0 upgrade, but is repeated here due to
		// now-fixed issues that could have unset address origin since.
		&upgradeStep{
			description: "add origin to IP addresses",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddOriginToIPAddresses()
			},
		},
		&upgradeStep{
			description: `remove "unsupported" link-layer device data`,
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().RemoveUnsupportedLinkLayer()
			},
		},
		&upgradeStep{
			description: `add bakery config`,
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddBakeryConfig()
			},
		},
		&upgradeStep{
			description: `update status documents to remove neverset`,
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().ReplaceNeverSetWithUnset()
			},
		},
	}
}
