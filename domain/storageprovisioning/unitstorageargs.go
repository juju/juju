// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioning

import (
	"context"
	"slices"

	domainapplicationcharm "github.com/juju/juju/domain/application/charm"
	domainnetwork "github.com/juju/juju/domain/network"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/errors"
	internalstorage "github.com/juju/juju/internal/storage"
)

// StorageDirective defines a storage directive that already exists for either
// an application or unit.
type StorageDirective struct {
	// CharmMetadataName is the metadata name of the charm the directive exists
	// for.
	CharmMetadataName string

	// Count represents the number of storage instances that should be made for
	// this directive. This value should be the desired count but not the limit.
	// For the maximum supported limit see [StorageDirective.MaxCount].
	Count uint32

	// CharmStorageType represents the storage type of the charm that the
	// directive relates to.
	CharmStorageType domainapplicationcharm.StorageType

	// MaxCount represents the maximum number of storage instances that can be
	// made for this directive. If [domainapplicationcharm.StorageNoMaxCount] is
	// the value, it means that no maximum exists for the storage directive.
	MaxCount int

	// Name relates to the charm storage name definition and must match up.
	Name domainstorage.Name

	// PoolUUID defines the storage pool uuid to use for the directive.
	PoolUUID domainstorage.StoragePoolUUID

	// Size defines the size of the storage directive in MiB.
	Size uint64
}

// StorageKindFromCharmStorageType provides a mapping from charm storage
// type to storage kind.
func StorageKindFromCharmStorageType(
	storageType domainapplicationcharm.StorageType,
) (domainstorage.StorageKind, error) {
	switch storageType {
	case domainapplicationcharm.StorageBlock:
		return domainstorage.StorageKindBlock, nil
	case domainapplicationcharm.StorageFilesystem:
		return domainstorage.StorageKindFilesystem, nil
	default:
		return -1, errors.Errorf(
			"no mapping exists from charm storage type %q to storage kind",
			storageType,
		)
	}
}

// MakeNewUnitStorageArgs returns the unit storage arguments and IAAS unit
// storage arguments required to provision storage for a brand new unit of an
// application with the given storage directives. The unit's storage is
// attached to the net node of the machine hosting the unit.
//
// This is used by the relation domain, which creates subordinate units when a
// unit enters scope of a container scoped relation, and by the application
// domain for its own units. A brand new unit has no existing storage instances
// or attachments to reuse, so the existing-storage arguments are always nil.
func MakeNewUnitStorageArgs(
	ctx context.Context,
	poolProvider StoragePoolProvider,
	attachNetNodeUUID domainnetwork.NetNodeUUID,
	storageDirectives []StorageDirective,
) (domainstorage.CreateUnitStorageArg, domainstorage.CreateIAASUnitStorageArg, error) {
	unitStorageArgs, err := makeNewUnitStorageArgs(ctx, poolProvider, attachNetNodeUUID, storageDirectives)
	if err != nil {
		return domainstorage.CreateUnitStorageArg{},
			domainstorage.CreateIAASUnitStorageArg{}, err
	}

	iaasUnitStorageArgs, err := MakeIAASUnitStorageArgs(ctx, unitStorageArgs.StorageInstances)
	if err != nil {
		return domainstorage.CreateUnitStorageArg{},
			domainstorage.CreateIAASUnitStorageArg{}, err
	}

	return unitStorageArgs, iaasUnitStorageArgs, nil
}

// makeNewUnitStorageArgs makes the unit storage arguments for a brand new
// unit from the given storage directives. All the new instances are attached
// to the supplied net node, and the unit owns all of them.
//
// The accumulators are initialised to nil, so that a unit without storage
// directives gets zero-value storage arguments: no storage is provisioned,
// and the arguments compare equal to the zero value of
// [domainstorage.CreateUnitStorageArg].
func makeNewUnitStorageArgs(
	ctx context.Context,
	poolProvider StoragePoolProvider,
	attachNetNodeUUID domainnetwork.NetNodeUUID,
	storageDirectives []StorageDirective,
) (domainstorage.CreateUnitStorageArg, error) {
	var (
		rvalDirectives []domainstorage.DirectiveArg
		rvalInstances  []domainstorage.CreateUnitStorageInstanceArg
		rvalToAttach   []domainstorage.CreateUnitStorageAttachmentArg
		// rvalToOwn is the list of storage instance uuid's that the unit
		// must own.
		rvalToOwn []domainstorage.StorageInstanceUUID
	)

	// We create a cached storage pool provider for the scope of this operation.
	// This exists to reduce load on the controller potentially requesting the
	// same storage pool provider over and over again.
	storagePoolProvider := CachedStoragePoolProvider{
		Cache:               map[domainstorage.StoragePoolUUID]internalstorage.Provider{},
		StoragePoolProvider: poolProvider,
	}

	for _, sd := range storageDirectives {
		// Make the storage directive arg first. This MUST happen as the count
		// value in [sd] is about to be modified.
		rvalDirectives = append(rvalDirectives, domainstorage.DirectiveArg{
			Count:    sd.Count,
			Name:     sd.Name,
			PoolUUID: sd.PoolUUID,
			Size:     sd.Size,
		})

		instArgs, err := MakeUnitStorageInstancesFromDirective(
			ctx,
			sd.Count,
			storagePoolProvider,
			sd,
		)
		if err != nil {
			return domainstorage.CreateUnitStorageArg{}, errors.Errorf(
				"making new storage %q instance args: %w", sd.Name, err,
			)
		}

		// Allocate capacity we know we are going to need.
		rvalToAttach = slices.Grow(rvalToAttach, len(instArgs))
		rvalInstances = slices.Grow(rvalInstances, len(instArgs))
		rvalToOwn = slices.Grow(rvalToOwn, len(instArgs))
		for _, inst := range instArgs {
			storageAttachArg, err := MakeStorageAttachmentArgFromCreateStorageInstance(
				attachNetNodeUUID, inst,
			)

			if err != nil {
				return domainstorage.CreateUnitStorageArg{}, errors.Errorf(
					"making storage attachment arguments for new storage instance: %w", err,
				)
			}

			rvalToOwn = append(rvalToOwn, inst.UUID)
			rvalToAttach = append(rvalToAttach, storageAttachArg)
			rvalInstances = append(rvalInstances, inst)
		}
	}

	return domainstorage.CreateUnitStorageArg{
		StorageDirectives: rvalDirectives,
		StorageInstances:  rvalInstances,
		StorageToAttach:   rvalToAttach,
		StorageToOwn:      rvalToOwn,
	}, nil
}

// MakeUnitStorageInstancesFromDirective is responsible for taking a storage
// directive and creating a set of storage instance args that are capable of
// fulfilling the requirements of the directive.
// The directive provides storage defaults including count, but here the
// caller is specifying the actual count to use.
func MakeUnitStorageInstancesFromDirective(
	ctx context.Context,
	count uint32,
	storagePoolProvider StoragePoolProvider,
	directive StorageDirective,
) ([]domainstorage.CreateUnitStorageInstanceArg, error) {
	// Early exit if no storage instances are to be created. Save's a lot of
	// busy work that goes unused.
	if count == 0 {
		return nil, nil
	}

	storageKind, err := StorageKindFromCharmStorageType(directive.CharmStorageType)
	if err != nil {
		return nil, errors.Capture(err)
	}

	provider, err := storagePoolProvider.GetProviderForPool(
		ctx, directive.PoolUUID,
	)
	if err != nil {
		return nil, errors.Errorf(
			"getting storage provider for storage directive pool %q: %w",
			directive.PoolUUID, err,
		)
	}

	composition, err := CalculateStorageInstanceComposition(
		storageKind, provider,
	)
	if err != nil {
		return nil, errors.Errorf(
			"calculating storage entity composition for directive: %w", err,
		)
	}

	rval := make([]domainstorage.CreateUnitStorageInstanceArg, 0, count)
	for range count {
		uuid, err := domainstorage.NewStorageInstanceUUID()
		if err != nil {
			return nil, errors.Errorf(
				"new storage instance uuid: %w", err,
			)
		}

		instArg := domainstorage.CreateUnitStorageInstanceArg{
			CharmName:       directive.CharmMetadataName,
			Kind:            storageKind,
			Name:            directive.Name,
			RequestSizeMiB:  directive.Size,
			StoragePoolUUID: directive.PoolUUID,
			UUID:            uuid,
		}

		if composition.FilesystemRequired {
			u, err := domainstorage.NewFilesystemUUID()
			if err != nil {
				return nil, errors.Errorf(
					"generating new storage filesystem uuid: %w", err,
				)
			}

			instArg.Filesystem = &domainstorage.CreateUnitStorageFilesystemArg{
				UUID:           u,
				ProvisionScope: composition.FilesystemProvisionScope,
			}
		}

		if composition.VolumeRequired {
			u, err := domainstorage.NewVolumeUUID()
			if err != nil {
				return nil, errors.Errorf(
					"generating new storage volume uuid: %w", err,
				)
			}

			instArg.Volume = &domainstorage.CreateUnitStorageVolumeArg{
				UUID:           u,
				ProvisionScope: composition.VolumeProvisionScope,
			}
		}

		rval = append(rval, instArg)
	}

	return rval, nil
}

// MakeStorageAttachmentArgFromCreateStorageInstance builds the attachment
// arguments for a newly created storage instance.
func MakeStorageAttachmentArgFromCreateStorageInstance(
	netNodeUUID domainnetwork.NetNodeUUID,
	storageInstance domainstorage.CreateUnitStorageInstanceArg,
) (domainstorage.CreateUnitStorageAttachmentArg, error) {
	uuid, err := domainstorage.NewStorageAttachmentUUID()
	if err != nil {
		return domainstorage.CreateUnitStorageAttachmentArg{}, errors.Errorf(
			"generating new storage attachment uuid: %w", err,
		)
	}

	rval := domainstorage.CreateUnitStorageAttachmentArg{
		StorageInstanceUUID: storageInstance.UUID,
		UUID:                uuid,
	}

	if storageInstance.Filesystem != nil {
		uuid, err := domainstorage.NewFilesystemAttachmentUUID()
		if err != nil {
			return domainstorage.CreateUnitStorageAttachmentArg{}, errors.Errorf(
				"generating new filesystem attachment uuid: %w", err,
			)
		}

		rval.FilesystemAttachment = &domainstorage.CreateUnitStorageFilesystemAttachmentArg{
			FilesystemUUID: storageInstance.Filesystem.UUID,
			NetNodeUUID:    netNodeUUID,
			ProvisionScope: storageInstance.Filesystem.ProvisionScope,
			UUID:           uuid,
		}
	}

	if storageInstance.Volume != nil {
		uuid, err := domainstorage.NewVolumeAttachmentUUID()
		if err != nil {
			return domainstorage.CreateUnitStorageAttachmentArg{}, errors.Errorf(
				"generating new volume attachment uuid: %w", err,
			)
		}

		rval.VolumeAttachment = &domainstorage.CreateUnitStorageVolumeAttachmentArg{
			VolumeUUID:     storageInstance.Volume.UUID,
			NetNodeUUID:    netNodeUUID,
			ProvisionScope: storageInstance.Volume.ProvisionScope,
			UUID:           uuid,
		}
	}

	return rval, nil
}

// MakeIAASUnitStorageArgs returns [domainstorage.CreateIAASUnitStorageArg] that
// complement the unit storage arguments provided for IAAS units.
func MakeIAASUnitStorageArgs(
	_ context.Context,
	storageInst []domainstorage.CreateUnitStorageInstanceArg,
) (domainstorage.CreateIAASUnitStorageArg, error) {
	var arg domainstorage.CreateIAASUnitStorageArg
	for _, v := range storageInst {
		// TODO(storage): refactor this to use the storage instance composition
		// calculated from the storageprovisioning domain.
		var comp StorageInstanceComposition
		if v.Filesystem != nil {
			comp.FilesystemRequired = true
			comp.FilesystemProvisionScope = v.Filesystem.ProvisionScope
		}
		if v.Volume != nil {
			comp.VolumeRequired = true
			comp.VolumeProvisionScope = v.Volume.ProvisionScope
		}
		s, err := CalculateStorageInstanceOwnershipScope(
			comp)
		if err != nil {
			return domainstorage.CreateIAASUnitStorageArg{}, errors.Errorf(
				"calculating storage ownership for storage instance %q: %w",
				v.UUID, err,
			)
		}
		if s != OwnershipScopeMachine {
			continue
		}
		if v.Filesystem != nil {
			arg.FilesystemsToOwn = append(arg.FilesystemsToOwn,
				v.Filesystem.UUID)
		}
		if v.Volume != nil {
			arg.VolumesToOwn = append(arg.VolumesToOwn,
				v.Volume.UUID)
		}
	}
	return arg, nil
}
