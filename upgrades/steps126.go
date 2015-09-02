// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/names"
)

// stepsFor126 returns upgrade steps for Juju 1.26.
func stepsFor126() []Step {
	return []Step{
		&upgradeStep{
			description: "installed boolean needs to be set in the uniter local state",
			targets:     []Target{AllMachines},
			run: func(context Context) error {
				config := context.AgentConfig()
				tag, ok := config.Tag().(names.UnitTag)
				if !ok {
					// not a Unit; skipping
					return nil
				}
				return uniter.AddInstalledToUniterState(tag, config.DataDir())
			},
		},
	}
}

// stateStepsFor126 returns upgrade steps for Juju 1.26 that manipulate state directly.
func stateStepsFor126() []Step {
	return []Step{
		&upgradeStep{
			description: "add status to filesystem",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddFilesystemStatus(context.State())
			}},
	}
}
