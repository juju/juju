// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stepsFor245 returns upgrade steps for Juju 2.4.5
func stepsFor245() []Step {
	return []Step{
		&upgradeStep{
			description: "update exec.start.sh log path if incorrect",
			targets:     []Target{AllMachines},
			run:         writeServiceFiles(false),
		},
	}
}
