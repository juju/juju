// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/errors"
	"github.com/juju/juju/environs"
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
			},
		},
		&upgradeStep{
			description: "tag machine instances",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				st := context.State()
				machines, err := st.AllMachines()
				if err != nil {
					return errors.Trace(err)
				}
				cfg, err := st.EnvironConfig()
				if err != nil {
					return errors.Trace(err)
				}
				env, err := environs.New(cfg)
				if err != nil {
					return errors.Trace(err)
				}
				return addInstanceTags(env, machines)
			},
		},
	}
}
