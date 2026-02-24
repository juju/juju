// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/set"
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
	// GetNetNodeUUIDsByMachineOrUnitID returns net node UUIDs for all machine or
	// and unit names provided. If a machine name or unit name is not found then it
	// is excluded from the result.
	GetNetNodeUUIDsByMachineOrUnitName(
		ctx context.Context,
		machines []string,
		units []string,
	) (map[string]string, map[string]string, error)

	// ImportStorageInstances imports storage instances and storage unit
	// unit owners if the unit name is provided.
	ImportStorageInstances(ctx context.Context, args []internal.ImportStorageInstanceArgs) error

	// ImportFilesystemsIAAS imports filesystems from the provided parameters
	// for IAAS models.
	ImportFilesystemsIAAS(
		ctx context.Context,
		fsArgs []internal.ImportFilesystemIAASArgs,
		attachmentArgs []internal.ImportFilesystemAttachmentIAASArgs,
	) error

	// GetStoragePoolProvidersByNames returns a map of storage pool names to their
	// provider types for the specified storage pool names.
	GetStoragePoolProvidersByNames(ctx context.Context, names []string) (map[string]string, error)

	// GetStorageInstanceUUIDsByIDs retrieves the UUIDs of storage instances by
	// their IDs.
	GetStorageInstanceUUIDsByIDs(ctx context.Context, storageIDs []string) (map[string]string, error)
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

// ImportFilesystemsIAAS imports filesystems from the provided parameters for
// IAAS models.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when any of the params did not pass validation.
// - [domainstorageerrors.StoragePoolNotFound] when any of the specified
// storage pools do not exist.
// - [domainstorageerrors.ProviderTypeNotFound] when the provider type for any
// of the specified storage pools cannot be found in the storage registry.
// - [domainstorageerrors.StorageInstanceNotFound] when any of the
// provided IDs do not have a corresponding storage instance.
func (s *StorageImportService) ImportFilesystemsIAAS(ctx context.Context, params []domainstorage.ImportFilesystemParams) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(params) == 0 {
		return nil
	}

	var (
		poolNames = make([]string, len(params))
		// The vast majority of the time, storageInstanceIDs will be full length
		storageInstanceIDs = make([]string, 0, len(params))
		units              = set.NewStrings()
		machines           = set.NewStrings()
	)
	for i, arg := range params {
		if err := arg.Validate(); err != nil {
			return errors.Errorf("validating import filesystem params %d: %w", i, err)
		}

		poolNames[i] = arg.PoolName
		if arg.StorageInstanceID != "" {
			storageInstanceIDs = append(storageInstanceIDs, arg.StorageInstanceID)
		}

		for _, attachment := range arg.Attachments {
			if attachment.HostUnitName != "" {
				units.Add(attachment.HostUnitName)
			} else {
				machines.Add(attachment.HostMachineName)
			}
		}
	}

	poolScopes, err := s.retrieveFilesystemProviderScopesForPools(ctx, poolNames)
	if err != nil {
		return errors.Errorf("getting provider scopes of filesystems: %w", err)
	}

	storageInstanceUUIDsByID, err := s.st.GetStorageInstanceUUIDsByIDs(ctx, storageInstanceIDs)
	if err != nil {
		return errors.Errorf("retrieving storage instance UUIDs by IDs: %w", err)
	}

	var machineNodes, unitNodes map[string]string
	if len(machines)+len(units) > 0 {
		machineNodes, unitNodes, err = s.st.GetNetNodeUUIDsByMachineOrUnitName(ctx, machines.Values(), units.Values())
		if err != nil {
			return errors.Errorf("retrieving net node UUIDs by machine or unit names: %w", err)
		}
	}

	fsArgs := make([]internal.ImportFilesystemIAASArgs, len(params))
	attachmentArgs := make([]internal.ImportFilesystemAttachmentIAASArgs, 0)
	for i, arg := range params {
		providerScope, ok := poolScopes[arg.PoolName]
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

		fsUUID, err := domainstorage.NewFilesystemUUID()
		if err != nil {
			return errors.Errorf("generating UUID for filesystem %q: %w", arg.ID, err)
		}

		fsArgs[i] = internal.ImportFilesystemIAASArgs{
			UUID:                fsUUID.String(),
			ID:                  arg.ID,
			Life:                life.Alive,
			SizeInMiB:           arg.SizeInMiB,
			ProviderID:          arg.ProviderID,
			StorageInstanceUUID: storageInstanceUUID,
			Scope:               providerScope,
		}

		for _, attachment := range arg.Attachments {
			attachmentUUID, err := domainstorage.NewFilesystemAttachmentUUID()
			if err != nil {
				return errors.Errorf("generating UUID for filesystem attachment of filesystem %q: %w", arg.ID, err)
			}

			var netNodeUUID string
			if attachment.HostUnitName != "" {
				var ok bool
				netNodeUUID, ok = unitNodes[attachment.HostUnitName]
				if !ok {
					return errors.Errorf("net node for host unit %q not found", attachment.HostUnitName).
						Add(coreerrors.NotFound)
				}
			} else {
				var ok bool
				netNodeUUID, ok = machineNodes[attachment.HostMachineName]
				if !ok {
					return errors.Errorf("net node for host machine %q not found", attachment.HostMachineName).
						Add(coreerrors.NotFound)
				}
			}

			attachmentArgs = append(attachmentArgs, internal.ImportFilesystemAttachmentIAASArgs{
				UUID:           attachmentUUID.String(),
				FilesystemUUID: fsUUID.String(),
				NetNodeUUID:    netNodeUUID,
				Scope:          providerScope,
				Life:           life.Alive,
				MountPoint:     attachment.MountPoint,
				ReadOnly:       attachment.ReadOnly,
			})
		}
	}

	return s.st.ImportFilesystemsIAAS(ctx, fsArgs, attachmentArgs)
}

func (s *StorageImportService) retrieveFilesystemProviderScopesForPools(
	ctx context.Context, poolNames []string,
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
			domainstorage.StorageKindFilesystem, storageProvider)
		if err != nil {
			return nil, errors.Errorf(
				"calculating storage instance composition for pool %q: %w",
				poolName, err,
			)
		}

		providerScopes[poolName] = ic.FilesystemProvisionScope
	}

	return providerScopes, nil
}
