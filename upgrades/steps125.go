// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/juju/state"
)

// stateStepsFor125 returns upgrade steps for Juju 1.25 that manipulate state directly.
func stateStepsFor125() []Step {
	return []Step{
		&upgradeStep{
			description: "set hosted environment count to number of hosted environments",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.SetHostedEnvironCount(context.State())
			}},
	}
}
