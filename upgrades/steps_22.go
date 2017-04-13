// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"os"
	"path/filepath"
)

// stateStepsFor22 returns upgrade steps for Juju 2.2 that manipulate state directly.
func stateStepsFor22() []Step {
	return []Step{
		&upgradeStep{
			description: "add machineid to non-detachable storage docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddNonDetachableStorageMachineId()
			},
		},
		&upgradeStep{
			description: "remove application config settings with nil value",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().RemoveNilValueApplicationSettings()
			},
		},
		&upgradeStep{
			description: "add controller log pruning config settings",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddControllerLogPruneSettings()
			},
		},
	}
}

// stepsFor22 returns upgrade steps for Juju 2.2 that only need the API.
func stepsFor22() []Step {
	return []Step{
		&upgradeStep{
			description: "remove meter status file",
			targets:     []Target{AllMachines},
			run:         removeMeterStatusFile,
		},
	}
}

// removeMeterStatusFile removes the meter status file from the agent data directory.
func removeMeterStatusFile(context Context) error {
	dataDir := context.AgentConfig().DataDir()
	meterStatusFile := filepath.Join(dataDir, "meter-status.yaml")
	return os.RemoveAll(meterStatusFile)
}
