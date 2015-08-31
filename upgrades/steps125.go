// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/state"
	"github.com/juju/juju/version"
	"github.com/juju/utils/exec"
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
		&upgradeStep{
			description: "add missing env-uuid to statuses",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddMissingEnvUUIDOnStatuses(context.State())
			},
		},
		&upgradeStep{
			description: "add attachmentCount to volume",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddVolumeAttachmentCount(context.State())
			}},
		&upgradeStep{
			description: "add attachmentCount to filesystem",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddFilesystemsAttachmentCount(context.State())
			}},
		&upgradeStep{
			description: "add binding to volume",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddBindingToVolumes(context.State())
			}},
		&upgradeStep{
			description: "add binding to filesystem",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddBindingToFilesystems(context.State())
			}},
		&upgradeStep{
			description: "add status to volume",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddVolumeStatus(context.State())
			}},
		&upgradeStep{
			description: "move lastlogin and last connection to their own collections",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.MigrateLastLoginAndLastConnection(context.State())
			}},
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
		&upgradeStep{
			description: "add juju registry key",
			targets:     []Target{HostMachine},
			run:         addJujuRegKey,
		},
	}
}

// removeJujudpass removes a file that is no longer used on versions >1.25
// The Jujud.pass file was created during cloud init before
// so we know it's location for sure in case it exists
func removeJujudpass(context Context) error {
	if version.Current.OS == version.Windows {
		fileLocation := "C:\\Juju\\Jujud.pass"
		if err := osRemove(fileLocation); err != nil {
			// Don't fail the step if we can't get rid of the old files.
			// We don't actually care if they still exist or not.
			logger.Warningf("can't delete old password file %q: %s", fileLocation, err)
		}
	}
	return nil
}

var execRunCommands = exec.RunCommands

// addJujuRegKey tries to create the same key that is now created during cloudinit
// on machines having version 1.25 or up
// Since support for ACL's in golang is quite disastrous at the moment, and they're
// not especially easy to use, this is done using the exact same steps used in cloudinit
func addJujuRegKey(context Context) error {
	if version.Current.OS == version.Windows {
		cmds := cloudconfig.CreateJujuRegistryKeyCmds()
		_, err := execRunCommands(exec.RunParams{
			Commands: strings.Join(cmds, "\n"),
		})
		if err != nil {
			return errors.Annotate(err, "could not create juju registry key")
		}
		logger.Infof("created juju registry key at %s", osenv.JujuRegistryKey)
		return nil
	}
	return nil
}
