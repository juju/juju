// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor2929 returns database upgrade steps for Juju 2.9.29
func stateStepsFor2929() []Step {
	return []Step{
		&upgradeStep{
			description: "remove controller config for max-logs-age and max-logs-size if set",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddControllerConfigGrpcAPIPorts()
			},
		},
	}
}
