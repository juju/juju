// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/tags"
)

// UpgradeOperations is part of the upgrades.OperationSource interface.
func (env *environ) UpgradeOperations(ctx envcontext.ProviderCallContext, args environs.UpgradeOperationsParams) []environs.UpgradeOperation {
	return []environs.UpgradeOperation{{
		providerVersion1,
		[]environs.UpgradeStep{
			diskLabelsUpgradeStep{env, args.ControllerUUID},
		},
	}}
}

// diskLabelsUpgradeStep sets labels on the environ's disks,
// associating them with the model and controller.
type diskLabelsUpgradeStep struct {
	env            *environ
	controllerUUID string
}

// Description is part of the environs.UpgradeStep interface.
func (diskLabelsUpgradeStep) Description() string {
	return "Set disk labels"
}

// Run is part of the environs.UpgradeStep interface.
func (step diskLabelsUpgradeStep) Run(ctx envcontext.ProviderCallContext) error {
	env := step.env
	disks, err := env.gce.Disks()
	if err != nil {
		return env.HandleCredentialError(ctx, err)
	}
	for _, disk := range disks {
		if !isValidVolume(disk.Name) {
			continue
		}
		if disk.Labels[tags.JujuModel] != "" || disk.Labels[tags.JujuController] != "" {
			continue
		}
		if disk.Description != "" && disk.Description != env.uuid {
			continue
		}
		if disk.Labels == nil {
			disk.Labels = make(map[string]string)
		}
		disk.Labels[tags.JujuModel] = env.uuid
		disk.Labels[tags.JujuController] = step.controllerUUID
		if err := env.gce.SetDiskLabels(disk.Zone, disk.Name, disk.LabelFingerprint, disk.Labels); err != nil {
			return errors.Annotatef(env.HandleCredentialError(ctx, err), "cannot set labels on volume %q", disk.Name)
		}
	}
	return nil
}
