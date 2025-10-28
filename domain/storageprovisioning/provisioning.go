// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioning

import (
	domainstorage "github.com/juju/juju/domain/storage"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	"github.com/juju/juju/internal/errors"
	internalstorage "github.com/juju/juju/internal/storage"
)

// StorageInstanceComposition describes the composition of how a storage
// instance should be realised in the model.
type StorageInstanceComposition struct {
	FilesystemProvisionScope ProvisionScope
	FilesystemRequired       bool
	VolumeProvisionScope     ProvisionScope
	VolumeRequired           bool
}

// StorageProvider defines an interface describing the information that is
// required from a storage provider.
type StorageProvider interface {
	// Scope returns the provisioning scope that this provider wishes to operate
	// from.
	Scope() internalstorage.Scope

	// Supports returns true or false if the provider supports the supplied
	// storage kind.
	Supports(internalstorage.StorageKind) bool
}

// CheckStorageProviderSupportsStorageKind is responsible for checking whether a
// storage provider is capable of fulfilling a request to create a given kind of
// storage.
//
// To understand how this would be achieved see
// [CalculateStorageInstanceComposition].
func CheckStorageProviderSupportsStorageKind(
	provider StorageProvider, kind domainstorage.StorageKind,
) bool {
	switch {
	// If the provider supports creating block devices and a block device is
	// requested.
	case kind == domainstorage.StorageKindBlock &&
		provider.Supports(internalstorage.StorageKindBlock):
		return true

	// If the provider supports creating filesystems directly and a filesystem
	// is requested.
	case kind == domainstorage.StorageKindFilesystem &&
		provider.Supports(internalstorage.StorageKindFilesystem):
		return true

	// If the provider supports creating block devices and a filesystem is
	// requested then we can create the filesystem on top of the block device
	// (volume).
	case kind == domainstorage.StorageKindFilesystem &&
		provider.Supports(internalstorage.StorageKindBlock):
		return true

	default:
		return false
	}
}

// CalculateStorageInstanceComposition is responsible for taking a storage kind
// that is to be created and coming up with a composition that can be achieved
// with the current provider.
func CalculateStorageInstanceComposition(
	kind domainstorage.StorageKind,
	provider StorageProvider,
) (StorageInstanceComposition, error) {
	// There exists some edge cases in this func. We purposely don't try and
	// define strong error values. Callers of this function should have already
	// verified that the provider is capable of provisioning the supplied
	// storage kind  for their purpose.
	//
	// This function performs an ugly dance when it comes to filesystems and
	// providers that only support block devices. It is assumed that if a
	// provider only supports block devices then a filesystem is able to be
	// provisioned on top of this block device.
	//
	// TODO (tlm): Re-model storage providers. The better approach to the above
	// would be to have storage providers provide their  full composition to
	// achieve a storage kind.
	var rval StorageInstanceComposition

	var pScope ProvisionScope
	switch s := provider.Scope(); s {
	case internalstorage.ScopeEnviron:
		pScope = ProvisionScopeModel
	case internalstorage.ScopeMachine:
		pScope = ProvisionScopeMachine
	default:
		return StorageInstanceComposition{}, errors.Errorf(
			"unrecognised scope %q from provider",
			s,
		)
	}

	switch {
	// If the provider supports creating block devices and a block device is
	// requested.
	case kind == domainstorage.StorageKindBlock &&
		provider.Supports(internalstorage.StorageKindBlock):
		rval.VolumeRequired = true
		rval.VolumeProvisionScope = pScope

	// If the provider supports creating filesystems directly and a filesystem
	// is requested.
	case kind == domainstorage.StorageKindFilesystem &&
		provider.Supports(internalstorage.StorageKindFilesystem):
		rval.FilesystemRequired = true
		rval.FilesystemProvisionScope = pScope

	// If the provider supports creating block devices and a filesystem is
	// requested then we can create the filesystem on top of the block device
	// (volume).
	case kind == domainstorage.StorageKindFilesystem &&
		provider.Supports(internalstorage.StorageKindBlock):
		// If the provider only supports block devices then we must make sure
		// the provision scope of the filesystem is machine as it has to be made
		// on the machine.
		rval.FilesystemRequired = true
		rval.FilesystemProvisionScope = ProvisionScopeMachine
		rval.VolumeRequired = true
		rval.VolumeProvisionScope = pScope

	default:
		// This should never happen but we cover the case to be safe.
		return StorageInstanceComposition{}, errors.New(
			"provider does not support storage kind",
		)
	}

	return rval, nil
}

// CalculateStorageInstanceOwnershipScope determines if the storage is owned by
// either the model or the machine. This is distinctly separate from, but is
// related to provision scope. Provision scope indicates how a volume or
// filesystem will be provisioned. Ownership scope indicates if the storage
// instance that holds the volume and filesystem, is owned by the model or the
// machine.
func CalculateStorageInstanceOwnershipScope(
	composition StorageInstanceComposition,
) (OwnershipScope, error) {
	var provisionScope ProvisionScope
	if composition.VolumeRequired {
		// Even if the a filesystem is required, if there is a volume, it backs
		// the filesystem, so we use the volumes provisioning scope to calculate
		// ownership scope. This might at some point be not true, if a volume is
		// provisioned by a machine but the provider can allow it to be
		// re-attached to annother machine.
		provisionScope = composition.VolumeProvisionScope
	} else if composition.FilesystemRequired {
		provisionScope = composition.FilesystemProvisionScope
	} else {
		return -1, storageprovisioningerrors.OwershipScopeIncalculable
	}
	switch provisionScope {
	case ProvisionScopeMachine:
		return OwnershipScopeMachine, nil
	case ProvisionScopeModel:
		return OwnershipScopeModel, nil
	default:
		return -1, storageprovisioningerrors.OwershipScopeIncalculable
	}
}
