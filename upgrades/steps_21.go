// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/juju/state"
)

// stateStepsFor21 returns upgrade steps for Juju 2.1 that manipulate state directly.
func stateStepsFor21() []Step {
	return []Step{
		&upgradeStep{
			description: "drop old log index",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.DropOldLogIndex(context.State())
			},
		},
	}
}
