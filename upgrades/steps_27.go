// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/agent"
	k8sprovider "github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/core/paths"
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
		},
		&upgradeStep{
			description: "ensure stored addresses refer to space by ID, and remove old space name/provider ID",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().ConvertAddressSpaceIDs()
			},
		},
		&upgradeStep{
			description: "replace space name in endpointBindingDoc bindings with an space ID",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().ReplaceSpaceNameWithIDEndpointBindings()
			},
		},
		&upgradeStep{
			description: `ensure model config for default-space is "" if either absent or is set to "_default"`,
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().EnsureDefaultSpaceSetting()
			},
		},
		&upgradeStep{
			description: "remove controller config for max-logs-age and max-logs-size if set",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().RemoveControllerConfigMaxLogAgeAndSize()
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
	}
}

func setJujuFolderPermissionsToAdm(dir string) error {
	wantedOwner, wantedGroup := paths.SyslogUserGroup()

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return errors.Trace(err)
		}
		if info.IsDir() {
			return nil
		}
		if err := paths.SetOwnership(path, wantedOwner, wantedGroup); err != nil {
			return errors.Trace(err)
		}
		if err := os.Chmod(path, paths.LogfilePermission); err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	if err != nil {
		return errors.Trace(err)
	}
	logger.Infof("Successfully changed permissions of dir %q", dir)
	return nil
}

// We rewrite/reset the systemd files and change the existing log file permissions
func resetLogPermissions(context Context) error {
	tag := context.AgentConfig().Tag()
	if tag.Kind() != names.MachineTagKind {
		logger.Infof("skipping agent %q, not a machine", tag.String())
		return nil
	}

	// For now a CAAS cannot be machineTagKind so it will not come as far as here for k8.
	// But to make sure for future refactoring, which are planned, we check here as well.
	if context.AgentConfig().Value(agent.ProviderType) == k8sprovider.CAASProviderType {
		logger.Infof("skipping agent %q, is CAAS", k8sprovider.CAASProviderType)
		return nil
	}

	if err := writeServiceFiles(false)(context); err != nil {
		return errors.Trace(err)
	}

	return errors.Trace(setJujuFolderPermissionsToAdm(context.AgentConfig().LogDir()))
}
