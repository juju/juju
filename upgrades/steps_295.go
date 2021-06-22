// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor295 returns upgrade steps for juju 2.9.5
func stateStepsFor295() []Step {
	return []Step{
		&upgradeStep{
			description: `change "dynamic" link-layer address configs to "dhcp"`,
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().UpdateDHCPAddressConfigs()
			},
		},
	}
}
