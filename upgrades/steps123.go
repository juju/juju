// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

// stateStepsFor123 returns upgrade steps form Juju 1.23 that manipulate state directly.
func stateStepsFor123() []Step {
	return []Step{}
}

// stepsFor123 returns upgrade steps form Juju 1.23 that only need the API.
func stepsFor123() []Step {
	return []Step{
		&upgradeStep{
			description: "add environment UUID to agent config",
			targets:     []Target{AllMachines},
			run:         addEnvironmentUUIDToAgentConfig,
		},
	}
}
