// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
	"github.com/juju/juju/version"
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

// stepsFor125 returns upgrade steps for Juju 1.25 that only need the API.
func stepsFor125() []Step {
	return []Step{
		&upgradeStep{
			description: "remove Jujud.pass file on windows",
			targets:     []Target{HostMachine},
			run:         removeJujudpass,
		},
	}
}

// The Jujud.pass file was created during cloud init before
// so we know it's location for sure in case it exists
func removeJujudpass(context Context) error {
	if version.Current.OS == version.Windows {
		fileLocation := "C:\\Juju\\Jujud.pass"
		if err := osRemove(fileLocation); err != nil {
			// Don't fail the step if we can't get rid of the old files.
			// We don't actually care if they still exist or not.
			logger.Warningf("Can't delete old password file %q: %s", fileLocation, err)
		}
	}
	return nil
}
