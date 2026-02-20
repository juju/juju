// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"slices"

	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	coreblockdevice "github.com/juju/juju/core/blockdevice"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/blockdevice"
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
	// GetBlockDevicesForMachine returns the BlockDevices for the specified
	// machine.
	GetBlockDevicesForMachine(
		ctx context.Context, machineUUID machine.UUID,
	) (map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice, error)

	// GetMachineUUIDByNetNodeUUID gets the uuid for the machine with the
	// given net node UUID.
	//
	// The following errors may be returned:
	// - [machineerrors.MachineNotFound] when the machine is not found.
	// - [machineerrors.MachineIsDead] when the machine is dead.
	GetMachineUUIDByNetNodeUUID(ctx context.Context, netNodeUUID string) (machine.UUID, error)

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

	// GetStoragePoolProvidersByNames returns a map of storage pool names to their
	// provider types for the specified storage pool names.
	GetStoragePoolProvidersByNames(ctx context.Context, names []string) (map[string]string, error)

	// ImportFilesystemsIAAS imports filesystems from the provided parameters
	// for IAAS models.
	ImportFilesystemsIAAS(
		ctx context.Context,
		fsArgs []internal.ImportFilesystemIAASArgs,
		attachmentArgs []internal.ImportFilesystemAttachmentIAASArgs,
	) error

	// ImportStorageInstances creates new storage instances and storage
	// unit owners if the unit name is provided.
	ImportStorageInstances(ctx context.Context, args []internal.ImportStorageInstanceArgs) error

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

	poolScopes, err := s.retrieveProviderScopesForPools(ctx, domainstorage.StorageKindFilesystem, poolNames)
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
	slices.Sort(poolNames)
	poolNames = slices.Compact(poolNames)
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

		volumeAttachments, volumeAttachmentsNewBD, err := s.transformVolumeAttachments(ctx, in.Attachments)
		if err != nil {
			return internal.ImportVolumeArgs{}, errors.Errorf("transforming volume attachments: %w", err)
		}

		volumeAttachmentPlans, err := s.transformVolumeAttachmentPlans(ctx, in.AttachmentPlans)
		if err != nil {
			return internal.ImportVolumeArgs{}, errors.Errorf("transforming volume attachment plans: %w", err)
		}

		return internal.ImportVolumeArgs{
			UUID:                          volumeUUID.String(),
			ID:                            in.ID,
			LifeID:                        life.Alive,
			StorageInstanceUUID:           storageInstanceUUID,
			StorageID:                     in.StorageID,
			Provisioned:                   in.Provisioned,
			ProvisionScopeID:              provisionScope,
			SizeMiB:                       in.SizeMiB,
			HardwareID:                    in.HardwareID,
			WWN:                           in.WWN,
			ProviderID:                    in.ProviderID,
			Persistent:                    in.Persistent,
			Attachments:                   volumeAttachments,
			AttachmentsWithNewBlockDevice: volumeAttachmentsNewBD,
			AttachmentPlans:               volumeAttachmentPlans,
		}, nil
	})
}

func (s *StorageImportService) transformVolumeAttachments(
	ctx context.Context,
	attachments []domainstorage.ImportVolumeAttachment,
) ([]internal.ImportVolumeAttachment, []internal.ImportVolumeAttachmentNewBlockDevice, error) {
	if len(attachments) == 0 {
		return nil, nil, nil
	}

	// Collect data used to find required UUIDs for both
	// return types.
	machineNames := make([]string, 0)
	unitNames := make([]string, 0)
	for _, attachment := range attachments {
		if machine := attachment.MachineID; machine != "" {
			machineNames = append(machineNames, machine)
		}
		if unit := attachment.UnitID; unit != "" {
			unitNames = append(unitNames, unit)
		}
	}

	machineNetNodes, unitNetNodes, err := s.st.GetNetNodeUUIDsByMachineOrUnitName(ctx, machineNames, unitNames)
	if err != nil {
		return nil, nil, errors.Errorf("getting net node uuid: %w", err)
	}

	// Break the attachments into two groups, one which has a block device
	// already imported, and one that does not.
	existingAttachments := make([]internal.ImportVolumeAttachment, 0)
	createBlockDeviceAttachments := make([]internal.ImportVolumeAttachmentNewBlockDevice, 0)
	for _, attachment := range attachments {
		uuid, err := domainstorage.NewVolumeAttachmentUUID()
		if err != nil {
			return nil, nil, errors.Capture(err)
		}
		var (
			netNodeUUID string
			ok          bool
		)
		if attachment.MachineID != "" {
			netNodeUUID, ok = machineNetNodes[attachment.MachineID]
			if !ok {
				return nil, nil, errors.Errorf("network node uuid for machine %q not found", attachment.MachineID)
			}
		} else if attachment.UnitID != "" {
			netNodeUUID, ok = unitNetNodes[attachment.UnitID]
			if !ok {
				return nil, nil, errors.Errorf("network node uuid for unit %q not found", attachment.MachineID)
			}
		}

		machineUUID, err := s.st.GetMachineUUIDByNetNodeUUID(ctx, netNodeUUID)
		if err != nil {
			return nil, nil, errors.Errorf("getting machine uuid: %w", err)
		}

		iva := internal.ImportVolumeAttachment{
			UUID:        uuid.String(),
			LifeID:      life.Alive,
			NetNodeUUID: netNodeUUID,
			ReadOnly:    attachment.ReadOnly,
		}

		machineBlockDevices, err := s.st.GetBlockDevicesForMachine(ctx, machineUUID)
		if err != nil {
			return nil, nil, errors.Errorf("getting block devices for machine: %w", err)
		}

		if blockDeviceUUID, ok := findBlockDeviceUUID(attachment.CoreBlockDevice(), machineBlockDevices); ok {
			iva.BlockDeviceUUID = blockDeviceUUID
			existingAttachments = append(existingAttachments, iva)
			continue
		}

		// No block device found for the volume attachment,
		// add info to create a new one.
		blockDeviceUUID, err := blockdevice.NewBlockDeviceUUID()
		if err != nil {
			return nil, nil, errors.Capture(err)
		}
		iva.BlockDeviceUUID = blockDeviceUUID.String()
		createBlockDeviceAttachments = append(createBlockDeviceAttachments,
			internal.ImportVolumeAttachmentNewBlockDevice{
				ImportVolumeAttachment: iva,
				BusAddress:             attachment.BusAddress,
				DeviceLink:             attachment.DeviceLink,
				DeviceName:             attachment.DeviceName,
				Provisioned:            attachment.Provisioned,
				MachineUUID:            machineUUID.String(),
			})
	}

	return existingAttachments, createBlockDeviceAttachments, nil
}

func findBlockDeviceUUID(
	find coreblockdevice.BlockDevice,
	blockDevices map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice,
) (string, bool) {
	for bdUUID, bd := range blockDevices {
		if blockdevice.SameDevice(find, bd) {
			return bdUUID.String(), true
		}
	}
	return "", false
}

func (s *StorageImportService) transformVolumeAttachmentPlans(
	ctx context.Context,
	plans []domainstorage.ImportVolumeAttachmentPlan,
) ([]internal.ImportVolumeAttachmentPlan, error) {
	if len(plans) == 0 {
		return nil, nil
	}

	machineNames := transform.Slice(plans, func(in domainstorage.ImportVolumeAttachmentPlan) string {
		return in.MachineID
	})

	machineNetNodes, _, err := s.st.GetNetNodeUUIDsByMachineOrUnitName(ctx, machineNames, []string{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Break the plans into two groups, one which has a block device
	// already imported, and one that does not.
	existingPlans := make([]internal.ImportVolumeAttachmentPlan, 0)
	for _, plan := range plans {
		var (
			netNodeUUID string
			ok          bool
		)
		if plan.MachineID != "" {
			netNodeUUID, ok = machineNetNodes[plan.MachineID]
			if !ok {
				return nil, errors.Errorf("network node uuid for machine %q not found", plan.MachineID)
			}
		}
		uuid, err := domainstorage.NewVolumeAttachmentUUID()
		if err != nil {
			return nil, errors.Capture(err)
		}
		deviceType, err := domainstorage.ParseVolumeDeviceType(plan.DeviceType)
		if err != nil {
			return nil, errors.Capture(err)
		}
		existingPlans = append(existingPlans, internal.ImportVolumeAttachmentPlan{
			UUID:             uuid.String(),
			LifeID:           life.Alive,
			NetNodeUUID:      netNodeUUID,
			DeviceTypeID:     int(deviceType),
			DeviceAttributes: plan.DeviceAttributes,
		})
	}

	return existingPlans, nil
}
