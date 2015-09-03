// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import "github.com/juju/juju/state"

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
