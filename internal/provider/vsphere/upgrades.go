// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"path"

	"github.com/juju/errors"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/tags"
)

// UpgradeOperations is part of the upgrades.OperationSource interface.
func (env *environ) UpgradeOperations(ctx envcontext.ProviderCallContext, args environs.UpgradeOperationsParams) []environs.UpgradeOperation {
	return []environs.UpgradeOperation{{
		TargetVersion: providerVersion1,
		Steps: []environs.UpgradeStep{
			extraConfigUpgradeStep{env, args.ControllerUUID},
			modelFoldersUpgradeStep{env, args.ControllerUUID},
		},
	}}
}

// extraConfigUpgradeStep moves top-level VMs into a model-specific
// VM folder.
type extraConfigUpgradeStep struct {
	env            *environ
	controllerUUID string
}

// Description is part of the environs.UpgradeStep interface.
func (extraConfigUpgradeStep) Description() string {
	return "Update ExtraConfig properties with standard Juju tags"
}

// Run is part of the environs.UpgradeStep interface.
func (step extraConfigUpgradeStep) Run(ctx envcontext.ProviderCallContext) error {
	const (
		legacyControllerTag   = "juju_controller_uuid_key"
		legacyIsControllerTag = "juju_is_controller_key"
	)
	return step.env.withSession(ctx, func(env *sessionEnviron) error {
		vms, err := env.client.VirtualMachines(env.ctx, env.namespace.Prefix()+"*")
		if err != nil || len(vms) == 0 {
			return env.handleCredentialError(ctx, err)
		}
		for _, vm := range vms {
			update := false
			isController := false
			for _, opt := range vm.Config.ExtraConfig {
				value := opt.GetOptionValue()
				switch value.Key {
				case legacyControllerTag:
					update = true
				case legacyIsControllerTag:
					isController = true
				}
			}
			if !update {
				continue
			}
			metadata := map[string]string{
				tags.JujuController: step.controllerUUID,
				tags.JujuModel:      env.Config().UUID(),
			}
			if isController {
				metadata[tags.JujuIsController] = "true"
			}
			if err := env.client.UpdateVirtualMachineExtraConfig(
				env.ctx, vm, metadata,
			); err != nil {
				return errors.Annotatef(env.handleCredentialError(ctx, err), "updating VM %s", vm.Name)
			}
		}
		return nil
	})
}

// modelFoldersUpgradeStep moves top-level VMs into a model-specific
// VM folder.
type modelFoldersUpgradeStep struct {
	env            *environ
	controllerUUID string
}

// Description is part of the environs.UpgradeStep interface.
func (modelFoldersUpgradeStep) Description() string {
	return "Move VMs into controller/model folders"
}

// Run is part of the environs.UpgradeStep interface.
func (step modelFoldersUpgradeStep) Run(ctx envcontext.ProviderCallContext) error {
	return step.env.withSession(ctx, func(env *sessionEnviron) error {
		// We must create the folder even if there are no VMs in the model.
		modelFolderPath := path.Join(env.getVMFolder(), controllerFolderName(step.controllerUUID), env.modelFolderName())

		// EnsureVMFolder needs credential attributes to be defined separately
		// from the folders it is supposed to create
		if _, err := env.client.EnsureVMFolder(
			env.ctx,
			env.getVMFolder(),
			path.Join(controllerFolderName(step.controllerUUID), env.modelFolderName()),
		); err != nil {
			return errors.Annotate(env.handleCredentialError(ctx, err), "creating model folder")
		}

		// List all instances at the top level with the model UUID,
		// and move them into the folder.
		vms, err := env.client.VirtualMachines(env.ctx, env.namespace.Prefix()+"*")
		if err != nil || len(vms) == 0 {
			return env.handleCredentialError(ctx, err)
		}
		refs := make([]types.ManagedObjectReference, len(vms))
		for i, vm := range vms {
			logger.Debugf(ctx, "moving VM %q into %q", vm.Name, modelFolderPath)
			refs[i] = vm.Reference()
		}
		if err := env.client.MoveVMsInto(env.ctx, modelFolderPath, refs...); err != nil {
			return errors.Annotate(env.handleCredentialError(ctx, err), "moving VMs into model folder")
		}
		return nil
	})
}
