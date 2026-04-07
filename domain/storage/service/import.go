// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
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
	SetModelStoragePools(ctx context.Context, pools []internal.RecommendedStoragePoolArg) error
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

// ImportStoragePools creates new storage pools from the supplied
// [domainstorage.UserStoragePoolParams]. Provider default and recommended
// storage pools are discovered and computed internally based on the provider
// types referenced by these parameters.
// This is slightly different to [CreateStoragePools] because:
//  1. the storage pool name validation uses a legacy regex and,
//  2. only user-defined pools are provided explicitly; provider default and
//     recommended pools are derived by the service.
//
// The following errors may be returned:
// - [domainstorageerrors.StoragePoolNameInvalid] when the supplied storage
// pool name is considered invalid or empty.
// - [domainstorageerrors.ProviderTypeInvalid] when the supplied provider
// type value is invalid for further use.
// - [domainstorageerrors.ProviderTypeNotFound] when the supplied provider
// type is not known to the controller.
// - [domainstorageerrors.StoragePoolAlreadyExists] when a storage pool for the
// supplied name already exists in the model.
// - [domainstorageerrors.StoragePoolAttributeInvalid] when one of the supplied
// storage pool attributes is invalid.
func (s *StorageImportService) ImportStoragePools(
	ctx context.Context,
	userPools []domainstorage.UserStoragePoolParams,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	pools, recommendedPools, err := s.getStoragePoolsToImport(ctx, userPools)
	if err != nil {
		return errors.Capture(err)
	}

	for _, pool := range pools {
		err := validateStoragePoolCreation(ctx, s.registryGetter, pool.Name,
			domainstorage.ProviderType(pool.Type), pool.Attrs,
			domainstorage.IsValidStoragePoolNameWithLegacy)
		if err != nil {
			return err
		}
		err = pool.UUID.Validate()
		if err != nil {
			return errors.Errorf("storage pool %q UUID is not valid: %w", pool.Name, err)
		}

		coercedAttrs := transform.Map(
			pool.Attrs,
			func(k string, v any) (string, string) {
				return k, fmt.Sprint(v)
			},
		)

		arg := internal.CreateStoragePool{
			Attrs:        coercedAttrs,
			Name:         pool.Name,
			Origin:       pool.Origin,
			ProviderType: domainstorage.ProviderType(pool.Type),
			UUID:         pool.UUID,
		}
		// TODO(adisazhar123): refactor opportunity. Bulk insert.
		err = s.st.CreateStoragePool(ctx, arg)
		if err != nil {
			return errors.Errorf("creating storage pool %q: %w", pool.Name, err)
		}
	}

	err = s.setRecommendedStoragePools(ctx, recommendedPools)
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// getStoragePoolsToImport resolves the full set of storage pools to create during
// model import.
//
// It starts with user-defined storage pools provided as input, ensuring they
// take precedence over provider default pools on name and provider conflicts.
// Provider default pools are then added where safe, followed by resolving any
// recommended storage pools from the registry.
//
// The function returns:
//  1. A slice of storage pools that should be created during import
//  2. A slice of recommended storage pools referencing existing or newly created pools
func (s *StorageImportService) getStoragePoolsToImport(
	ctx context.Context,
	userPools []domainstorage.UserStoragePoolParams,
) (
	[]domainstorage.ImportStoragePoolParams,
	[]domainstorage.RecommendedStoragePoolParams,
	error,
) {
	poolsToCreate := make([]domainstorage.ImportStoragePoolParams, 0)
	// We first create the list of pools from the migrated models.
	// This is to ensure that the user-defined pools from the import are chosen
	// should the name conflicts with provider default pools.
	for _, v := range userPools {
		uuid, err := domainstorage.NewStoragePoolUUID()
		if err != nil {
			return nil, nil, errors.Errorf("generating uuid for user pool %q: %w", v.Name, err)
		}
		poolsToCreate = append(poolsToCreate, domainstorage.ImportStoragePoolParams{
			UUID:   uuid,
			Name:   v.Name,
			Origin: domainstorage.StoragePoolOriginUser,
			Type:   v.Provider,
			Attrs:  v.Attributes,
		})
	}

	modelStorageRegistry, err := s.registryGetter.GetStorageRegistry(ctx)
	if err != nil {
		return nil, nil, errors.Errorf(
			"getting storage provider registry for model: %w", err,
		)
	}

	providerTypes, err := modelStorageRegistry.StorageProviderTypes()
	if err != nil {
		return nil, nil, errors.Errorf(
			"getting storage provider types for model storage registry: %w", err,
		)
	}

	for _, providerType := range providerTypes {
		provider, err := modelStorageRegistry.StorageProvider(providerType)
		if err != nil {
			return nil, nil, errors.Errorf(
				"getting storage provider %q from registry: %w",
				providerType, err,
			)
		}

		providerDefaultPools := provider.DefaultPools()
		for _, providerDefaultPool := range providerDefaultPools {
			providerDefault, err := s.defaultPoolForImport(ctx, poolsToCreate, providerDefaultPool)
			if err != nil {
				return nil, nil, err
			}
			if providerDefault != nil {
				poolsToCreate = append(poolsToCreate, *providerDefault)
			}
		}
	}

	defaultPools, recommendedPools, err := s.getRecommendedStoragePools(poolsToCreate,
		modelStorageRegistry)
	if err != nil {
		return nil, nil, errors.Errorf("getting recommended storage pools: %w", err)
	}
	poolsToCreate = append(poolsToCreate, defaultPools...)
	return poolsToCreate, recommendedPools, nil
}

// setRecommendedStoragePools persists the set of recommended storage pools
// that are to be used for a model.
func (s *StorageImportService) setRecommendedStoragePools(ctx context.Context,
	pools []domainstorage.RecommendedStoragePoolParams) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	poolArgs := make([]internal.RecommendedStoragePoolArg, len(pools))
	for i, pool := range pools {
		poolArgs[i] = internal.RecommendedStoragePoolArg{
			StoragePoolUUID: pool.StoragePoolUUID,
			StorageKind:     pool.StorageKind,
		}
	}

	return s.st.SetModelStoragePools(ctx, poolArgs)
}

func (s *StorageImportService) defaultPoolForImport(
	ctx context.Context,
	existingPools []domainstorage.ImportStoragePoolParams,
	config *internalstorage.Config) (*domainstorage.ImportStoragePoolParams, error) {
	// A storage pool with a duplicate provider and name already exists.
	// We don't want to choose this default pool to avoid conflicting with
	// the existing one.
	if slices.ContainsFunc(existingPools, func(pool domainstorage.ImportStoragePoolParams) bool {
		return pool.Name == config.Name() && pool.Type == config.Provider().String()
	}) {
		return nil, nil
	}
	name := config.Name()
	provider := config.Provider().String()
	uuid, err := domainstorage.GetProviderDefaultStoragePoolUUID(
		name, provider)

	// Logic carried over from [SeedDefaultStoragePools] func
	// in [github.com/juju/juju/domain/model/service.ProviderModelService].
	if errors.Is(err, coreerrors.NotFound) {
		// This happens when the default pool is not supported yet by the
		// storage domain. This shouldn't stop the model from being created.
		// Instead we log the problem.
		s.logger.Warningf(
			ctx,
			"storage provider %q default pool %q is not recognised, adding to model with generated uuid.",
			provider,
			name,
		)
		return nil, nil
	} else if err != nil {
		return nil, errors.Errorf(
			"getting storage pool uuid for default provider %q pool %q",
			provider,
			name,
		)
	}

	// The provider default pool doesn't conflict with the user-defined pools, it's safe
	// to return it for creation.
	return &domainstorage.ImportStoragePoolParams{
		UUID:   uuid,
		Name:   name,
		Origin: domainstorage.StoragePoolOriginProviderDefault,
		Type:   provider,
		Attrs:  config.Attrs(),
	}, nil
}

// getRecommendedStoragePools determines the recommended storage pools
// for each supported storage kind and resolves which of them need to be
// created during import.
//
// For each recommended pool provided by the registry, the function:
//   - Resolves the pool's UUID using provider defaults
//   - Checks for duplicates against existingPools using UUID, and then
//     pool name and provider type
//   - Appends a pool to the creation list only if it does not already exist
//     and does not conflict with a user-defined pool
//
// The returned values are:
//  1. A slice of [ImportStoragePoolParams] describing provider default pools that
//     should be created during import
//  2. A slice of [RecommendedStoragePoolParams] mapping storage kinds to the
//     resolved storage pool UUIDs, which may refer to pools of the provider.
func (s *StorageImportService) getRecommendedStoragePools(
	existingPools []domainstorage.ImportStoragePoolParams,
	reg internalstorage.ProviderRegistry,
) (
	[]domainstorage.ImportStoragePoolParams,
	[]domainstorage.RecommendedStoragePoolParams,
	error,
) {
	poolsToCreate := make([]domainstorage.ImportStoragePoolParams, 0)
	recommendedPools := make([]domainstorage.RecommendedStoragePoolParams, 0)

	// ensureStoragePool ensures that a provider-recommended storage pool is
	// accounted for during import.
	//
	// It checks the given configuration against existingPools to avoid duplicates.
	// If an identical pool already exists (by UUID), its UUID is returned.
	// If a user-defined pool with the same name and provider exists, no pool is
	// added and an empty UUID is returned.
	//
	// Otherwise, the pool is appended to [poolsToCreate] and its generated UUID
	// is returned.
	ensurePool := func(cfg *internalstorage.Config) (domainstorage.StoragePoolUUID, error) {
		// Get the UUID of the given pool so that we can later
		// check for duplication within [existingPools].
		uuid, err := domainstorage.GenerateProviderDefaultStoragePoolUUIDWithDefaults(
			cfg.Name(),
			cfg.Provider().String(),
		)
		if err != nil {
			return "", errors.Capture(err)
		}

		// Check if the UUID matches an existing pool that is NOT a user-defined pool.
		// This means that we can recommend a provider default pool for the model.
		index := slices.IndexFunc(existingPools, func(e domainstorage.ImportStoragePoolParams) bool {
			return e.UUID == uuid
		},
		)
		// The given pool exists in [existingPools]. We don't want to add a duplicate
		// so return early.
		if index != -1 &&
			existingPools[index].Origin == domainstorage.StoragePoolOriginProviderDefault {
			return (existingPools)[index].UUID, nil
		} else if index != -1 &&
			existingPools[index].Origin == domainstorage.StoragePoolOriginUser {
			// The chances of a recommended provider default pool UUID matching a user-defined
			// pool UUID is slim to none. But we add it here for defensive programming.
			return "", nil
		}

		// We don't want to add a user-defined pool for recommendation and/or creation.
		// We have no way of guaranteeing that a "foo" user-defined pool is the same
		// "foo" provider default pool.
		if slices.ContainsFunc(existingPools, func(pool domainstorage.ImportStoragePoolParams) bool {
			return pool.Name == cfg.Name() &&
				pool.Type == cfg.Provider().String() &&
				pool.Origin == domainstorage.StoragePoolOriginUser
		}) {
			return "", nil
		}
		poolsToCreate = append(poolsToCreate, domainstorage.ImportStoragePoolParams{
			UUID:   uuid,
			Name:   cfg.Name(),
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Type:   cfg.Provider().String(),
			Attrs:  cfg.Attrs(),
		})
		return uuid, nil
	}

	// Get filesystem recommendation.
	poolCfg := reg.RecommendedPoolForKind(internalstorage.StorageKindFilesystem)
	if poolCfg != nil {
		uuid, err := ensurePool(poolCfg)
		if err != nil {
			return nil, nil, errors.Capture(err)
		}
		if uuid != "" {
			recommendedPools = append(recommendedPools, domainstorage.RecommendedStoragePoolParams{
				StorageKind:     domainstorage.StorageKindFilesystem,
				StoragePoolUUID: uuid,
			})
		}
	}

	// Get block device recommendation.
	poolCfg = reg.RecommendedPoolForKind(internalstorage.StorageKindBlock)
	if poolCfg != nil {
		uuid, err := ensurePool(poolCfg)
		if err != nil {
			return nil, nil, errors.Capture(err)
		}
		if uuid != "" {
			recommendedPools = append(recommendedPools, domainstorage.RecommendedStoragePoolParams{
				StorageKind:     domainstorage.StorageKindBlock,
				StoragePoolUUID: uuid,
			})
		}
	}

	return poolsToCreate, recommendedPools, nil
}
