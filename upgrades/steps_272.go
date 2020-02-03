// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor272 returns upgrade steps for Juju 2.7.2.
func stepsFor272() []Step {
	return []Step{
		&upgradeStep{
			description: "ensure systemd files are located under /etc/systemd/system",
			targets:     []Target{AllMachines},
			run:         writeServiceFiles(false),
		},
	}
}
