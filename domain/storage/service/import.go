// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"maps"
	"slices"

	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	coreblockdevice "github.com/juju/juju/core/blockdevice"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/providertracker"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/blockdevice"
	blockdeviceerrors "github.com/juju/juju/domain/blockdevice/errors"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/network"
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
	// CreateStoragePool creates a new storage pool in the model with the
	// specified args and uuid value.
	//
	// The following errors can be expected:
	// - [storageerrors.PoolAlreadyExists] if a pool with the same name or
	// uuid already exist in the model.
	CreateStoragePool(context.Context, internal.CreateStoragePool) error

	// GetBlockDevicesForMachinesByNetNodeUUIDs returns the BlockDevices for the
	// specified machines. If a machine is not found or dead then it is excluded
	// from the result.
	GetBlockDevicesForMachinesByNetNodeUUIDs(
		ctx context.Context, netNodeUUIDs []network.NetNodeUUID,
	) (map[network.NetNodeUUID][]internal.BlockDevice, error)

	// and unit names provided. If a machine name or unit name is not found then it
	// is excluded from the result.
	GetNetNodeUUIDsByMachineOrUnitName(
		ctx context.Context,
		machines []machine.Name,
		units []unit.Name,
	) (map[machine.Name]network.NetNodeUUID, map[unit.Name]network.NetNodeUUID, error)

	// GetStorageInstanceUUIDsByIDs retrieves the UUIDs of storage instances by
	// their IDs.
	GetStorageInstanceUUIDsByIDs(ctx context.Context, storageIDs []string) (map[string]domainstorage.StorageInstanceUUID, error)

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
		fsArgs []internal.ImportFilesystemArgs,
		attachmentArgs []internal.ImportFilesystemAttachmentArgs,
	) error

	// ImportStorageInstances imports storage instances, storage attachments, and
	// storage unit owners if the unit name is provided.
	ImportStorageInstances(
		ctx context.Context,
		instanceArgs []internal.ImportStorageInstanceArgs,
		attachmentArgs []internal.ImportStorageInstanceAttachmentArgs,
	) error

	// ImportVolumes creates new storage volumes and related database structures.
	ImportVolumes(ctx context.Context, args []internal.ImportVolumeArgs) error

	// SetModelStoragePools replaces the model's recommended storage pools with the
	// supplied set. All existing model storage pool mappings are removed before the
	// new ones are inserted.
	//
	// If any referenced storage pool UUID does not exist in the model, this
	// returns [domainstorageerrors.StoragePoolNotFound]. Supplying an empty slice
	// results in a no-op.
	SetModelStoragePools(ctx context.Context, pools []domainstorage.RecommendedStoragePoolArg) error
}

// StorageImportService defines a service for importing storage entities during
// model import.
type StorageImportService struct {
	st                      StorageImportState
	ephemeralProviderRunner providertracker.EphemeralProviderRunnerGetter[internalstorage.FilesystemModelMigration]
	registryGetter          corestorage.ModelStorageRegistryGetter
	logger                  logger.Logger
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
	return s.importFilesystems(ctx, params)
}

func (s *StorageImportService) importFilesystems(ctx context.Context, params []domainstorage.ImportFilesystemParams) error {
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

	var (
		machineNodes map[machine.Name]network.NetNodeUUID
		unitNodes    map[unit.Name]network.NetNodeUUID
	)
	if len(machines)+len(units) > 0 {
		machineNodes, unitNodes, err = s.st.GetNetNodeUUIDsByMachineOrUnitName(ctx,
			transform.Slice(machines.Values(), func(in string) machine.Name { return machine.Name(in) }),
			transform.Slice(units.Values(), func(in string) unit.Name { return unit.Name(in) }),
		)
		if err != nil {
			return errors.Errorf("retrieving net node UUIDs by machine or unit names: %w", err)
		}
	}

	fsArgs := make([]internal.ImportFilesystemArgs, len(params))
	attachmentArgs := make([]internal.ImportFilesystemAttachmentArgs, 0)
	for i, arg := range params {
		providerScope, ok := poolScopes[arg.PoolName]
		if !ok {
			return errors.Errorf("storage pool %q not found for filesystem %q", arg.PoolName, arg.ID).
				Add(domainstorageerrors.StoragePoolNotFound)
		}

		var storageInstanceUUID domainstorage.StorageInstanceUUID
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

		fsArgs[i] = internal.ImportFilesystemArgs{
			UUID:                fsUUID.String(),
			ID:                  arg.ID,
			Life:                life.Alive,
			SizeInMiB:           arg.SizeInMiB,
			ProviderID:          arg.ProviderID,
			StorageInstanceUUID: storageInstanceUUID.String(),
			Scope:               providerScope,
		}

		for _, attachment := range arg.Attachments {
			attachmentUUID, err := domainstorage.NewFilesystemAttachmentUUID()
			if err != nil {
				return errors.Errorf("generating UUID for filesystem attachment of filesystem %q: %w", arg.ID, err)
			}

			netNodeUUID, err := getAttachmentNetNodeUUID(
				machine.Name(attachment.HostMachineName),
				unit.Name(attachment.HostUnitName),
				machineNodes,
				unitNodes,
			)
			if err != nil {
				return errors.Errorf("getting net node UUID for filesystem attachment of filesystem %q: %w", arg.ID, err)
			}

			attachmentArgs = append(attachmentArgs, internal.ImportFilesystemAttachmentArgs{
				UUID:           attachmentUUID.String(),
				FilesystemUUID: fsUUID.String(),
				NetNodeUUID:    netNodeUUID.String(),
				Scope:          providerScope,
				Life:           life.Alive,
				MountPoint:     attachment.MountPoint,
				ProviderID:     attachment.ProviderID,
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

// ImportFilesystemsCAAS imports filesystems for CAAS models. It differs from
// ImportFilesystemsIAAS in that it must find the persistent volume claim name
// to be used as the attachment ProviderID.
func (s *StorageImportService) ImportFilesystemsCAAS(
	ctx context.Context,
	params []domainstorage.ImportFilesystemParams) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	uidToNames, err := s.getPersistentVolumeClaimIdentifiers(ctx)
	if err != nil {
		return errors.Errorf("getting persistent volume claim identifiers: %w", err)
	}

	newParams := make([]domainstorage.ImportFilesystemParams, len(params))
	copy(newParams, params)
	for i, param := range params {
		newAttach := make([]domainstorage.ImportFilesystemAttachmentsParams, len(param.Attachments))
		copy(newAttach, param.Attachments)
		param.Attachments = newAttach
		for j, attachment := range param.Attachments {
			newProviderID, ok := uidToNames[attachment.ProviderID]
			if !ok {
				return errors.Errorf("persistent volume claim identifier %q not found", attachment.ProviderID)
			}
			newParams[i].Attachments[j].ProviderID = newProviderID
		}
	}

	return s.importFilesystems(ctx, newParams)
}

// ImportVolumes creates new volumes and storage instance volumes.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when any of the args did not pass validation.
// - [domainstorageerrors.ProviderTypeNotFound] if storage provider was not found.
// - [domainstorageerrors.StorageInstanceNotFound] if the storage ID was not found.
// - [domainstorageerrors.StoragePoolNotFound] if any of the storage pools do not exist.
// - [applicationerrors.UnitNotFound] when a host unit name is provided but not
// found in the model.
// - [machineerrors.MachineNotFound] when a host machine name is provided but not
// found in the model.
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
	var poolNames []string
	for _, v := range params {
		if index, seen := slices.BinarySearch(poolNames, v.Pool); !seen {
			poolNames = slices.Insert(poolNames, index, v.Pool)
		}
	}
	poolNamesToSP, err := s.retrieveProviderScopesForPools(ctx, domainstorage.StorageKindBlock, poolNames)
	if err != nil {
		return nil, errors.Capture(err)
	}

	instanceIDs := transform.Slice(params, func(in domainstorage.ImportVolumeParams) string {
		return in.StorageInstanceID
	})
	instanceUUIDsByIDs, err := s.st.GetStorageInstanceUUIDsByIDs(ctx, instanceIDs)
	if err != nil {
		return nil, errors.Capture(err)
	}

	machineNetNodes, err := s.getAllNetNodeUUIDsForImportVolumeArgs(ctx, params)
	if err != nil {
		return nil, errors.Capture(err)
	}

	mach := slices.Collect(maps.Values(machineNetNodes))
	blockDevicesByNetNode, err := s.st.GetBlockDevicesForMachinesByNetNodeUUIDs(ctx, mach)
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
		storageInstanceUUID, ok := instanceUUIDsByIDs[in.StorageInstanceID]
		if !ok {
			return internal.ImportVolumeArgs{}, errors.Errorf("storage instance %q not found",
				in.StorageInstanceID).Add(domainstorageerrors.StorageInstanceNotFound)
		}

		volumeAttachments, err := s.transformVolumeAttachments(
			in.Attachments,
			machineNetNodes,
			blockDevicesByNetNode,
		)
		if err != nil {
			return internal.ImportVolumeArgs{}, errors.Errorf("transforming volume attachments: %w", err)
		}

		volumeAttachmentPlans, err := s.transformVolumeAttachmentPlans(in.AttachmentPlans, machineNetNodes)
		if err != nil {
			return internal.ImportVolumeArgs{}, errors.Errorf("transforming volume attachment plans: %w", err)
		}

		return internal.ImportVolumeArgs{
			UUID:                volumeUUID,
			ID:                  in.ID,
			LifeID:              life.Alive,
			StorageInstanceUUID: storageInstanceUUID,
			StorageInstanceID:   in.StorageInstanceID,
			Provisioned:         in.Provisioned,
			ProvisionScopeID:    provisionScope,
			SizeMiB:             in.SizeMiB,
			HardwareID:          in.HardwareID,
			WWN:                 in.WWN,
			ProviderID:          in.ProviderID,
			Persistent:          in.Persistent,
			Attachments:         volumeAttachments,
			AttachmentPlans:     volumeAttachmentPlans,
		}, nil
	})
}

func (s *StorageImportService) getAllNetNodeUUIDsForImportVolumeArgs(
	ctx context.Context,
	params []domainstorage.ImportVolumeParams,
) (map[machine.Name]network.NetNodeUUID, error) {
	// Ensure we have a unique set of machines, with no empty strings.
	machineNames := set.NewStrings()
	for _, param := range params {
		for _, attachment := range param.Attachments {
			machineNames.Add(attachment.HostMachineName)
		}
		for _, plan := range param.AttachmentPlans {
			machineNames.Add(plan.HostMachineName)
		}
	}
	if machineNames.Size() == 0 {
		return nil, nil
	}
	machineNetNodeUUIDS, _, err := s.st.GetNetNodeUUIDsByMachineOrUnitName(
		ctx,
		transform.Slice(machineNames.Values(), func(in string) machine.Name { return machine.Name(in) }),
		nil,
	)
	if err != nil {
		return nil, errors.Errorf("getting net node uuids: %w", err)
	}
	if len(machineNetNodeUUIDS) != machineNames.Size() {
		return nil, errors.Errorf("not all machines found")
	}
	return machineNetNodeUUIDS, nil
}

func (s *StorageImportService) transformVolumeAttachments(
	attachments []domainstorage.ImportVolumeAttachmentParams,
	machineNetNodeUUIDS map[machine.Name]network.NetNodeUUID,
	blockDevicesByNetNode map[network.NetNodeUUID][]internal.BlockDevice,
) ([]internal.ImportVolumeAttachmentArgs, error) {
	if len(attachments) == 0 {
		return nil, nil
	}

	existingAttachments := make([]internal.ImportVolumeAttachmentArgs, len(attachments))
	for i, attachment := range attachments {
		uuid, err := domainstorage.NewVolumeAttachmentUUID()
		if err != nil {
			return nil, errors.Capture(err)
		}

		netNodeUUID, err := getAttachmentNetNodeUUID(
			machine.Name(attachment.HostMachineName),
			"",
			machineNetNodeUUIDS,
			nil,
		)
		if err != nil {
			return nil, errors.Capture(err)
		}

		iva := internal.ImportVolumeAttachmentArgs{
			UUID:        uuid,
			LifeID:      life.Alive,
			NetNodeUUID: netNodeUUID,
			ReadOnly:    attachment.ReadOnly,
		}

		attachmentBlockDevice := attachment.CoreBlockDevice()
		if blockdevice.IsEmpty(attachmentBlockDevice) {
			existingAttachments = append(existingAttachments, iva)
			continue
		}

		machineBlockDevices, ok := blockDevicesByNetNode[netNodeUUID]
		if !ok {
			return nil, errors.Errorf("block devices for net node %q not found", netNodeUUID).Add(blockdeviceerrors.BlockDeviceNotFound)
		}

		if blockDeviceUUID, ok := findBlockDeviceUUID(attachmentBlockDevice, machineBlockDevices); ok {
			iva.BlockDeviceUUID = blockDeviceUUID
			existingAttachments[i] = iva
			continue
		}
		return nil, errors.Errorf("block device for machine %q not found", attachment.HostMachineName).Add(blockdeviceerrors.BlockDeviceNotFound)
	}

	return existingAttachments, nil
}

func getAttachmentNetNodeUUID(
	machineName machine.Name,
	unitName unit.Name,
	machineNetNodeUUIDS map[machine.Name]network.NetNodeUUID,
	unitNetNodeUUIDs map[unit.Name]network.NetNodeUUID,
) (network.NetNodeUUID, error) {
	var (
		machineNetNodeUUID, unitNetNodeUUID network.NetNodeUUID
		ok                                  bool
	)
	if machineName != "" {
		machineNetNodeUUID, ok = machineNetNodeUUIDS[machineName]
		if !ok {
			return "", errors.Errorf("network node uuid for machine %q not found", machineName).Add(machineerrors.MachineNotFound)
		}
	}
	if unitName != "" {
		unitNetNodeUUID, ok = unitNetNodeUUIDs[unitName]
		if !ok {
			return "", errors.Errorf("network node uuid for unit %q not found", unitName).Add(applicationerrors.UnitNotFound)
		}
	}
	if machineNetNodeUUID != "" && unitNetNodeUUID != "" && machineNetNodeUUID != unitNetNodeUUID {
		return "", errors.Errorf("conflicting data unit %q not on machine %q", unitName, machineName)
	}
	if machineNetNodeUUID.String() != "" {
		return machineNetNodeUUID, nil
	}
	return unitNetNodeUUID, nil
}

func findBlockDeviceUUID(
	find coreblockdevice.BlockDevice,
	blockDevices []internal.BlockDevice,
) (blockdevice.BlockDeviceUUID, bool) {
	for _, bd := range blockDevices {
		if blockdevice.SameDevice(find, bd.BlockDevice) {
			return bd.UUID, true
		}
	}
	return "", false
}

func (s *StorageImportService) transformVolumeAttachmentPlans(
	plans []domainstorage.ImportVolumeAttachmentPlanParams,
	machineNetNodeUUIDs map[machine.Name]network.NetNodeUUID,
) ([]internal.ImportVolumeAttachmentPlanArgs, error) {
	if len(plans) == 0 {
		return nil, nil
	}

	existingPlans := make([]internal.ImportVolumeAttachmentPlanArgs, 0)
	for _, plan := range plans {
		netNodeUUID, ok := machineNetNodeUUIDs[machine.Name(plan.HostMachineName)]
		if !ok {
			return nil, errors.Errorf("network node uuid for machine %q not found", plan.HostMachineName).Add(applicationerrors.MachineNotFound)
		}
		uuid, err := domainstorage.NewVolumeAttachmentPlanUUID()
		if err != nil {
			return nil, errors.Capture(err)
		}
		var deviceTypePtr *domainstorage.VolumeDeviceType
		if plan.DeviceType != "" {
			deviceType, err := domainstorage.ParseVolumeDeviceType(plan.DeviceType)
			if err != nil {
				return nil, errors.Capture(err)
			}
			deviceTypePtr = &deviceType
		}
		existingPlans = append(existingPlans, internal.ImportVolumeAttachmentPlanArgs{
			UUID:             uuid,
			LifeID:           life.Alive,
			NetNodeUUID:      netNodeUUID,
			DeviceTypeID:     deviceTypePtr,
			DeviceAttributes: plan.DeviceAttributes,
		})
	}

	return existingPlans, nil
}

// getPersistentVolumeClaimIdentifiers gets a map of UID to Name from
// the slice of PersistentVolumeClaimIdentifiers data. Intended for use
// with kubernetes only.
func (s *StorageImportService) getPersistentVolumeClaimIdentifiers(ctx context.Context) (map[string]string, error) {

	var data []internalstorage.PersistentVolumeClaimIdentifiers
	err := s.ephemeralProviderRunner(ctx, func(ctx context.Context, provider internalstorage.FilesystemModelMigration) error {
		var err error
		data, err = provider.GetPersistentVolumeClaimIdentifiers(ctx)
		if err != nil {
			return errors.Errorf(
				"from provider: %w", err,
			)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	uidToNames := transform.SliceToMap(data, func(in internalstorage.PersistentVolumeClaimIdentifiers) (string, string) {
		return in.UID, in.Name
	})

	return uidToNames, nil
}
