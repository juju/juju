// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/trace"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/domain/storage/internal"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/errors"
	internalstorage "github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/uuid"
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

	// GetStorageInstanceUUIDsByIDs retrieves the UUIDs of storage instances by
	// their IDs.
	GetStorageInstanceUUIDsByIDs(ctx context.Context, storageIDs []string) (map[string]string, error)

	// GetStorageInstanceUUIDsByVolumeIDs retrieves the UUIDs of storage
	// instances by their linked volume IDs.
	GetStorageInstanceUUIDsByVolumeIDs(ctx context.Context, volumeIDs []string) (map[string]string, error)

	// GetStoragePoolProvidersByNames returns a map of storage pool names to their
	// provider types for the specified storage pool names.
	GetStoragePoolProvidersByNames(ctx context.Context, names []string) (map[string]string, error)

	// GetUnitUUIDsByNames returns a map of unit names to unit UUIDs for the provided
	// unit names.
	GetUnitUUIDsByNames(ctx context.Context, units []string) (map[string]string, error)

	// ImportFilesystemsIAAS imports filesystems from the provided parameters
	// for IAAS models.
	ImportFilesystemsIAAS(
		ctx context.Context,
		fsArgs []internal.ImportFilesystemIAASArgs,
		attachmentArgs []internal.ImportFilesystemAttachmentIAASArgs,
	) error

	// ImportStorageInstances imports storage instances, storage attachments and
	// storage unit owners if the unit name is provided.
	ImportStorageInstances(
		ctx context.Context,
		instanceArgs []internal.ImportStorageInstanceArgs,
		attachmentArgs []internal.ImportStorageInstanceAttachmentArgs,
	) error

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
// - [applicationerrors.UnitNotFound] when a unit name is provided but not found in the model.
func (s *StorageImportService) ImportStorageInstances(ctx context.Context, params []domainstorage.ImportStorageInstanceParams) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(params) == 0 {
		return nil
	}

	units := set.NewStrings()
	for i, param := range params {
		if err := param.Validate(); err != nil {
			return errors.Errorf("validating import storage instance params %d: %w", i, err)
		}

		if param.UnitName != "" {
			units.Add(param.UnitName)
		}
		for _, attachment := range param.AttachedUnitNames {
			units.Add(attachment)
		}
	}

	var unitUUIDs map[string]string
	if len(units) > 0 {
		var err error
		unitUUIDs, err = s.st.GetUnitUUIDsByNames(ctx, units.Values())
		if err != nil {
			return errors.Errorf("getting unit UUIDs by names: %w", err)
		}
	}

	instanceArgs := make([]internal.ImportStorageInstanceArgs, len(params))
	attachmentArgs := make([]internal.ImportStorageInstanceAttachmentArgs, 0)
	for i, param := range params {
		storageUUID, err := domainstorage.NewStorageInstanceUUID()
		if err != nil {
			return errors.Capture(err)
		}

		var unitUUID string
		if unitName := param.UnitName; unitName != "" {
			var ok bool
			unitUUID, ok = unitUUIDs[unitName]
			if !ok {
				return errors.Errorf("unit with name %q not found for storage instance", unitName).
					Add(applicationerrors.UnitNotFound)
			}
		}

		instanceArgs[i] = internal.ImportStorageInstanceArgs{
			UUID: storageUUID.String(),
			// 3.6 does not pass life of a storage instance during
			// import. Assume alive. domainlife.Life has a test which
			// validates the data against the db.
			Life:              life.Alive,
			PoolName:          param.PoolName,
			RequestedSizeMiB:  param.RequestedSizeMiB,
			StorageInstanceID: param.StorageInstanceID,
			StorageName:       param.StorageName,
			StorageKind:       param.StorageKind,
			UnitUUID:          unitUUID,
		}

		for _, attachment := range param.AttachedUnitNames {
			attachmentUUID, err := domainstorage.NewStorageAttachmentUUID()
			if err != nil {
				return errors.Capture(err)
			}

			attachmentUnitUUID, ok := unitUUIDs[attachment]
			if !ok {
				return errors.Errorf("unit with name %q not found for storage instance attachment", attachment).
					Add(applicationerrors.UnitNotFound)
			}

			attachmentArgs = append(attachmentArgs, internal.ImportStorageInstanceAttachmentArgs{
				UUID:                attachmentUUID.String(),
				StorageInstanceUUID: storageUUID.String(),
				UnitUUID:            attachmentUnitUUID,
				Life:                life.Alive,
			})
		}
	}

	return s.st.ImportStorageInstances(ctx, instanceArgs, attachmentArgs)
}

type filesystemImportPartition struct {
	poolNames          []string
	storageInstanceIDs []string
	volumeIDs          []string
	orphanIndexes      []int
	machines           []string
	units              []string
}

type filesystemImportLookups struct {
	poolScopes                             map[string]domainstorageprovisioning.ProvisionScope
	storageInstanceUUIDsByID               map[string]string
	storageInstanceUUIDsByVolumeID         map[string]string
	orphanStorageInstanceUUIDsByFilesystem map[int]string
	machineNodes                           map[string]string
	unitNodes                              map[string]string
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
// - [applicationerrors.UnitNotFound] when a host unit name is provided but not
// found in the model.
// - [machineerrors.MachineNotFound] when a host machine name is provided but not
// found in the model.
func (s *StorageImportService) ImportFilesystemsIAAS(ctx context.Context, params []domainstorage.ImportFilesystemParams) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(params) == 0 {
		return nil
	}

	partition, err := s.partitionFilesystemImportParams(params)
	if err != nil {
		return errors.Capture(err)
	}

	lookups, err := s.resolveFilesystemImportLookups(ctx, params, partition)
	if err != nil {
		return errors.Capture(err)
	}

	fsArgs, attachmentArgs, err := s.makeImportFilesystemIAASArgs(params, lookups)
	if err != nil {
		return errors.Capture(err)
	}

	return s.st.ImportFilesystemsIAAS(ctx, fsArgs, attachmentArgs)
}

func (s *StorageImportService) partitionFilesystemImportParams(
	params []domainstorage.ImportFilesystemParams,
) (filesystemImportPartition, error) {
	partition := filesystemImportPartition{
		poolNames:          make([]string, len(params)),
		storageInstanceIDs: make([]string, 0, len(params)),
		volumeIDs:          make([]string, 0, len(params)),
		orphanIndexes:      make([]int, 0, len(params)),
	}

	units := set.NewStrings()
	machines := set.NewStrings()
	for i, arg := range params {
		if err := arg.Validate(); err != nil {
			return filesystemImportPartition{}, errors.Errorf("validating import filesystem params %d: %w", i, err)
		}

		partition.poolNames[i] = arg.PoolName
		switch {
		case arg.StorageInstanceID != "":
			partition.storageInstanceIDs = append(partition.storageInstanceIDs, arg.StorageInstanceID)
		case arg.VolumeID != "":
			partition.volumeIDs = append(partition.volumeIDs, arg.VolumeID)
		default:
			partition.orphanIndexes = append(partition.orphanIndexes, i)
		}

		for _, attachment := range arg.Attachments {
			if attachment.HostUnitName != "" {
				units.Add(attachment.HostUnitName)
			} else {
				machines.Add(attachment.HostMachineName)
			}
		}
	}

	partition.machines = machines.Values()
	partition.units = units.Values()
	return partition, nil
}

func (s *StorageImportService) resolveFilesystemImportLookups(
	ctx context.Context,
	params []domainstorage.ImportFilesystemParams,
	partition filesystemImportPartition,
) (filesystemImportLookups, error) {
	poolScopes, err := s.retrieveProviderScopesForPools(ctx, domainstorage.StorageKindFilesystem, partition.poolNames)
	if err != nil {
		return filesystemImportLookups{}, errors.Errorf("getting provider scopes of filesystems: %w", err)
	}

	storageInstanceUUIDsByID := map[string]string{}
	if len(partition.storageInstanceIDs) > 0 {
		storageInstanceUUIDsByID, err = s.st.GetStorageInstanceUUIDsByIDs(ctx, partition.storageInstanceIDs)
		if err != nil {
			return filesystemImportLookups{}, errors.Errorf("retrieving storage instance UUIDs by IDs: %w", err)
		}
	}

	storageInstanceUUIDsByVolumeID := map[string]string{}
	if len(partition.volumeIDs) > 0 {
		storageInstanceUUIDsByVolumeID, err = s.st.GetStorageInstanceUUIDsByVolumeIDs(ctx, partition.volumeIDs)
		if err != nil {
			return filesystemImportLookups{}, errors.Errorf("retrieving storage instance UUIDs by volume IDs: %w", err)
		}
	}

	orphanStorageInstanceUUIDsByFilesystem := map[int]string{}
	if len(partition.orphanIndexes) > 0 {
		orphanStorageInstanceUUIDsByFilesystem, err = s.importOrphanedFilesystemStorageInstances(ctx, params, partition.orphanIndexes)
		if err != nil {
			return filesystemImportLookups{}, errors.Errorf("importing orphaned storage instances: %w", err)
		}
	}

	lookups := filesystemImportLookups{
		poolScopes:                             poolScopes,
		storageInstanceUUIDsByID:               storageInstanceUUIDsByID,
		storageInstanceUUIDsByVolumeID:         storageInstanceUUIDsByVolumeID,
		orphanStorageInstanceUUIDsByFilesystem: orphanStorageInstanceUUIDsByFilesystem,
	}

	if len(partition.machines)+len(partition.units) == 0 {
		return lookups, nil
	}

	lookups.machineNodes, lookups.unitNodes, err = s.st.GetNetNodeUUIDsByMachineOrUnitName(
		ctx, partition.machines, partition.units,
	)
	if err != nil {
		return filesystemImportLookups{}, errors.Errorf("retrieving net node UUIDs by machine or unit names: %w", err)
	}

	return lookups, nil
}

func (s *StorageImportService) makeImportFilesystemIAASArgs(
	params []domainstorage.ImportFilesystemParams,
	lookups filesystemImportLookups,
) ([]internal.ImportFilesystemIAASArgs, []internal.ImportFilesystemAttachmentIAASArgs, error) {
	fsArgs := make([]internal.ImportFilesystemIAASArgs, len(params))
	attachmentArgs := make([]internal.ImportFilesystemAttachmentIAASArgs, 0)
	for i, arg := range params {
		providerScope, ok := lookups.poolScopes[arg.PoolName]
		if !ok {
			return nil, nil, errors.Errorf("storage pool %q not found for filesystem %q", arg.PoolName, arg.ID).
				Add(domainstorageerrors.StoragePoolNotFound)
		}

		storageInstanceUUID, err := s.resolveFilesystemStorageInstanceUUID(i, arg, lookups)
		if err != nil {
			return nil, nil, errors.Capture(err)
		}

		fsUUID, err := domainstorage.NewFilesystemUUID()
		if err != nil {
			return nil, nil, errors.Errorf("generating UUID for filesystem %q: %w", arg.ID, err)
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
				return nil, nil, errors.Errorf("generating UUID for filesystem attachment of filesystem %q: %w", arg.ID, err)
			}

			netNodeUUID, err := s.resolveFilesystemAttachmentNetNodeUUID(attachment, lookups)
			if err != nil {
				return nil, nil, errors.Capture(err)
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

	return fsArgs, attachmentArgs, nil
}

func (s *StorageImportService) resolveFilesystemStorageInstanceUUID(
	filesystemIndex int,
	arg domainstorage.ImportFilesystemParams,
	lookups filesystemImportLookups,
) (string, error) {
	switch {
	case arg.StorageInstanceID != "":
		storageInstanceUUID, ok := lookups.storageInstanceUUIDsByID[arg.StorageInstanceID]
		if !ok {
			return "", errors.Errorf("storage instance with ID %q not found for filesystem %q", arg.StorageInstanceID, arg.ID).
				Add(domainstorageerrors.StorageInstanceNotFound)
		}
		return storageInstanceUUID, nil
	case arg.VolumeID != "":
		storageInstanceUUID, ok := lookups.storageInstanceUUIDsByVolumeID[arg.VolumeID]
		if !ok {
			return "", errors.Errorf("storage instance for volume %q not found for filesystem %q", arg.VolumeID, arg.ID).
				Add(domainstorageerrors.StorageInstanceNotFound)
		}
		return storageInstanceUUID, nil
	default:
		storageInstanceUUID, ok := lookups.orphanStorageInstanceUUIDsByFilesystem[filesystemIndex]
		if !ok {
			return "", errors.Errorf("orphaned storage instance for filesystem %q not found", arg.ID).
				Add(domainstorageerrors.StorageInstanceNotFound)
		}
		return storageInstanceUUID, nil
	}
}

func (s *StorageImportService) resolveFilesystemAttachmentNetNodeUUID(
	attachment domainstorage.ImportFilesystemAttachmentsParams,
	lookups filesystemImportLookups,
) (string, error) {
	if attachment.HostUnitName != "" {
		netNodeUUID, ok := lookups.unitNodes[attachment.HostUnitName]
		if !ok {
			return "", errors.Errorf("net node for host unit %q not found", attachment.HostUnitName).
				Add(applicationerrors.UnitNotFound)
		}
		return netNodeUUID, nil
	}

	netNodeUUID, ok := lookups.machineNodes[attachment.HostMachineName]
	if !ok {
		return "", errors.Errorf("net node for host machine %q not found", attachment.HostMachineName).
			Add(machineerrors.MachineNotFound)
	}
	return netNodeUUID, nil
}

func (s *StorageImportService) importOrphanedFilesystemStorageInstances(
	ctx context.Context,
	params []domainstorage.ImportFilesystemParams,
	orphanIndexes []int,
) (map[int]string, error) {
	if len(orphanIndexes) == 0 {
		return map[int]string{}, nil
	}

	// Include a dashless UUID in the name to be defensive against storage_id
	// collisions. Dashless guarantees the storage name is valid.
	uuid, err := uuid.NewUUID()
	if err != nil {
		return nil, errors.Capture(err)
	}
	orphanedStorageName := fmt.Sprintf("orphaned%s", strings.ReplaceAll(uuid.String(), "-", ""))

	orphanedStorageIDs := make([]string, len(orphanIndexes))
	for i := range orphanedStorageIDs {
		orphanedStorageIDs[i] = fmt.Sprintf("%s/%d", orphanedStorageName, i)
	}

	instanceArgs := make([]internal.ImportStorageInstanceArgs, len(orphanIndexes))
	orphanStorageUUIDsByFilesystemIndex := make(map[int]string, len(orphanIndexes))
	for i, filesystemIndex := range orphanIndexes {
		filesystem := params[filesystemIndex]

		storageUUID, err := domainstorage.NewStorageInstanceUUID()
		if err != nil {
			return nil, errors.Capture(err)
		}

		instanceArgs[i] = internal.ImportStorageInstanceArgs{
			UUID:              storageUUID.String(),
			Life:              life.Alive,
			PoolName:          filesystem.PoolName,
			RequestedSizeMiB:  filesystem.SizeInMiB,
			StorageInstanceID: orphanedStorageIDs[i],
			StorageName:       orphanedStorageName,
			StorageKind:       "filesystem",
		}
		orphanStorageUUIDsByFilesystemIndex[filesystemIndex] = storageUUID.String()
	}

	if err := s.st.ImportStorageInstances(ctx, instanceArgs, nil); err != nil {
		return nil, errors.Capture(err)
	}

	return orphanStorageUUIDsByFilesystemIndex, nil
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

// ImportVolumes creates new volumes and storage instance volumes.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when any of the args did not pass validation.
// - [domainstorageerrors.ProviderTypeNotFound] if storage provider was not found.
// - [domainstorageerrors.StorageInstanceNotFound] if the storage ID was not found.
// - [domainstorageerrors.StoragePoolNotFound] if any of the storage pools do not exist.
func (s *StorageImportService) ImportVolumes(ctx context.Context, params []domainstorage.ImportVolumeParams) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(params) == 0 {
		return nil
	}
	for i, param := range params {
		if err := param.Validate(); err != nil {
			return errors.Errorf("validating import volume params %d: %w", i, err)
		}
	}

	args, err := s.transformImportVolumeArgs(ctx, params)
	if err != nil {
		return errors.Capture(err)
	}

	return s.st.ImportVolumes(ctx, args)
}

func (s *StorageImportService) transformImportVolumeArgs(
	ctx context.Context,
	params []domainstorage.ImportVolumeParams,
) ([]internal.ImportVolumeArgs, error) {
	poolNames := transform.Slice(params, func(in domainstorage.ImportVolumeParams) string {
		return in.Pool
	})
	poolNamesToSP, err := s.retrieveProviderScopesForPools(ctx, domainstorage.StorageKindBlock, poolNames)
	if err != nil {
		return nil, errors.Capture(err)
	}

	instanceIDs := transform.Slice(params, func(in domainstorage.ImportVolumeParams) string {
		return in.StorageID
	})
	instanceUUIDsByIDs, err := s.st.GetStorageInstanceUUIDsByIDs(ctx, instanceIDs)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return transform.SliceOrErr(params, func(in domainstorage.ImportVolumeParams) (internal.ImportVolumeArgs, error) {
		volumeUUID, err := domainstorage.NewVolumeUUID()
		if err != nil {
			return internal.ImportVolumeArgs{}, errors.Errorf("generating volume uuid for %s: %w", in.ProviderID, err)
		}
		provisionScope, ok := poolNamesToSP[in.Pool]
		if !ok {
			return internal.ImportVolumeArgs{}, errors.Errorf("storage pool %q not found",
				in.Pool).Add(domainstorageerrors.StoragePoolNotFound)
		}
		storageInstanceUUID, ok := instanceUUIDsByIDs[in.StorageID]
		if !ok {
			return internal.ImportVolumeArgs{}, errors.Errorf("storage instance %q not found",
				in.StorageID).Add(domainstorageerrors.StorageInstanceNotFound)
		}
		return internal.ImportVolumeArgs{
			UUID:                volumeUUID.String(),
			ID:                  in.ID,
			LifeID:              life.Alive,
			StorageInstanceUUID: storageInstanceUUID,
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
