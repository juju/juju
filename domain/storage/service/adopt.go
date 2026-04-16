// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"strings"

	"github.com/juju/collections/transform"

	coreerrors "github.com/juju/juju/core/errors"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/trace"
	domainstatus "github.com/juju/juju/domain/status"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	domainstorageinternal "github.com/juju/juju/domain/storage/internal"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/errors"
	internalstorage "github.com/juju/juju/internal/storage"
)

// AdoptState defines an interface for interacting with the underlying state.
type AdoptState interface {
	// GetStorageResourceTagInfoForModel retrieves the model based resource tag
	// information for storage entities.
	GetStorageResourceTagInfoForModel(
		ctx context.Context,
		resourceTagModelConfigKey string,
	) (domainstorageprovisioning.ModelResourceTagInfo, error)

	// CreateStorageInstanceWithExistingFilesystem creates a new storage
	// instance, with a filesystem using existing provisioned filesystem
	// details. It returns the new storage ID for the created storage instance.
	CreateStorageInstanceWithExistingFilesystem(
		ctx context.Context,
		args domainstorageinternal.CreateStorageInstanceWithExistingFilesystem,
	) (string, error)

	// CreateStorageInstanceWithExistingVolumeBackedFilesystem creates a new
	// storage instance, with a filesystem and volume using existing provisioned
	// volume details. It returns the new storage ID for the created storage
	// instance.
	CreateStorageInstanceWithExistingVolumeBackedFilesystem(
		ctx context.Context,
		args domainstorageinternal.CreateStorageInstanceWithExistingVolumeBackedFilesystem,
	) (string, error)
}

// AdoptFilesystem adopts a filesystem by invoking the provider of the given
// storage pool to identify the filesystem on the given natural entity specified
// by the provider ID (e.g. a filesystem on a volume or a filesystem directly).
// The result of this call is the name of a new storage instance using the given
// storage name.
// The following errors can be expected:
// - [domainstorageerrors.StoragePoolNotFound] if the specified storage pool
// does not exist.
// - [domainstorageerrors.StorageEntityNotFoundInPool] if the pool name is not
// valid.
// - [domainstorageerrors.InvalidStorageName] if the storage name is not valid.
// - [coreerrors.NotValid] if the storage pool uuid is not valid.
// - [domainstorageerrors.ProviderTypeNotFound] if the storage pool refers to a
// missing storage provider type.
// - [domainstorageerrors.AdoptionNotSupported] if the storage provider referred
// to by the specified storage pool does not support adopting storage entities
// or does not support adopting the specified storage entity.
func (s *StorageService) AdoptFilesystem(
	ctx context.Context,
	storageName domainstorage.Name,
	poolUUID domainstorage.StoragePoolUUID,
	providerID string,
	force bool,
) (corestorage.ID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	err := storageName.Validate()
	if err != nil {
		return "", errors.New(
			"invalid storage name",
		).Add(domainstorageerrors.InvalidStorageName)
	}
	err = poolUUID.Validate()
	if err != nil {
		return "", errors.Errorf(
			"invalid storage pool uuid: %w", err,
		).Add(coreerrors.NotValid)
	}
	if providerID == "" {
		return "", errors.New(
			"provider id cannot be empty",
		).Add(coreerrors.NotValid)
	}

	pool, err := s.st.GetStoragePool(ctx, poolUUID)
	if errors.Is(err, domainstorageerrors.StoragePoolNotFound) {
		return "", errors.New(
			"storage pool not found",
		).Add(domainstorageerrors.StoragePoolNotFound)
	} else if err != nil {
		return "", errors.Errorf("getting storage pool: %w", err)
	}

	poolConfig, err := internalstorage.NewConfig(
		pool.Name,
		internalstorage.ProviderType(pool.Provider),
		transform.Map(pool.Attrs, func(k string, v string) (string, any) {
			return k, v
		}),
	)
	if err != nil {
		return "", errors.Errorf(
			"storage pool %q is misconfigured: %w", pool.Name, err,
		)
	}

	registry, err := s.registryGetter.GetStorageRegistry(ctx)
	if err != nil {
		return "", errors.Errorf("getting storage registry: %w", err)
	}

	sp, err := registry.StorageProvider(
		internalstorage.ProviderType(pool.Provider))
	if errors.Is(err, coreerrors.NotFound) {
		return "", errors.Errorf(
			"storage provider type %q not found for pool %q",
			pool.Provider, pool.Name,
		).Add(domainstorageerrors.ProviderTypeNotFound)
	} else if err != nil {
		return "", errors.Errorf("getting storage provider: %w", err)
	}

	ic, err := domainstorageprovisioning.CalculateStorageInstanceComposition(
		domainstorage.StorageKindFilesystem, sp)
	if err != nil {
		return "", errors.Errorf(
			"calculating storage instance composition: %w", err,
		)
	}
	if !ic.FilesystemRequired {
		// This is not possible, since a filesystem kind is the only possible
		// outcome.
		return "", errors.New(
			"calculated storage instance composition is paradoxical",
		)
	} else if !ic.VolumeRequired &&
		ic.FilesystemProvisionScope == domainstorage.ProvisionScopeMachine {
		return "", errors.New(
			"adopting machine scoped filesystem without model scoped volume is not possible",
		).Add(domainstorageerrors.AdoptionNotSupported)
	} else if ic.VolumeRequired &&
		ic.VolumeProvisionScope == domainstorage.ProvisionScopeMachine {
		return "", errors.New(
			"adopting filesystem on machine scoped volume is not possible",
		).Add(domainstorageerrors.AdoptionNotSupported)
	} else if ic.VolumeRequired &&
		ic.FilesystemProvisionScope == domainstorage.ProvisionScopeModel {
		// This is not possible, since a model scoped provisioning of a
		// filesystem by its nature does not need a volume.
		return "", errors.New(
			"calculated storage instance composition is paradoxical",
		)
	}

	if ic.VolumeRequired {
		src, err := sp.VolumeSource(poolConfig)
		if err != nil {
			return "", errors.Errorf("getting volume source: %w", err)
		}
		imp, ok := src.(internalstorage.VolumeImporter)
		if !ok {
			return "", errors.New(
				"storage provider does not support adopting a volume",
			).Add(domainstorageerrors.AdoptionNotSupported)
		}
		return s.adoptVolumeBackedFilesystem(
			ctx, storageName, poolUUID, providerID, force, ic, imp)
	}

	src, err := sp.FilesystemSource(poolConfig)
	if err != nil {
		return "", errors.Errorf("getting filesystem source: %w", err)
	}
	imp, ok := src.(internalstorage.FilesystemImporter)
	if !ok {
		return "", errors.New(
			"storage provider does not support adopting a filesystem",
		).Add(domainstorageerrors.AdoptionNotSupported)
	}
	return s.adoptFilesystem(
		ctx, storageName, poolUUID, providerID, force, ic, imp)
}

// adoptVolumeBackedFilesystem adopts a filesystem that is backed by a volume by
// using the given [internalstorage.VolumeImporter] to import the volume
// identified by providerID. On success, a new storage instance is persisted
// with its associated volume and filesystem in a detached state, and the
// resulting storage ID is returned.
// The following errors can be expected:
// - [domainstorageerrors.AdoptionNotSupported] if the storage provider does not
// support importing the specified volume.
// - [domainstorageerrors.StorageEntityNotFoundInPool] if no pooled volume with
// the given providerID exists.
func (s *StorageService) adoptVolumeBackedFilesystem(
	ctx context.Context,
	storageName domainstorage.Name,
	poolUUID domainstorage.StoragePoolUUID,
	providerID string,
	force bool,
	ic domainstorageprovisioning.StorageInstanceComposition,
	imp internalstorage.VolumeImporter,
) (corestorage.ID, error) {
	tags, err := s.getStorageResourceTagsForModel(ctx)
	if err != nil {
		return "", errors.Errorf("getting resource tag info: %w", err)
	}
	volInfo, err := imp.ImportVolume(
		ctx, providerID, storageName.String(), tags, force)
	if errors.Is(err, coreerrors.NotSupported) {
		return "", errors.Errorf(
			"storage provider does not support adopting volume %q",
			providerID,
		).Add(domainstorageerrors.AdoptionNotSupported)
	} else if errors.Is(err, coreerrors.NotFound) {
		return "", errors.Errorf(
			"pooled volume %q not found", providerID,
		).Add(domainstorageerrors.StorageEntityNotFoundInPool)
	} else if err != nil {
		return "", errors.Errorf("importing volume: %w", err)
	}

	storageInstanceUUID, err := domainstorage.NewStorageInstanceUUID()
	if err != nil {
		return "", errors.Capture(err)
	}
	filesystemUUID, err := domainstorage.NewFilesystemUUID()
	if err != nil {
		return "", errors.Capture(err)
	}
	volumeUUID, err := domainstorage.NewVolumeUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	updatedAt := s.clock.Now().UTC()
	fsArgs := domainstorageinternal.CreateStorageInstanceWithExistingFilesystem{
		Name:                      storageName,
		RequestedSizeMiB:          volInfo.Size,
		StoragePoolUUID:           poolUUID,
		UUID:                      storageInstanceUUID,
		FilesystemUUID:            filesystemUUID,
		FilesystemProvisionScope:  ic.FilesystemProvisionScope,
		FilesystemSize:            volInfo.Size,
		FilesystemProviderID:      "", // No Provider ID when Volume Backed.
		FilesystemStatusID:        int(domainstatus.StorageFilesystemStatusTypeDetached),
		FilesystemStatusMessage:   "filesystem imported",
		FilesystemStatusUpdatedAt: updatedAt,
	}
	args := domainstorageinternal.CreateStorageInstanceWithExistingVolumeBackedFilesystem{
		CreateStorageInstanceWithExistingFilesystem: fsArgs,
		VolumeUUID:            volumeUUID,
		VolumeProvisionScope:  ic.VolumeProvisionScope,
		VolumeSize:            volInfo.Size,
		VolumeProviderID:      volInfo.VolumeId,
		VolumeHardwareID:      volInfo.HardwareId,
		VolumeWWN:             volInfo.WWN,
		VolumePersistent:      volInfo.Persistent,
		VolumeStatusID:        int(domainstatus.StorageVolumeStatusTypeDetached),
		VolumeStatusMessage:   "volume imported",
		VolumeStatusUpdatedAt: updatedAt,
	}

	storageInstanceID, err := s.st.CreateStorageInstanceWithExistingVolumeBackedFilesystem(
		ctx, args,
	)
	if err != nil {
		return "", errors.Errorf(
			"creating adopted storage instance with volume backed filesystem: %w",
			err,
		)
	}

	return corestorage.ID(storageInstanceID), nil
}

// adoptFilesystem adopts a directly-provisioned filesystem (i.e. one that is
// not backed by a volume) by using the given
// [internalstorage.FilesystemImporter] to import the filesystem identified by
// providerID. On success, a new storage instance is persisted with its
// associated filesystem in a detached state, and the resulting storage ID is
// returned.
// The following errors can be expected:
// - [domainstorageerrors.AdoptionNotSupported] if the storage provider does not
// support importing the specified filesystem.
// - [domainstorageerrors.StorageEntityNotFoundInPool] if no pooled filesystem
// with the given providerID exists.
func (s *StorageService) adoptFilesystem(
	ctx context.Context,
	storageName domainstorage.Name,
	poolUUID domainstorage.StoragePoolUUID,
	providerID string,
	force bool,
	ic domainstorageprovisioning.StorageInstanceComposition,
	imp internalstorage.FilesystemImporter,
) (corestorage.ID, error) {
	tags, err := s.getStorageResourceTagsForModel(ctx)
	if err != nil {
		return "", errors.Errorf("getting resource tag info: %w", err)
	}
	fsInfo, err := imp.ImportFilesystem(
		ctx, providerID, storageName.String(), tags, force)
	if errors.Is(err, coreerrors.NotSupported) {
		return "", errors.Errorf(
			"storage provider does not support adopting filesystem %q",
			providerID,
		).Add(domainstorageerrors.AdoptionNotSupported)
	} else if errors.Is(err, coreerrors.NotFound) {
		return "", errors.Errorf(
			"pooled filesystem %q not found", providerID,
		).Add(domainstorageerrors.StorageEntityNotFoundInPool)
	} else if err != nil {
		return "", errors.Errorf("importing filesystem: %w", err)
	}

	storageInstanceUUID, err := domainstorage.NewStorageInstanceUUID()
	if err != nil {
		return "", errors.Capture(err)
	}
	filesystemUUID, err := domainstorage.NewFilesystemUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	updatedAt := s.clock.Now().UTC()
	args := domainstorageinternal.CreateStorageInstanceWithExistingFilesystem{
		Name:                      storageName,
		RequestedSizeMiB:          fsInfo.Size,
		StoragePoolUUID:           poolUUID,
		UUID:                      storageInstanceUUID,
		FilesystemUUID:            filesystemUUID,
		FilesystemProvisionScope:  ic.FilesystemProvisionScope,
		FilesystemSize:            fsInfo.Size,
		FilesystemProviderID:      fsInfo.ProviderId,
		FilesystemStatusID:        int(domainstatus.StorageFilesystemStatusTypeDetached),
		FilesystemStatusMessage:   "filesystem imported",
		FilesystemStatusUpdatedAt: updatedAt,
	}

	storageInstanceID, err := s.st.CreateStorageInstanceWithExistingFilesystem(
		ctx, args,
	)
	if err != nil {
		return "", errors.Errorf(
			"creating adopted storage instance with filesystem: %w", err,
		)
	}

	return corestorage.ID(storageInstanceID), nil
}

// getStorageResourceTagsForModel returns the tags to apply to storage in this
// model.
func (s *StorageService) getStorageResourceTagsForModel(ctx context.Context) (
	map[string]string, error,
) {
	info, err := s.st.GetStorageResourceTagInfoForModel(
		ctx, config.ResourceTagsKey)
	if err != nil {
		return nil, errors.Capture(err)
	}

	rval := make(map[string]string, 3)
	// Resource tags as defined in model config are space separated key-value
	// pairs, where the key and value are separated by an equals sign.
	for pair := range strings.SplitSeq(info.BaseResourceTags, " ") {
		if pair == "" {
			continue
		}
		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, errors.Errorf("malformed resource tag %q", pair)

		}
		if strings.HasPrefix(key, tags.JujuTagPrefix) {
			continue
		}
		rval[key] = value
	}
	rval[tags.JujuController] = info.ControllerUUID
	rval[tags.JujuModel] = info.ModelUUID

	return rval, nil
}
