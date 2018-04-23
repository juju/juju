// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils/series"
)
// stateStepsFor24 returns upgrade steps for Juju 2.4.0 that manipulate state directly.
func stateStepsFor24() []Step {
	return []Step{
		&upgradeStep{
			description: "move or drop the old audit log collection",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().MoveOldAuditLog()
			},
		},
		&upgradeStep{
			description: "move controller info Mongo space to controller config HA space if valid",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().MoveMongoSpaceToHASpaceConfig()
			},
		},
		&upgradeStep{
			description: "create empty application settings for all applications",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().CreateMissingApplicationConfig()
			},
		},
		&upgradeStep{
			description: "remove votingmachineids",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().RemoveVotingMachineIds()
			},
		},
	}
}

// stepsFor24 returns upgrade steps for Juju 2.4.
func stepsFor24() []Step {
        return []Step{
                &upgradeStep{
                        description: "Install the service file in Standard location '/lib/systemd'",
                        targets:     []Target{AllMachines},
                        run:         installServiceFile,
                },
        }
}

// install the service files in Standard location - '/lib/systemd/juju-init/.
func installServiceFile(context Context) error {
	hostSeries, err := series.HostSeries()
	if (nil == err) {
		if ("xenial" == hostSeries) {
			oldDataDir := context.AgentConfig().DataDir()
			oldInitDataDir := filepath.Join(oldDataDir, "init")

			err = context.ServiceConfig().WriteServiceFile()
			if ( nil == err ) {
				logger.Infof("Successfully installed the service files in standard /lib/systemd/ locatoin and relinked")
			} else {
				logger.Errorf("Unsuccessfull installing the servie files in /lib/systemd/...")
				return err
			}
			// Cleanup the old dir - /var/lib/init/
			return os.Remove(oldInitDataDir)
		} else {
			logger.Infof("Upgrade to systemd possible only for 'xenial' and above")
			return nil
		}
	} else {
		return errors.Trace(err)
	}
}
