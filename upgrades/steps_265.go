// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor265 returns upgrade steps for Juju 2.6.5.
func stateStepsFor265() []Step {
	return []Step{
		&upgradeStep{
			description: "add models-logs-size to controller config",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddModelLogsSize()
			},
		},
	}
}
