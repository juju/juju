// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils/series"

	"github.com/juju/juju/service"
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
		&upgradeStep{
			description: "add cloud model counts",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddCloudModelCounts()
			},
		},
		&upgradeStep{
			description: "bootstrap raft cluster",
			targets:     []Target{Controller},
			run:         BootstrapRaft,
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

// install the service files in Standard location - '/lib/systemd/system path.
func installServiceFile(context Context) error {
	hostSeries, err := series.HostSeries()
	if err == nil {
		initName, err := service.VersionInitSystem(hostSeries)
		if err != nil {
			logger.Errorf("unsuccessful writing the service files in /lib/systemd/system path")
			return err
		} else {
			if initName == service.InitSystemSystemd {
				oldDataDir := context.AgentConfig().DataDir()
				oldInitDataDir := filepath.Join(oldDataDir, "init")

				sysdManager := service.NewServiceManagerWithDefaults()
				err = sysdManager.WriteServiceFiles()
				if err != nil {
					logger.Errorf("unsuccessful writing the service files in /lib/systemd/system path")
					return err
				}
				// Cleanup the old dir - /var/lib/init
				return os.RemoveAll(oldInitDataDir)
			} else {
				logger.Infof("upgrade to systemd possible only for 'xenial' and above")
				return nil
			}
		}
	} else {
		return errors.Trace(err)
	}
}
