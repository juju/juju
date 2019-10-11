// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"

	"github.com/juju/juju/juju/paths"

	"github.com/juju/errors"
	"github.com/juju/juju/service"
	"github.com/juju/utils/series"
	"gopkg.in/juju/names.v3"
)

// stateStepsFor27 returns upgrade steps for Juju 2.7.0.
func stateStepsFor27() []Step {
	return []Step{
		&upgradeStep{
			description: "add controller node docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddControllerNodeDocs()
			},
		},
		&upgradeStep{
			description: "recreate spaces with IDs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddSpaceIdToSpaceDocs()
			},
		},
		&upgradeStep{
			description: "change subnet AvailabilityZone to AvailabilityZones",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().ChangeSubnetAZtoSlice()
			},
		},
		&upgradeStep{
			description: "change subnet SpaceName to SpaceID",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().ChangeSubnetSpaceNameToSpaceID()
			},
		},
		&upgradeStep{
			description: "recreate subnets with IDs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddSubnetIdToSubnetDocs()
			},
		},
		&upgradeStep{
			description: "replace portsDoc.SubnetID as a CIDR with an ID.",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().ReplacePortsDocSubnetIDCIDR()
			},
		},
		&upgradeStep{
			description: "ensure application settings exist for all relations",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().EnsureRelationApplicationSettings()
			},
		}, &upgradeStep{
			description: "add fields to controller config for model logfiles",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddModelLogfileControllerConfig()
			},
		},
	}
}

// stepsFor27 returns upgrade steps for Juju 2.7.
func stepsFor27() []Step {
	return []Step{
		&upgradeStep{
			description: "change owner of unit and machine logs to adm",
			targets:     []Target{AllMachines},
			run:         resetLogPermissions,
		},
		&upgradeStep{
			description: "ensure stored addresses refer to space by ID, and remove old space name/provider ID",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().ConvertAddressSpaceIDs()
			},
		},
	}
}

func setLogOwnerCorrectLogPermissions(filename string) error {
	group, err := user.LookupGroup("adm")
	if err != nil {
		return err
	}
	gid, err := strconv.Atoi(group.Gid)
	if err != nil {
		return err
	}
	usr, err := user.Lookup("syslog")
	if err != nil {
		return err
	}
	uid, err := strconv.Atoi(usr.Uid)
	if err != nil {
		return err
	}
	if err := os.Chmod(filename, 0640); err != nil {
		return err
	}
	return os.Chown(filename, uid, gid)
}

func setFolderPermissionsToAdm(dir string) error {
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if err := setLogOwnerCorrectLogPermissions(info.Name()); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	logger.Infof("Successfully changed permissions of /var/log/juju")
	return nil
}

// This adds upgrade steps, we just rewrite the default values which are set before.
// With this we can make sure that things are changed in one default place
func resetLogPermissions(context Context) error {
	if context.AgentConfig().Tag().Kind() == names.UnitTagKind {
		logger.Infof("skipping units, because machine agents are already writing the files")
		return nil
	}
	if context.AgentConfig().Model().Kind() == names.CAASModelTagKind {
		logger.Infof("skipping CAAS")
		return nil
	}
	hostSeries, isSystemd, err := getCurrentInit()
	if err != nil {
		return err
	}
	if !isSystemd {
		return nil
	}
	sysdManager := service.NewServiceManagerWithDefaults()
	if err = sysdManager.WriteServiceFiles(); err != nil {
		logger.Errorf("unsuccessful writing the service files in /lib/systemd/system path")
		return err
	}
	logDir, err := paths.LogDir(hostSeries)
	if err != nil {
		logger.Errorf("unsuccessful trying to access the logDir")
		return nil
	}
	if err = setFolderPermissionsToAdm(logDir); err != nil {
		logger.Errorf("unsuccessful setting permissions for /var/log/juju/ recursive")
		return err
	}
	logger.Infof("Successfully wrote service files in /lib/systemd/system path")
	return nil
	// TODO: either add here or under service/systemd a function to check whether the agent systemd services are correct or not
}

func getCurrentInit() (string, bool, error) {
	hostSeries, err := series.HostSeries()
	if err != nil {
		return "", false, errors.Trace(err)
	}
	initName, err := service.VersionInitSystem(hostSeries)
	if err != nil {
		logger.Errorf("unsuccessful writing the service files in /lib/systemd/system path because unable to access init system")
		return "", false, err
	}
	if initName == service.InitSystemSystemd {
		return hostSeries, true, nil
	} else {
		logger.Infof("upgrade to systemd possible only systems using systemd, current system is running", initName)
		return "", false, nil
	}
}
