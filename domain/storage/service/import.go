// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/transform"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/life"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/domain/storage/internal"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/errors"
	internalstorage "github.com/juju/juju/internal/storage"
)

// StorageImportState defines an interface for interacting with the underlying
// state for storage import operations.
type StorageImportState interface {
	// ImportStorageInstances imports storage instances and storage unit
	// unit owners if the unit name is provided.
	ImportStorageInstances(ctx context.Context, args []internal.ImportStorageInstanceArgs) error

	// GetNetNodeUUIDsByMachineOrUnitID returns net node UUIDs for all machine or
	// and unit names provided.
	GetNetNodeUUIDsByMachineOrUnitName(ctx context.Context, machines []string, units []string) (map[string]string, error)

	// GetStoragePoolProvidersByNames returns a map of storage pool names to their
	// provider types for the specified storage pool names.
	GetStoragePoolProvidersByNames(ctx context.Context, names []string) (map[string]string, error)

	// GetStorageInstanceUUIDsByIDs retrieves the UUIDs of storage instances by
	// their IDs.
	GetStorageInstanceUUIDsByIDs(ctx context.Context, storageIDs []string) (map[string]string, error)

	// ImportFilesystems creates new filesystems
	ImportFilesystems(ctx context.Context, args []internal.ImportFilesystemArgs) error

	// ImportVolumes creates new storage volumes and related database structures.
	ImportVolumes(ctx context.Context, args []internal.ImportVolumeArgs) error
}

// StorageImportService defines a service for importing storage entities during
// model import.
type StorageImportService struct {
	st             StorageImportState
	registryGetter corestorage.ModelStorageRegistryGetter
	logger         logger.Logger
}

// ImportStorageInstances imports storage instances and storage unit
// owners. Storage unit owners are created if the unit name is provided.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when any of the params did not pass validation.
func (s *StorageImportService) ImportStorageInstances(ctx context.Context, params []domainstorage.ImportStorageInstanceParams) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(params) == 0 {
		return nil
	}

	for i, param := range params {
		if err := param.Validate(); err != nil {
			return errors.Errorf("validating import storage instance params %d: %w", i, err)
		}
	}

	args, err := transform.SliceOrErr(params, func(in domainstorage.ImportStorageInstanceParams) (internal.ImportStorageInstanceArgs, error) {
		storageUUID, err := domainstorage.NewStorageInstanceUUID()
		if err != nil {
			return internal.ImportStorageInstanceArgs{}, err
		}
		return internal.ImportStorageInstanceArgs{
			UUID: storageUUID.String(),
			// 3.6 does not pass life of a storage instance during
			// import. Assume alive. domainlife.Life has a test which
			// validates the data against the db.
			Life:             int(life.Alive),
			PoolName:         in.PoolName,
			RequestedSizeMiB: in.RequestedSizeMiB,
			StorageID:        in.StorageID,
			StorageName:      in.StorageName,
			StorageKind:      in.StorageKind,
			UnitName:         in.UnitName,
		}, nil
	})
	if err != nil {
		return errors.Capture(err)
	}

	return s.st.ImportStorageInstances(ctx, args)
}

// ImportFilesystems imports filesystems from the provided parameters.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when any of the params did not pass validation.
// - [domainstorageerrors.StoragePoolNotFound] when any of the specified
// storage pools do not exist.
// - [domainstorageerrors.ProviderTypeNotFound] when the provider type for any
// of the specified storage pools cannot be found in the storage registry.
// - [domainstorageerrors.StorageInstanceNotFound] when any of the
// provided IDs do not have a corresponding storage instance.
func (s *StorageImportService) ImportFilesystems(ctx context.Context, params []domainstorage.ImportFilesystemParams) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(params) == 0 {
		return nil
	}

	for i, arg := range params {
		if err := arg.Validate(); err != nil {
			return errors.Errorf("validating import filesystem params %d: %w", i, err)
		}
	}

	poolNames := transform.Slice(params, func(arg domainstorage.ImportFilesystemParams) string { return arg.PoolName })
	providerScopes, err := s.retrieveProviderScopesForPools(ctx, domainstorage.StorageKindFilesystem, poolNames)
	if err != nil {
		return errors.Errorf("getting provider scopes of filesystems: %w", err)
	}

	// storage instance ID can be empty, which indicates the filesystem is not
	// associated with any storage instance.
	storageInstanceIDs := make([]string, 0, len(params))
	for _, arg := range params {
		if arg.StorageInstanceID != "" {
			storageInstanceIDs = append(storageInstanceIDs, arg.StorageInstanceID)
		}
	}
	storageInstanceUUIDsByID, err := s.st.GetStorageInstanceUUIDsByIDs(ctx, storageInstanceIDs)
	if err != nil {
		return errors.Errorf("retrieving storage instance UUIDs by IDs: %w", err)
	}

	fullArgs := make([]internal.ImportFilesystemArgs, len(params))
	for i, arg := range params {
		providerScope, ok := providerScopes[arg.PoolName]
		if !ok {
			// This indicates a programming error. We should fail in the state
			// if a pool name is not found.
			return errors.Errorf("storage pool %q not found for filesystem %q", arg.PoolName, arg.ID).
				Add(domainstorageerrors.StoragePoolNotFound)
		}

		var storageInstanceUUID string
		if arg.StorageInstanceID != "" {
			var ok bool
			storageInstanceUUID, ok = storageInstanceUUIDsByID[arg.StorageInstanceID]
			if !ok {
				return errors.Errorf("storage instance with ID %q not found for filesystem %q", arg.StorageInstanceID, arg.ID).
					Add(domainstorageerrors.StorageInstanceNotFound)
			}
		}

		uuid, err := domainstorage.NewFilesystemUUID()
		if err != nil {
			return errors.Errorf("generating UUID for filesystem %q: %w", arg.ID, err)
		}

		fullArgs[i] = internal.ImportFilesystemArgs{
			UUID:                uuid.String(),
			ID:                  arg.ID,
			Life:                life.Alive,
			SizeInMiB:           arg.SizeInMiB,
			ProviderID:          arg.ProviderID,
			StorageInstanceUUID: storageInstanceUUID,
			Scope:               providerScope,
		}
	}

	return s.st.ImportFilesystems(ctx, fullArgs)
}

// retrieveProviderScopesForPools gets the provider scopes for the given
// pool based on the provided storage kind.
func (s *StorageImportService) retrieveProviderScopesForPools(
	ctx context.Context, kind domainstorage.StorageKind, poolNames []string,
) (map[string]domainstorageprovisioning.ProvisionScope, error) {
	providerScopes := make(map[string]domainstorageprovisioning.ProvisionScope)

	providerMap, err := s.st.GetStoragePoolProvidersByNames(ctx, poolNames)
	if err != nil {
		return nil, errors.Errorf("getting storage pool providers by names: %w", err)
	}

	registry, err := s.registryGetter.GetStorageRegistry(ctx)
	if err != nil {
		return nil, errors.Errorf("getting storage registry: %w", err)
	}

	for poolName, providerType := range providerMap {
		storageProvider, err := registry.StorageProvider(
			internalstorage.ProviderType(providerType))
		if errors.Is(err, coreerrors.NotFound) {
			return nil, errors.Errorf(
				"storage provider type %q not found for pool %q",
				providerType, poolName,
			).Add(domainstorageerrors.ProviderTypeNotFound)
		} else if err != nil {
			return nil, errors.Errorf("getting storage provider %q for storage pool %q: %w",
				providerType, poolName, err)
		}

		ic, err := domainstorageprovisioning.CalculateStorageInstanceComposition(
			kind, storageProvider)
		if err != nil {
			return nil, errors.Errorf(
				"calculating storage instance composition for pool %q: %w",
				poolName, err,
			)
		}

		switch kind {
		case domainstorage.StorageKindFilesystem:
			providerScopes[poolName] = ic.FilesystemProvisionScope
		case domainstorage.StorageKindBlock:
			providerScopes[poolName] = ic.VolumeProvisionScope
		}
	}

	return providerScopes, nil
}

// ImportVolumes associates a volume (either native or volume backed) hosted by
// a cloud provider with a new storage instance (and storage pool) in a model.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when any of the args did not pass validation.
// - [domainstorageerrors.ProviderTypeNotFound] if storage provider was not found.
// - [domainstorageerrors.StoragePoolNotFound] if any of the storage pools do not exist.
func (s *StorageImportService) ImportVolumes(ctx context.Context, params domainstorage.ImportVolumeParams) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if params == nil {
		return nil
	}
	if err := params.Validate(); err != nil {
		return errors.Capture(err)
	}
	args, err := s.transformImportVolumeArgs(ctx, params)
	if err != nil {
		return errors.Capture(err)
	}

	return s.st.ImportVolumes(ctx, args)
}

func (s *StorageImportService) transformImportVolumeArgs(
	ctx context.Context,
	params domainstorage.ImportVolumeParams,
) ([]internal.ImportVolumeArgs, error) {
	poolNames := transform.Slice(params, func(in domainstorage.ImportVolumeParam) string {
		return in.Pool
	})
	poolNamesToSP, err := s.retrieveProviderScopesForPools(ctx, domainstorage.StorageKindBlock, poolNames)
	if err != nil {
		return nil, errors.Capture(err)
	}

	instanceIDs := transform.Slice(params, func(in domainstorage.ImportVolumeParam) string {
		return in.StorageID
	})
	instanceUUIDsByIDs, err := s.st.GetStorageInstanceUUIDsByIDs(ctx, instanceIDs)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return transform.SliceOrErr(params, func(in domainstorage.ImportVolumeParam) (internal.ImportVolumeArgs, error) {
		volumeUUID, err := domainstorage.NewVolumeUUID()
		if err != nil {
			return internal.ImportVolumeArgs{}, errors.Errorf("generating volume uuid for %s: %w", in.ProviderID, err)
		}
		// in.Pool was validate and exists, or retrieveStoragePools
		// would have failed with a StoragePoolNotFound error.
		provisionScope, _ := poolNamesToSP[in.Pool]
		// in.StorageID was validate and exists, or GetStorageInstanceUUIDsByIDs
		// would have failed with a NotFound error.
		storageInstanceUUID, _ := instanceUUIDsByIDs[in.StorageID]
		return internal.ImportVolumeArgs{
			UUID:                volumeUUID.String(),
			ID:                  in.ID,
			LifeID:              life.Alive,
			StorageInstanceUUID: storageInstanceUUID,
			StorageID:           in.StorageID,
			Provisioned:         in.Provisioned,
			ProvisionScopeID:    provisionScope,
			SizeMiB:             in.SizeMiB,
			HardwareID:          in.HardwareID,
			WWN:                 in.WWN,
			ProviderID:          in.ProviderID,
			Persistent:          in.Persistent,
		}, nil
	})
}
