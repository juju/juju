// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"

	coreblockdevice "github.com/juju/juju/core/blockdevice"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainblockdevice "github.com/juju/juju/domain/blockdevice"
	domainlife "github.com/juju/juju/domain/life"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// storageProvisioningService is the subset of the domain storage provisioning
// service needed by the model storage adapters.
type storageProvisioningService interface {
	// Volume operations
	GetVolumeUUIDForID(ctx context.Context, volumeID string) (domainstorage.VolumeUUID, error)
	GetVolumeByID(ctx context.Context, volumeID string) (storageprovisioning.Volume, error)
	GetVolumeLife(ctx context.Context, uuid domainstorage.VolumeUUID) (domainlife.Life, error)
	GetVolumeParams(ctx context.Context, uuid domainstorage.VolumeUUID) (storageprovisioning.VolumeParams, error)
	GetVolumeRemovalParams(ctx context.Context, uuid domainstorage.VolumeUUID) (storageprovisioning.VolumeRemovalParams, error)
	SetVolumeProvisionedInfo(ctx context.Context, volumeID string, info storageprovisioning.VolumeProvisionedInfo) error

	// Filesystem operations
	GetFilesystemUUIDForID(ctx context.Context, filesystemID string) (domainstorage.FilesystemUUID, error)
	GetFilesystemForID(ctx context.Context, filesystemID string) (storageprovisioning.Filesystem, error)
	GetFilesystemLife(ctx context.Context, uuid domainstorage.FilesystemUUID) (domainlife.Life, error)
	GetFilesystemParams(ctx context.Context, uuid domainstorage.FilesystemUUID) (storageprovisioning.FilesystemParams, error)
	GetFilesystemRemovalParams(ctx context.Context, uuid domainstorage.FilesystemUUID) (storageprovisioning.FilesystemRemovalParams, error)
	SetFilesystemProvisionedInfo(ctx context.Context, filesystemID string, info storageprovisioning.FilesystemProvisionedInfo) error

	// Volume attachment operations
	GetVolumeAttachmentUUIDForVolumeIDMachine(ctx context.Context, volumeID string, machineUUID machine.UUID) (domainstorage.VolumeAttachmentUUID, error)
	GetVolumeAttachment(ctx context.Context, uuid domainstorage.VolumeAttachmentUUID) (storageprovisioning.VolumeAttachment, error)
	GetVolumeAttachmentLife(ctx context.Context, uuid domainstorage.VolumeAttachmentUUID) (domainlife.Life, error)
	GetVolumeAttachmentParams(ctx context.Context, uuid domainstorage.VolumeAttachmentUUID) (storageprovisioning.VolumeAttachmentParams, error)
	SetVolumeAttachmentProvisionedInfo(ctx context.Context, uuid domainstorage.VolumeAttachmentUUID, info storageprovisioning.VolumeAttachmentProvisionedInfo) error

	// Filesystem attachment operations
	GetFilesystemAttachmentUUIDForFilesystemIDMachine(ctx context.Context, filesystemID string, machineUUID machine.UUID) (domainstorage.FilesystemAttachmentUUID, error)
	GetFilesystemAttachmentUUIDForFilesystemIDUnit(ctx context.Context, filesystemID string, unitUUID coreunit.UUID) (domainstorage.FilesystemAttachmentUUID, error)
	GetFilesystemAttachmentForMachine(ctx context.Context, filesystemID string, machineUUID machine.UUID) (storageprovisioning.FilesystemAttachment, error)
	GetFilesystemAttachmentForUnit(ctx context.Context, filesystemID string, unitUUID coreunit.UUID) (storageprovisioning.FilesystemAttachment, error)
	GetFilesystemAttachmentLife(ctx context.Context, uuid domainstorage.FilesystemAttachmentUUID) (domainlife.Life, error)
	GetFilesystemAttachmentParams(ctx context.Context, uuid domainstorage.FilesystemAttachmentUUID) (storageprovisioning.FilesystemAttachmentParams, error)
	SetFilesystemAttachmentProvisionedInfoForMachine(ctx context.Context, filesystemID string, machineUUID machine.UUID, info storageprovisioning.FilesystemAttachmentProvisionedInfo) error
	SetFilesystemAttachmentProvisionedInfoForUnit(ctx context.Context, filesystemID string, unitUUID coreunit.UUID, info storageprovisioning.FilesystemAttachmentProvisionedInfo) error

	// Attachment ID lookups
	GetVolumeAttachmentIDs(ctx context.Context, volumeAttachmentUUIDs []string) (map[string]storageprovisioning.VolumeAttachmentID, error)
	GetFilesystemAttachmentIDs(ctx context.Context, filesystemAttachmentUUIDs []string) (map[string]storageprovisioning.FilesystemAttachmentID, error)

	// Block device for volume attachment
	GetBlockDeviceForVolumeAttachment(ctx context.Context, uuid domainstorage.VolumeAttachmentUUID) (domainblockdevice.BlockDeviceUUID, error)

	// Model resource tags
	GetStorageResourceTagsForModel(ctx context.Context) (map[string]string, error)

	// Watchers (model-scoped)
	WatchModelProvisionedVolumes(ctx context.Context) (watcher.StringsWatcher, error)
	WatchModelProvisionedFilesystems(ctx context.Context) (watcher.StringsWatcher, error)
	WatchModelProvisionedVolumeAttachments(ctx context.Context) (watcher.StringsWatcher, error)
	WatchModelProvisionedFilesystemAttachments(ctx context.Context) (watcher.StringsWatcher, error)

	// Watchers (machine-scoped)
	WatchMachineProvisionedVolumes(ctx context.Context, machineUUID machine.UUID) (watcher.StringsWatcher, error)
	WatchMachineProvisionedFilesystems(ctx context.Context, machineUUID machine.UUID) (watcher.StringsWatcher, error)
	WatchMachineProvisionedVolumeAttachments(ctx context.Context, machineUUID machine.UUID) (watcher.StringsWatcher, error)
	WatchMachineProvisionedFilesystemAttachments(ctx context.Context, machineUUID machine.UUID) (watcher.StringsWatcher, error)
	WatchVolumeAttachmentPlans(ctx context.Context, machineUUID machine.UUID) (watcher.StringsWatcher, error)

	// Volume attachment plans
	GetVolumeAttachmentPlanUUIDForVolumeIDMachine(ctx context.Context, volumeID string, machineUUID machine.UUID) (domainstorage.VolumeAttachmentPlanUUID, error)
	GetVolumeAttachmentPlan(ctx context.Context, uuid domainstorage.VolumeAttachmentPlanUUID) (storageprovisioning.VolumeAttachmentPlan, error)
	CreateVolumeAttachmentPlan(ctx context.Context, attachmentUUID domainstorage.VolumeAttachmentUUID, deviceType domainstorage.VolumeDeviceType, attrs map[string]string) (domainstorage.VolumeAttachmentPlanUUID, error)
	SetVolumeAttachmentPlanProvisionedInfo(ctx context.Context, uuid domainstorage.VolumeAttachmentPlanUUID, info storageprovisioning.VolumeAttachmentPlanProvisionedInfo) error
	SetVolumeAttachmentPlanProvisionedBlockDevice(ctx context.Context, uuid domainstorage.VolumeAttachmentPlanUUID, blockDeviceUUID domainblockdevice.BlockDeviceUUID) error
}

// machineService is the subset of the domain machine service needed by the
// model storage adapters.
type machineService interface {
	GetMachineUUID(ctx context.Context, machineName machine.Name) (machine.UUID, error)
	GetInstanceID(ctx context.Context, machineUUID machine.UUID) (instance.Id, error)
	WatchMachineCloudInstances(ctx context.Context, machineUUID machine.UUID) (watcher.NotifyWatcher, error)
}

// applicationService is the subset of the domain application service needed
// by the model storage adapters.
type applicationService interface {
	GetUnitUUID(ctx context.Context, unitName coreunit.Name) (coreunit.UUID, error)
}

// removalService is the subset of the domain removal service needed by the
// model storage adapters.
type removalService interface {
	RemoveDeadVolume(ctx context.Context, uuid domainstorage.VolumeUUID) error
	RemoveDeadFilesystem(ctx context.Context, uuid domainstorage.FilesystemUUID) error
	MarkVolumeAttachmentAsDead(ctx context.Context, uuid domainstorage.VolumeAttachmentUUID) error
	MarkFilesystemAttachmentAsDead(ctx context.Context, uuid domainstorage.FilesystemAttachmentUUID) error
	MarkVolumeAttachmentPlanAsDead(ctx context.Context, uuid domainstorage.VolumeAttachmentPlanUUID) error
}

// storageStatusService is the subset of the domain status service needed by
// the model storage adapters.
type storageStatusService interface {
	SetVolumeStatus(ctx context.Context, volumeID string, sInfo status.StatusInfo) error
	SetFilesystemStatus(ctx context.Context, filesystemID string, sInfo status.StatusInfo) error
}

// blockDeviceService is the subset of the domain block device service needed
// by the model storage adapters.
type blockDeviceService interface {
	WatchBlockDevicesForMachine(ctx context.Context, machineUUID machine.UUID) (watcher.NotifyWatcher, error)
	GetBlockDevice(ctx context.Context, uuid domainblockdevice.BlockDeviceUUID) (coreblockdevice.BlockDevice, error)
	MatchOrCreateBlockDevice(ctx context.Context, machineUUID machine.UUID, device coreblockdevice.BlockDevice) (domainblockdevice.BlockDeviceUUID, error)
}

// modelStorageAdapter implements VolumeAccessor, FilesystemAccessor,
// LifecycleManager, MachineAccessor, and StatusSetter by reading from
// domain services directly instead of through an API facade.
type modelStorageAdapter struct {
	storageSvc     storageProvisioningService
	machineSvc     machineService
	appSvc         applicationService
	removalSvc     removalService
	statusSvc      storageStatusService
	blockDeviceSvc blockDeviceService
}

// VolumeAccessor implementation

func (a *modelStorageAdapter) WatchBlockDevices(ctx context.Context, m names.MachineTag) (watcher.NotifyWatcher, error) {
	machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machine.Name(m.Id()))
	if err != nil {
		return nil, errors.Trace(err)
	}
	return a.blockDeviceSvc.WatchBlockDevicesForMachine(ctx, machineUUID)
}

func (a *modelStorageAdapter) WatchVolumes(ctx context.Context, scope names.Tag) (watcher.StringsWatcher, error) {
	switch scope := scope.(type) {
	case names.ModelTag:
		return a.storageSvc.WatchModelProvisionedVolumes(ctx)
	case names.MachineTag:
		machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machine.Name(scope.Id()))
		if err != nil {
			return nil, errors.Trace(err)
		}
		return a.storageSvc.WatchMachineProvisionedVolumes(ctx, machineUUID)
	default:
		return nil, internalerrors.Errorf("unsupported scope %T", scope).Add(coreerrors.NotSupported)
	}
}

func (a *modelStorageAdapter) WatchVolumeAttachments(ctx context.Context, scope names.Tag) (watcher.MachineStorageIDsWatcher, error) {
	var sourceWatcher watcher.StringsWatcher
	switch scope := scope.(type) {
	case names.ModelTag:
		w, err := a.storageSvc.WatchModelProvisionedVolumeAttachments(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		sourceWatcher = w
	case names.MachineTag:
		machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machine.Name(scope.Id()))
		if err != nil {
			return nil, errors.Trace(err)
		}
		w, err := a.storageSvc.WatchMachineProvisionedVolumeAttachments(ctx, machineUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		sourceWatcher = w
	default:
		return nil, internalerrors.Errorf("unsupported scope %T", scope).Add(coreerrors.NotSupported)
	}
	return newAttachmentIDWatcher(sourceWatcher, func(ctx context.Context, ids ...string) ([]watcher.MachineStorageID, error) {
		attachmentIDs, err := a.storageSvc.GetVolumeAttachmentIDs(ctx, ids)
		if err != nil {
			return nil, errors.Trace(err)
		}
		var out []watcher.MachineStorageID
		for _, id := range attachmentIDs {
			if id.MachineName == nil && id.UnitName == nil {
				continue
			}
			msid := watcher.MachineStorageID{
				AttachmentTag: names.NewVolumeTag(id.VolumeID).String(),
			}
			if id.MachineName != nil {
				msid.MachineTag = names.NewMachineTag(id.MachineName.String()).String()
			} else if id.UnitName != nil {
				msid.MachineTag = names.NewUnitTag(id.UnitName.String()).String()
			}
			out = append(out, msid)
		}
		return out, nil
	})
}

func (a *modelStorageAdapter) WatchVolumeAttachmentPlans(ctx context.Context, scope names.Tag) (watcher.MachineStorageIDsWatcher, error) {
	switch scope := scope.(type) {
	case names.MachineTag:
		machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machine.Name(scope.Id()))
		if err != nil {
			return nil, errors.Trace(err)
		}
		w, err := a.storageSvc.WatchVolumeAttachmentPlans(ctx, machineUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return newAttachmentIDWatcher(w, func(ctx context.Context, volumeIDs ...string) ([]watcher.MachineStorageID, error) {
			if len(volumeIDs) == 0 {
				return nil, nil
			}
			out := make([]watcher.MachineStorageID, 0, len(volumeIDs))
			for _, volumeID := range volumeIDs {
				if !names.IsValidVolume(volumeID) {
					continue
				}
				out = append(out, watcher.MachineStorageID{
					MachineTag:    scope.String(),
					AttachmentTag: names.NewVolumeTag(volumeID).String(),
				})
			}
			return out, nil
		})
	default:
		return nil, internalerrors.Errorf("unsupported scope %T", scope).Add(coreerrors.NotSupported)
	}
}

func (a *modelStorageAdapter) Volumes(ctx context.Context, tags []names.VolumeTag) ([]params.VolumeResult, error) {
	results := make([]params.VolumeResult, len(tags))
	for i, tag := range tags {
		vol, err := a.storageSvc.GetVolumeByID(ctx, tag.Id())
		if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
			results[i].Error = &params.Error{Message: errors.Errorf("volume %q not found", tag.Id()).Error(), Code: params.CodeNotFound}
			continue
		}
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		if vol.SizeMiB == 0 {
			results[i].Error = &params.Error{Message: errors.Errorf("volume %q is not provisioned", tag.Id()).Error(), Code: params.CodeNotProvisioned}
			continue
		}
		results[i].Result = params.Volume{
			VolumeTag: tag.String(),
			Info: params.VolumeInfo{
				ProviderId: vol.ProviderID,
				HardwareId: vol.HardwareID,
				WWN:        vol.WWN,
				Persistent: vol.Persistent,
				SizeMiB:    vol.SizeMiB,
			},
		}
	}
	return results, nil
}

func (a *modelStorageAdapter) VolumeBlockDevices(ctx context.Context, ids []params.MachineStorageId) ([]params.BlockDeviceResult, error) {
	results := make([]params.BlockDeviceResult, len(ids))
	for i, id := range ids {
		volumeTag, err := names.ParseVolumeTag(id.AttachmentTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		machineTag, err := names.ParseMachineTag(id.MachineTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machine.Name(machineTag.Id()))
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		attachmentUUID, err := a.storageSvc.GetVolumeAttachmentUUIDForVolumeIDMachine(ctx, volumeTag.Id(), machineUUID)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		blockDevUUID, err := a.storageSvc.GetBlockDeviceForVolumeAttachment(ctx, attachmentUUID)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		bd, err := a.blockDeviceSvc.GetBlockDevice(ctx, blockDevUUID)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		if bd.DeviceName == "" || len(bd.DeviceLinks) == 0 {
			results[i].Error = &params.Error{
				Message: errors.Errorf(
					"volume attachment %q on machine %q is not provisioned",
					volumeTag.Id(), machineTag.Id(),
				).Error(),
				Code: params.CodeNotProvisioned,
			}
			continue
		}
		var provenance params.BlockDeviceProvenance
		switch bd.Provenance {
		case coreblockdevice.ProviderProvenance:
			provenance = params.BlockDeviceProvenanceProvider
		case coreblockdevice.MachineProvenance:
			provenance = params.BlockDeviceProvenanceMachine
		default:
			results[i].Error = &params.Error{
				Message: errors.Errorf(
					"unexpected provenance value: %v", bd.Provenance,
				).Error(),
			}
			continue
		}
		results[i].Result = params.BlockDevice{
			DeviceName:     bd.DeviceName,
			DeviceLinks:    bd.DeviceLinks,
			Label:          bd.FilesystemLabel,
			UUID:           bd.FilesystemUUID,
			HardwareId:     bd.HardwareId,
			WWN:            bd.WWN,
			BusAddress:     bd.BusAddress,
			SizeMiB:        bd.SizeMiB,
			FilesystemType: bd.FilesystemType,
			InUse:          bd.InUse,
			MountPoint:     bd.MountPoint,
			SerialId:       bd.SerialId,
			Provenance:     provenance,
		}
	}
	return results, nil
}

func (a *modelStorageAdapter) VolumeAttachments(ctx context.Context, ids []params.MachineStorageId) ([]params.VolumeAttachmentResult, error) {
	results := make([]params.VolumeAttachmentResult, len(ids))
	for i, id := range ids {
		volumeTag, err := names.ParseVolumeTag(id.AttachmentTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		machineTag, err := names.ParseMachineTag(id.MachineTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machine.Name(machineTag.Id()))
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		attachmentUUID, err := a.storageSvc.GetVolumeAttachmentUUIDForVolumeIDMachine(ctx, volumeTag.Id(), machineUUID)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		va, err := a.storageSvc.GetVolumeAttachment(ctx, attachmentUUID)
		if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
			results[i].Error = &params.Error{Message: errors.Errorf("volume attachment %q on %q not found", volumeTag.Id(), machineTag.Id()).Error(), Code: params.CodeNotFound}
			continue
		}
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		if va.BlockDeviceName == "" || len(va.BlockDeviceLinks) == 0 {
			results[i].Error = &params.Error{Message: errors.Errorf("volume %q is not provisioned", volumeTag.Id()).Error(), Code: params.CodeNotProvisioned}
			continue
		}
		results[i].Result = params.VolumeAttachment{
			VolumeTag:  volumeTag.String(),
			MachineTag: machineTag.String(),
			Info: params.VolumeAttachmentInfo{
				DeviceName: va.BlockDeviceName,
				DeviceLink: domainblockdevice.IDLink(va.BlockDeviceLinks),
				BusAddress: va.BlockDeviceBusAddress,
				ReadOnly:   va.ReadOnly,
			},
		}
	}
	return results, nil
}

func (a *modelStorageAdapter) VolumeAttachmentPlans(ctx context.Context, ids []params.MachineStorageId) ([]params.VolumeAttachmentPlanResult, error) {
	results := make([]params.VolumeAttachmentPlanResult, len(ids))
	for i, id := range ids {
		volumeTag, err := names.ParseVolumeTag(id.AttachmentTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		machineTag, err := names.ParseMachineTag(id.MachineTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machine.Name(machineTag.Id()))
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		planUUID, err := a.storageSvc.GetVolumeAttachmentPlanUUIDForVolumeIDMachine(ctx, volumeTag.Id(), machineUUID)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		plan, err := a.storageSvc.GetVolumeAttachmentPlan(ctx, planUUID)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		planLife, err := plan.Life.Value()
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		results[i].Result = params.VolumeAttachmentPlan{
			VolumeTag:  volumeTag.String(),
			MachineTag: machineTag.String(),
			Life:       planLife,
			PlanInfo: params.VolumeAttachmentPlanInfo{
				DeviceType:       plan.DeviceType.String(),
				DeviceAttributes: plan.DeviceAttributes,
			},
		}
	}
	return results, nil
}

func (a *modelStorageAdapter) VolumeParams(ctx context.Context, tags []names.VolumeTag) ([]params.VolumeParamsResult, error) {
	results := make([]params.VolumeParamsResult, len(tags))
	volModelTags, err := a.storageSvc.GetStorageResourceTagsForModel(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for i, tag := range tags {
		uuid, err := a.storageSvc.GetVolumeUUIDForID(ctx, tag.Id())
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		volParams, err := a.storageSvc.GetVolumeParams(ctx, uuid)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		rval := params.VolumeParams{
			Attributes: make(map[string]any, len(volParams.Attributes)),
			VolumeTag:  tag.String(),
			Provider:   volParams.Provider,
			SizeMiB:    volParams.SizeMiB,
			Tags:       volModelTags,
		}
		for k, v := range volParams.Attributes {
			rval.Attributes[k] = v
		}
		if volParams.VolumeAttachmentUUID != nil {
			vaParams, err := a.storageSvc.GetVolumeAttachmentParams(ctx, *volParams.VolumeAttachmentUUID)
			if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
				// No attachment params yet.
			} else if err != nil {
				results[i].Error = &params.Error{Message: err.Error()}
				continue
			} else {
				rval.Attachment = &params.VolumeAttachmentParams{
					VolumeTag:  tag.String(),
					InstanceId: vaParams.MachineInstanceID,
					Provider:   vaParams.Provider,
					ProviderId: vaParams.ProviderID,
					ReadOnly:   vaParams.ReadOnly,
				}
				if vaParams.Machine != nil {
					rval.Attachment.MachineTag = names.NewMachineTag(vaParams.Machine.String()).String()
				}
			}
		}
		results[i].Result = rval
	}
	return results, nil
}

func (a *modelStorageAdapter) RemoveVolumeParams(ctx context.Context, tags []names.VolumeTag) ([]params.RemoveVolumeParamsResult, error) {
	results := make([]params.RemoveVolumeParamsResult, len(tags))
	for i, tag := range tags {
		uuid, err := a.storageSvc.GetVolumeUUIDForID(ctx, tag.Id())
		if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
			results[i].Error = &params.Error{Message: errors.Errorf("volume %q not found", tag.Id()).Error(), Code: params.CodeNotFound}
			continue
		}
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		rp, err := a.storageSvc.GetVolumeRemovalParams(ctx, uuid)
		if errors.Is(err, storageprovisioningerrors.VolumeNotDead) {
			results[i].Error = &params.Error{Message: errors.Errorf("volume %q is not yet dead", tag.Id()).Error()}
			continue
		}
		if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
			results[i].Error = &params.Error{Message: errors.Errorf("volume %q not found", tag.Id()).Error(), Code: params.CodeNotFound}
			continue
		}
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		results[i].Result = params.RemoveVolumeParams{
			Provider:   rp.Provider,
			ProviderId: rp.ProviderID,
			Destroy:    rp.Obliterate,
		}
	}
	return results, nil
}

func (a *modelStorageAdapter) VolumeAttachmentParams(ctx context.Context, ids []params.MachineStorageId) ([]params.VolumeAttachmentParamsResult, error) {
	results := make([]params.VolumeAttachmentParamsResult, len(ids))
	for i, id := range ids {
		machineTag, err := names.ParseMachineTag(id.MachineTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		volumeTag, err := names.ParseVolumeTag(id.AttachmentTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machine.Name(machineTag.Id()))
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		attachmentUUID, err := a.storageSvc.GetVolumeAttachmentUUIDForVolumeIDMachine(ctx, volumeTag.Id(), machineUUID)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		vaParams, err := a.storageSvc.GetVolumeAttachmentParams(ctx, attachmentUUID)
		if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
			results[i].Error = &params.Error{Message: errors.Errorf("volume attachment %q on %q not found", volumeTag.Id(), machineTag.Id()).Error(), Code: params.CodeNotFound}
			continue
		}
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		results[i].Result = params.VolumeAttachmentParams{
			VolumeTag:  volumeTag.String(),
			MachineTag: machineTag.String(),
			InstanceId: vaParams.MachineInstanceID,
			Provider:   vaParams.Provider,
			ProviderId: vaParams.ProviderID,
			ReadOnly:   vaParams.ReadOnly,
		}
	}
	return results, nil
}

func (a *modelStorageAdapter) SetVolumeInfo(ctx context.Context, vols []params.Volume) ([]params.ErrorResult, error) {
	results := make([]params.ErrorResult, len(vols))
	for i, vol := range vols {
		volumeTag, err := names.ParseVolumeTag(vol.VolumeTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		if vol.Info.Pool != "" {
			results[i].Error = &params.Error{Message: "pool field must not be set"}
			continue
		}
		info := storageprovisioning.VolumeProvisionedInfo{
			ProviderID: vol.Info.ProviderId,
			SizeMiB:    vol.Info.SizeMiB,
			HardwareID: vol.Info.HardwareId,
			WWN:        vol.Info.WWN,
			Persistent: vol.Info.Persistent,
		}
		err = a.storageSvc.SetVolumeProvisionedInfo(ctx, volumeTag.Id(), info)
		if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
			results[i].Error = &params.Error{Message: errors.Errorf("volume %q not found", volumeTag.Id()).Error(), Code: params.CodeNotFound}
			continue
		}
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
		}
	}
	return results, nil
}

func (a *modelStorageAdapter) SetVolumeAttachmentInfo(ctx context.Context, vas []params.VolumeAttachment) ([]params.ErrorResult, error) {
	results := make([]params.ErrorResult, len(vas))
	for i, va := range vas {
		machineTag, err := names.ParseMachineTag(va.MachineTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		volumeTag, err := names.ParseVolumeTag(va.VolumeTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machine.Name(machineTag.Id()))
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		attachmentUUID, err := a.storageSvc.GetVolumeAttachmentUUIDForVolumeIDMachine(ctx, volumeTag.Id(), machineUUID)
		if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
			results[i].Error = &params.Error{Message: errors.Errorf("volume attachment %q on %q not found", volumeTag.Id(), machineUUID).Error(), Code: params.CodeNotFound}
			continue
		}
		if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
			results[i].Error = &params.Error{Message: errors.Errorf("volume %q not found for attachment on %q", volumeTag.Id(), machineUUID).Error(), Code: params.CodeNotFound}
			continue
		}
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		info := storageprovisioning.VolumeAttachmentProvisionedInfo{
			ReadOnly: va.Info.ReadOnly,
		}
		if va.Info.DeviceName != "" || va.Info.DeviceLink != "" || va.Info.BusAddress != "" {
			device := coreblockdevice.BlockDevice{
				DeviceName: va.Info.DeviceName,
				BusAddress: va.Info.BusAddress,
			}
			if va.Info.DeviceLink != "" {
				device.DeviceLinks = []string{va.Info.DeviceLink}
			}
			blockDevUUID, err := a.blockDeviceSvc.MatchOrCreateBlockDevice(ctx, machineUUID, device)
			if err != nil {
				results[i].Error = &params.Error{Message: err.Error()}
				continue
			}
			info.BlockDeviceUUID = &blockDevUUID
		}
		err = a.storageSvc.SetVolumeAttachmentProvisionedInfo(ctx, attachmentUUID, info)
		if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
			results[i].Error = &params.Error{Message: errors.Errorf(
				"volume attachment for machine %q and volume %q not found",
				machineTag.Id(), volumeTag.Id(),
			).Error(), Code: params.CodeNotFound}
			continue
		}
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		if va.Info.PlanInfo == nil {
			continue
		}
		planUUID, err := a.storageSvc.GetVolumeAttachmentPlanUUIDForVolumeIDMachine(ctx, volumeTag.Id(), machineUUID)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		planInfo := storageprovisioning.VolumeAttachmentPlanProvisionedInfo{
			DeviceAttributes: va.Info.PlanInfo.DeviceAttributes,
		}
		switch va.Info.PlanInfo.DeviceType {
		case "local":
			planInfo.DeviceType = domainstorage.VolumeDeviceTypeLocal
		case "iscsi":
			planInfo.DeviceType = domainstorage.VolumeDeviceTypeISCSI
		}
		err = a.storageSvc.SetVolumeAttachmentPlanProvisionedInfo(ctx, planUUID, planInfo)
		if errors.Is(err, storageprovisioningerrors.VolumeAttachmentPlanNotFound) {
			results[i].Error = &params.Error{Message: errors.Errorf(
				"volume attachment plan for machine %q and volume %q not found",
				machineTag.Id(), volumeTag.Id(),
			).Error(), Code: params.CodeNotFound}
		} else if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
		}
	}
	return results, nil
}

func (a *modelStorageAdapter) CreateVolumeAttachmentPlans(ctx context.Context, plans []params.VolumeAttachmentPlan) ([]params.ErrorResult, error) {
	results := make([]params.ErrorResult, len(plans))
	for i, plan := range plans {
		machineTag, err := names.ParseMachineTag(plan.MachineTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		volumeTag, err := names.ParseVolumeTag(plan.VolumeTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machine.Name(machineTag.Id()))
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		attachmentUUID, err := a.storageSvc.GetVolumeAttachmentUUIDForVolumeIDMachine(ctx, volumeTag.Id(), machineUUID)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		var deviceType domainstorage.VolumeDeviceType
		switch plan.PlanInfo.DeviceType {
		case "iscsi":
			deviceType = domainstorage.VolumeDeviceTypeISCSI
		case "local":
			deviceType = domainstorage.VolumeDeviceTypeLocal
		default:
			results[i].Error = &params.Error{
				Message: errors.Errorf(
					"plan device type %q not valid", plan.PlanInfo.DeviceType,
				).Error(),
				Code: params.CodeNotValid,
			}
			continue
		}
		_, err = a.storageSvc.CreateVolumeAttachmentPlan(ctx, attachmentUUID, deviceType, plan.PlanInfo.DeviceAttributes)
		if errors.Is(err, storageprovisioningerrors.VolumeAttachmentPlanAlreadyExists) {
			continue
		} else if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
		}
	}
	return results, nil
}

func (a *modelStorageAdapter) RemoveVolumeAttachmentPlan(ctx context.Context, ids []params.MachineStorageId) ([]params.ErrorResult, error) {
	results := make([]params.ErrorResult, len(ids))
	for i, id := range ids {
		volumeTag, err := names.ParseVolumeTag(id.AttachmentTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		machineTag, err := names.ParseMachineTag(id.MachineTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machine.Name(machineTag.Id()))
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		planUUID, err := a.storageSvc.GetVolumeAttachmentPlanUUIDForVolumeIDMachine(ctx, volumeTag.Id(), machineUUID)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		err = a.removalSvc.MarkVolumeAttachmentPlanAsDead(ctx, planUUID)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
		}
	}
	return results, nil
}

func (a *modelStorageAdapter) SetVolumeAttachmentPlanBlockInfo(ctx context.Context, plans []params.VolumeAttachmentPlan) ([]params.ErrorResult, error) {
	results := make([]params.ErrorResult, len(plans))
	for i, plan := range plans {
		machineTag, err := names.ParseMachineTag(plan.MachineTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		volumeTag, err := names.ParseVolumeTag(plan.VolumeTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machine.Name(machineTag.Id()))
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		planUUID, err := a.storageSvc.GetVolumeAttachmentPlanUUIDForVolumeIDMachine(ctx, volumeTag.Id(), machineUUID)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		if plan.BlockDevice == nil {
			continue
		}
		device := coreblockdevice.BlockDevice{
			DeviceName:      plan.BlockDevice.DeviceName,
			DeviceLinks:     plan.BlockDevice.DeviceLinks,
			FilesystemLabel: plan.BlockDevice.Label,
			FilesystemUUID:  plan.BlockDevice.UUID,
			HardwareId:      plan.BlockDevice.HardwareId,
			WWN:             plan.BlockDevice.WWN,
			BusAddress:      plan.BlockDevice.BusAddress,
			SizeMiB:         plan.BlockDevice.SizeMiB,
			FilesystemType:  plan.BlockDevice.FilesystemType,
			InUse:           plan.BlockDevice.InUse,
			MountPoint:      plan.BlockDevice.MountPoint,
			SerialId:        plan.BlockDevice.SerialId,
		}
		if domainblockdevice.IsEmpty(device) {
			continue
		}
		blockDevUUID, err := a.blockDeviceSvc.MatchOrCreateBlockDevice(ctx, machineUUID, device)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		err = a.storageSvc.SetVolumeAttachmentPlanProvisionedBlockDevice(ctx, planUUID, blockDevUUID)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
		}
	}
	return results, nil
}

// FilesystemAccessor implementation

func (a *modelStorageAdapter) WatchFilesystems(ctx context.Context, scope names.Tag) (watcher.StringsWatcher, error) {
	switch scope := scope.(type) {
	case names.ModelTag:
		return a.storageSvc.WatchModelProvisionedFilesystems(ctx)
	case names.MachineTag:
		machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machine.Name(scope.Id()))
		if err != nil {
			return nil, errors.Trace(err)
		}
		return a.storageSvc.WatchMachineProvisionedFilesystems(ctx, machineUUID)
	default:
		return nil, internalerrors.Errorf("unsupported scope %T", scope).Add(coreerrors.NotSupported)
	}
}

func (a *modelStorageAdapter) WatchFilesystemAttachments(ctx context.Context, scope names.Tag) (watcher.MachineStorageIDsWatcher, error) {
	var sourceWatcher watcher.StringsWatcher
	switch scope := scope.(type) {
	case names.ModelTag:
		w, err := a.storageSvc.WatchModelProvisionedFilesystemAttachments(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		sourceWatcher = w
	case names.MachineTag:
		machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machine.Name(scope.Id()))
		if err != nil {
			return nil, errors.Trace(err)
		}
		w, err := a.storageSvc.WatchMachineProvisionedFilesystemAttachments(ctx, machineUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		sourceWatcher = w
	default:
		return nil, internalerrors.Errorf("unsupported scope %T", scope).Add(coreerrors.NotSupported)
	}
	return newAttachmentIDWatcher(sourceWatcher, func(ctx context.Context, ids ...string) ([]watcher.MachineStorageID, error) {
		attachmentIDs, err := a.storageSvc.GetFilesystemAttachmentIDs(ctx, ids)
		if err != nil {
			return nil, errors.Trace(err)
		}
		var out []watcher.MachineStorageID
		for _, id := range attachmentIDs {
			if id.MachineName == nil && id.UnitName == nil {
				continue
			}
			msid := watcher.MachineStorageID{
				AttachmentTag: names.NewFilesystemTag(id.FilesystemID).String(),
			}
			if id.MachineName != nil {
				msid.MachineTag = names.NewMachineTag(id.MachineName.String()).String()
			} else if id.UnitName != nil {
				msid.MachineTag = names.NewUnitTag(id.UnitName.String()).String()
			}
			out = append(out, msid)
		}
		return out, nil
	})
}

func (a *modelStorageAdapter) Filesystems(ctx context.Context, tags []names.FilesystemTag) ([]params.FilesystemResult, error) {
	results := make([]params.FilesystemResult, len(tags))
	for i, tag := range tags {
		fs, err := a.storageSvc.GetFilesystemForID(ctx, tag.Id())
		if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
			results[i].Error = &params.Error{Message: errors.Errorf("filesystem %q not found", tag.Id()).Error(), Code: params.CodeNotFound}
			continue
		}
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		if fs.SizeMiB == 0 {
			results[i].Error = &params.Error{Message: errors.Errorf("filesystem %q is not provisioned", tag.Id()).Error(), Code: params.CodeNotProvisioned}
			continue
		}
		result := params.Filesystem{
			FilesystemTag: tag.String(),
			Info: params.FilesystemInfo{
				ProviderId: fs.ProviderID,
				SizeMiB:    fs.SizeMiB,
			},
		}
		if fs.BackingVolume != nil {
			result.VolumeTag = names.NewVolumeTag(fs.BackingVolume.VolumeID).String()
		}
		results[i].Result = result
	}
	return results, nil
}

func (a *modelStorageAdapter) FilesystemAttachments(ctx context.Context, ids []params.MachineStorageId) ([]params.FilesystemAttachmentResult, error) {
	results := make([]params.FilesystemAttachmentResult, len(ids))
	for i, id := range ids {
		filesystemTag, err := names.ParseFilesystemTag(id.AttachmentTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		hostTag, err := names.ParseTag(id.MachineTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		var fsAttachment storageprovisioning.FilesystemAttachment
		switch tag := hostTag.(type) {
		case names.MachineTag:
			machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machine.Name(tag.Id()))
			if err != nil {
				results[i].Error = &params.Error{Message: err.Error()}
				continue
			}
			fsAttachment, err = a.storageSvc.GetFilesystemAttachmentForMachine(ctx, filesystemTag.Id(), machineUUID)
			if err != nil {
				results[i].Error = &params.Error{Message: err.Error()}
				continue
			}
		case names.UnitTag:
			unitUUID, err := a.getUnitUUID(ctx, tag)
			if err != nil {
				results[i].Error = &params.Error{Message: err.Error()}
				continue
			}
			fsAttachment, err = a.storageSvc.GetFilesystemAttachmentForUnit(ctx, filesystemTag.Id(), unitUUID)
			if err != nil {
				results[i].Error = &params.Error{Message: err.Error()}
				continue
			}
		default:
			results[i].Error = &params.Error{Message: errors.Errorf("filesystem attachment host tag %q not valid", hostTag.String()).Error(), Code: params.CodeNotValid}
			continue
		}
		if fsAttachment.MountPoint == "" {
			results[i].Error = &params.Error{Message: errors.Errorf("filesystem attachment %q on %q is not provisioned", filesystemTag.Id(), hostTag.String()).Error(), Code: params.CodeNotProvisioned}
			continue
		}
		results[i].Result = params.FilesystemAttachment{
			FilesystemTag: filesystemTag.String(),
			MachineTag:    hostTag.String(),
			Info: params.FilesystemAttachmentInfo{
				MountPoint: fsAttachment.MountPoint,
				ReadOnly:   fsAttachment.ReadOnly,
			},
		}
	}
	return results, nil
}

func (a *modelStorageAdapter) FilesystemParams(ctx context.Context, tags []names.FilesystemTag) ([]params.FilesystemParamsResultV5, error) {
	results := make([]params.FilesystemParamsResultV5, len(tags))
	fsModelTags, err := a.storageSvc.GetStorageResourceTagsForModel(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for i, tag := range tags {
		uuid, err := a.storageSvc.GetFilesystemUUIDForID(ctx, tag.Id())
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		fsParams, err := a.storageSvc.GetFilesystemParams(ctx, uuid)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		rval := params.FilesystemParamsV5{
			Attributes:    make(map[string]any, len(fsParams.Attributes)),
			FilesystemTag: tag.String(),
			Provider:      fsParams.Provider,
			ProviderId:    fsParams.ProviderID,
			SizeMiB:       fsParams.SizeMiB,
			Tags:          fsModelTags,
		}
		for k, v := range fsParams.Attributes {
			rval.Attributes[k] = v
		}
		if fsParams.BackingVolume != nil {
			rval.VolumeTag = names.NewVolumeTag(fsParams.BackingVolume.VolumeID).String()
		}
		results[i].Result = rval
	}
	return results, nil
}

func (a *modelStorageAdapter) RemoveFilesystemParams(ctx context.Context, tags []names.FilesystemTag) ([]params.RemoveFilesystemParamsResult, error) {
	results := make([]params.RemoveFilesystemParamsResult, len(tags))
	for i, tag := range tags {
		uuid, err := a.storageSvc.GetFilesystemUUIDForID(ctx, tag.Id())
		if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
			results[i].Error = &params.Error{Message: errors.Errorf("filesystem %q not found", tag.Id()).Error(), Code: params.CodeNotFound}
			continue
		}
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		rp, err := a.storageSvc.GetFilesystemRemovalParams(ctx, uuid)
		if errors.Is(err, storageprovisioningerrors.FilesystemNotDead) {
			results[i].Error = &params.Error{Message: errors.Errorf("filesystem %q is not yet dead", tag.Id()).Error()}
			continue
		}
		if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
			results[i].Error = &params.Error{Message: errors.Errorf("filesystem %q not found", tag.Id()).Error(), Code: params.CodeNotFound}
			continue
		}
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		results[i].Result = params.RemoveFilesystemParams{
			Provider:   rp.Provider,
			ProviderId: rp.ProviderID,
			Destroy:    rp.Obliterate,
		}
	}
	return results, nil
}

func (a *modelStorageAdapter) FilesystemAttachmentParams(ctx context.Context, ids []params.MachineStorageId) ([]params.FilesystemAttachmentParamsResultV5, error) {
	results := make([]params.FilesystemAttachmentParamsResultV5, len(ids))
	for i, id := range ids {
		hostTag, err := names.ParseTag(id.MachineTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		filesystemTag, err := names.ParseFilesystemTag(id.AttachmentTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		var attachmentUUID domainstorage.FilesystemAttachmentUUID
		switch tag := hostTag.(type) {
		case names.MachineTag:
			machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machine.Name(tag.Id()))
			if err != nil {
				results[i].Error = &params.Error{Message: err.Error()}
				continue
			}
			attachmentUUID, err = a.storageSvc.GetFilesystemAttachmentUUIDForFilesystemIDMachine(ctx, filesystemTag.Id(), machineUUID)
			if err != nil {
				results[i].Error = &params.Error{Message: err.Error()}
				continue
			}
		default:
			results[i].Error = &params.Error{Message: errors.Errorf("unsupported host tag %T", hostTag).Error()}
			continue
		}
		fsParams, err := a.storageSvc.GetFilesystemAttachmentParams(ctx, attachmentUUID)
		if errors.Is(err, storageprovisioningerrors.FilesystemAttachmentNotFound) {
			results[i].Error = &params.Error{Message: errors.Errorf("filesystem attachment for %q on %q not found", filesystemTag, hostTag).Error(), Code: params.CodeNotFound}
			continue
		}
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		instanceID := fsParams.MachineInstanceID
		if instanceID == "" {
			instanceID = fsParams.CAASInstanceID
		}
		results[i].Result = params.FilesystemAttachmentParamsV5{
			FilesystemTag: filesystemTag.String(),
			MachineTag:    hostTag.String(),
			InstanceId:    instanceID,
			Provider:      fsParams.Provider,
			ReadOnly:      fsParams.CharmStorageReadOnly,
		}
	}
	return results, nil
}

func (a *modelStorageAdapter) SetFilesystemInfo(ctx context.Context, fss []params.Filesystem) ([]params.ErrorResult, error) {
	results := make([]params.ErrorResult, len(fss))
	for i, fs := range fss {
		filesystemTag, err := names.ParseFilesystemTag(fs.FilesystemTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		if fs.Info.Pool != "" {
			results[i].Error = &params.Error{Message: "pool field must not be set"}
			continue
		}
		info := storageprovisioning.FilesystemProvisionedInfo{
			ProviderID: fs.Info.ProviderId,
			SizeMiB:    fs.Info.SizeMiB,
		}
		err = a.storageSvc.SetFilesystemProvisionedInfo(ctx, filesystemTag.Id(), info)
		if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
			results[i].Error = &params.Error{Message: errors.Errorf("filesystem %q not found", filesystemTag.Id()).Error(), Code: params.CodeNotFound}
			continue
		}
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
		}
	}
	return results, nil
}

func (a *modelStorageAdapter) SetFilesystemAttachmentInfo(ctx context.Context, fas []params.FilesystemAttachment) ([]params.ErrorResult, error) {
	results := make([]params.ErrorResult, len(fas))
	for i, fa := range fas {
		hostTag, err := names.ParseTag(fa.MachineTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		filesystemTag, err := names.ParseFilesystemTag(fa.FilesystemTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		info := storageprovisioning.FilesystemAttachmentProvisionedInfo{
			MountPoint: fa.Info.MountPoint,
			ReadOnly:   fa.Info.ReadOnly,
		}
		switch tag := hostTag.(type) {
		case names.MachineTag:
			machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machine.Name(tag.Id()))
			if err != nil {
				results[i].Error = &params.Error{Message: err.Error()}
				continue
			}
			err = a.storageSvc.SetFilesystemAttachmentProvisionedInfoForMachine(ctx, filesystemTag.Id(), machineUUID, info)
			if err != nil {
				results[i].Error = &params.Error{Message: err.Error()}
			}
		case names.UnitTag:
			unitUUID, err := a.getUnitUUID(ctx, tag)
			if err != nil {
				results[i].Error = &params.Error{Message: err.Error()}
				continue
			}
			err = a.storageSvc.SetFilesystemAttachmentProvisionedInfoForUnit(ctx, filesystemTag.Id(), unitUUID, info)
			if err != nil {
				results[i].Error = &params.Error{Message: err.Error()}
			}
		default:
			results[i].Error = &params.Error{Message: errors.Errorf("filesystem attachment host tag %q not valid", hostTag.String()).Error(), Code: params.CodeNotValid}
		}
	}
	return results, nil
}

func (a *modelStorageAdapter) getUnitUUID(ctx context.Context, tag names.UnitTag) (coreunit.UUID, error) {
	unitName, err := coreunit.NewName(tag.Id())
	if errors.Is(err, coreunit.InvalidUnitName) {
		return "", errors.Errorf("invalid unit name %q", tag.Id())
	} else if err != nil {
		return "", errors.Trace(err)
	}
	unitUUID, err := a.appSvc.GetUnitUUID(ctx, unitName)
	if errors.Is(err, coreunit.InvalidUnitName) {
		return "", errors.Errorf("invalid unit name %q", unitName)
	} else if errors.Is(err, applicationerrors.UnitNotFound) {
		return "", errors.Errorf("unit %q not found", unitName)
	} else if err != nil {
		return "", errors.Errorf("getting unit %q UUID: %v", unitName, err)
	}
	return unitUUID, nil
}

// LifecycleManager implementation

func (a *modelStorageAdapter) Life(ctx context.Context, tags []names.Tag) ([]params.LifeResult, error) {
	results := make([]params.LifeResult, len(tags))
	for i, tag := range tags {
		var l domainlife.Life
		var err error
		switch tag := tag.(type) {
		case names.VolumeTag:
			uuid, e := a.storageSvc.GetVolumeUUIDForID(ctx, tag.Id())
			if errors.Is(e, storageprovisioningerrors.VolumeNotFound) {
				results[i].Error = &params.Error{Message: errors.Errorf("volume not found for id %q", tag.Id()).Error(), Code: params.CodeNotFound}
				continue
			} else if e != nil {
				results[i].Error = &params.Error{Message: e.Error()}
				continue
			}
			l, err = a.storageSvc.GetVolumeLife(ctx, uuid)
			if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
				results[i].Error = &params.Error{Message: errors.Errorf("volume not found for id %q", tag.Id()).Error(), Code: params.CodeNotFound}
				continue
			}
		case names.FilesystemTag:
			uuid, e := a.storageSvc.GetFilesystemUUIDForID(ctx, tag.Id())
			if errors.Is(e, storageprovisioningerrors.FilesystemNotFound) {
				results[i].Error = &params.Error{Message: errors.Errorf("filesystem not found for id %q", tag.Id()).Error(), Code: params.CodeNotFound}
				continue
			} else if e != nil {
				results[i].Error = &params.Error{Message: e.Error()}
				continue
			}
			l, err = a.storageSvc.GetFilesystemLife(ctx, uuid)
			if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
				results[i].Error = &params.Error{Message: errors.Errorf("filesystem not found for id %q", tag.Id()).Error(), Code: params.CodeNotFound}
				continue
			}
		default:
			results[i].Error = &params.Error{Message: errors.Errorf("invalid tag %q, expected volume or filesystem", tag).Error(), Code: params.CodeNotValid}
			continue
		}
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		val, err := l.Value()
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		results[i].Life = val
	}
	return results, nil
}

func (a *modelStorageAdapter) Remove(ctx context.Context, tags []names.Tag) ([]params.ErrorResult, error) {
	results := make([]params.ErrorResult, len(tags))
	for i, tag := range tags {
		var err error
		switch tag := tag.(type) {
		case names.VolumeTag:
			uuid, e := a.storageSvc.GetVolumeUUIDForID(ctx, tag.Id())
			if errors.Is(e, storageprovisioningerrors.VolumeNotFound) {
				continue
			}
			if e != nil {
				err = e
				break
			}
			err = a.removalSvc.RemoveDeadVolume(ctx, uuid)
			if errors.Is(err, storageprovisioningerrors.VolumeNotDead) {
				err = errors.Errorf("volume %q is not yet dead", tag.Id())
			} else if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
				err = nil
			}
		case names.FilesystemTag:
			uuid, e := a.storageSvc.GetFilesystemUUIDForID(ctx, tag.Id())
			if errors.Is(e, storageprovisioningerrors.FilesystemNotFound) {
				continue
			}
			if e != nil {
				err = e
				break
			}
			err = a.removalSvc.RemoveDeadFilesystem(ctx, uuid)
			if errors.Is(err, storageprovisioningerrors.FilesystemNotDead) {
				err = errors.Errorf("filesystem %q is not yet dead", tag.Id())
			} else if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
				err = nil
			}
		default:
			results[i].Error = &params.Error{Message: errors.Errorf("tag %q invalid", tag).Error()}
			continue
		}
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
		}
	}
	return results, nil
}

func (a *modelStorageAdapter) AttachmentLife(ctx context.Context, ids []params.MachineStorageId) ([]params.LifeResult, error) {
	results := make([]params.LifeResult, len(ids))
	for i, id := range ids {
		hostTag, err := names.ParseTag(id.MachineTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		attachmentTag, err := names.ParseTag(id.AttachmentTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		var l domainlife.Life
		switch tag := attachmentTag.(type) {
		case names.VolumeTag:
			machineTag, ok := hostTag.(names.MachineTag)
			if !ok {
				results[i].Error = &params.Error{Message: "volume attachments only supported on machines"}
				continue
			}
			machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machine.Name(machineTag.Id()))
			if err != nil {
				results[i].Error = &params.Error{Message: err.Error()}
				continue
			}
			uuid, err := a.storageSvc.GetVolumeAttachmentUUIDForVolumeIDMachine(ctx, tag.Id(), machineUUID)
			if err != nil {
				results[i].Error = &params.Error{Message: err.Error()}
				continue
			}
			l, err = a.storageSvc.GetVolumeAttachmentLife(ctx, uuid)
			if err != nil {
				results[i].Error = &params.Error{Message: err.Error()}
				continue
			}
		case names.FilesystemTag:
			var fsUUID domainstorage.FilesystemAttachmentUUID
			switch host := hostTag.(type) {
			case names.MachineTag:
				machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machine.Name(host.Id()))
				if err != nil {
					results[i].Error = &params.Error{Message: err.Error()}
					continue
				}
				fsUUID, err = a.storageSvc.GetFilesystemAttachmentUUIDForFilesystemIDMachine(ctx, tag.Id(), machineUUID)
				if err != nil {
					results[i].Error = &params.Error{Message: err.Error()}
					continue
				}
			case names.UnitTag:
				unitUUID, err := a.getUnitUUID(ctx, host)
				if err != nil {
					results[i].Error = &params.Error{Message: err.Error()}
					continue
				}
				fsUUID, err = a.storageSvc.GetFilesystemAttachmentUUIDForFilesystemIDUnit(ctx, tag.Id(), unitUUID)
				if err != nil {
					results[i].Error = &params.Error{Message: err.Error()}
					continue
				}
			default:
				results[i].Error = &params.Error{Message: errors.Errorf("attachment tag %q is not valid", attachmentTag).Error(), Code: params.CodeNotValid}
				continue
			}
			l, err = a.storageSvc.GetFilesystemAttachmentLife(ctx, fsUUID)
			if err != nil {
				results[i].Error = &params.Error{Message: err.Error()}
				continue
			}
		default:
			results[i].Error = &params.Error{Message: errors.Errorf("attachment tag %q is not valid", attachmentTag).Error()}
			continue
		}
		val, err := l.Value()
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		results[i].Life = val
	}
	return results, nil
}

func (a *modelStorageAdapter) RemoveAttachments(ctx context.Context, ids []params.MachineStorageId) ([]params.ErrorResult, error) {
	results := make([]params.ErrorResult, len(ids))
	for i, id := range ids {
		hostTag, err := names.ParseTag(id.MachineTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		attachmentTag, err := names.ParseTag(id.AttachmentTag)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		switch tag := attachmentTag.(type) {
		case names.VolumeTag:
			machineTag, ok := hostTag.(names.MachineTag)
			if !ok {
				results[i].Error = &params.Error{Message: errors.Errorf("tag %q on host %q invalid", id.AttachmentTag, id.MachineTag).Error(), Code: params.CodeNotValid}
				continue
			}
			machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machine.Name(machineTag.Id()))
			if err != nil {
				results[i].Error = &params.Error{Message: err.Error()}
				continue
			}
			uuid, err := a.storageSvc.GetVolumeAttachmentUUIDForVolumeIDMachine(ctx, tag.Id(), machineUUID)
			if errors.Is(err, coreerrors.NotFound) {
				continue
			}
			if err != nil {
				results[i].Error = &params.Error{Message: err.Error()}
				continue
			}
			err = a.removalSvc.MarkVolumeAttachmentAsDead(ctx, uuid)
			if errors.Is(err, removalerrors.EntityStillAlive) {
				results[i].Error = &params.Error{Message: errors.Errorf("volume %q attachment to machine %q is still alive", tag.Id(), machineTag.Id()).Error()}
			} else if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
				continue
			} else if err != nil {
				results[i].Error = &params.Error{Message: err.Error()}
			}
		case names.FilesystemTag:
			var fsUUID domainstorage.FilesystemAttachmentUUID
			switch host := hostTag.(type) {
			case names.MachineTag:
				machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machine.Name(host.Id()))
				if err != nil {
					results[i].Error = &params.Error{Message: err.Error()}
					continue
				}
				fsUUID, err = a.storageSvc.GetFilesystemAttachmentUUIDForFilesystemIDMachine(ctx, tag.Id(), machineUUID)
				if errors.Is(err, coreerrors.NotFound) {
					continue
				}
				if err != nil {
					results[i].Error = &params.Error{Message: err.Error()}
					continue
				}
			case names.UnitTag:
				unitUUID, err := a.getUnitUUID(ctx, host)
				if err != nil {
					results[i].Error = &params.Error{Message: err.Error()}
					continue
				}
				fsUUID, err = a.storageSvc.GetFilesystemAttachmentUUIDForFilesystemIDUnit(ctx, tag.Id(), unitUUID)
				if errors.Is(err, coreerrors.NotFound) {
					continue
				}
				if err != nil {
					results[i].Error = &params.Error{Message: err.Error()}
					continue
				}
			default:
				results[i].Error = &params.Error{Message: errors.Errorf("tag %q on host %q invalid", id.AttachmentTag, id.MachineTag).Error(), Code: params.CodeNotValid}
				continue
			}
			err = a.removalSvc.MarkFilesystemAttachmentAsDead(ctx, fsUUID)
			if errors.Is(err, removalerrors.EntityStillAlive) {
				results[i].Error = &params.Error{Message: errors.Errorf("filesystem %q attachment to %s %q is still alive", tag.Id(), hostTag.Kind(), hostTag.Id()).Error()}
			} else if errors.Is(err, storageprovisioningerrors.FilesystemAttachmentNotFound) {
				continue
			} else if err != nil {
				results[i].Error = &params.Error{Message: err.Error()}
			}
		default:
			results[i].Error = &params.Error{Message: errors.Errorf("tag %q on host %q invalid", id.AttachmentTag, id.MachineTag).Error(), Code: params.CodeNotValid}
		}
	}
	return results, nil
}

// MachineAccessor implementation

func (a *modelStorageAdapter) WatchMachine(ctx context.Context, m names.MachineTag) (watcher.NotifyWatcher, error) {
	machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machine.Name(m.Id()))
	if err != nil {
		return nil, errors.Trace(err)
	}
	return a.machineSvc.WatchMachineCloudInstances(ctx, machineUUID)
}

func (a *modelStorageAdapter) InstanceIds(ctx context.Context, tags []names.MachineTag) ([]params.StringResult, error) {
	results := make([]params.StringResult, len(tags))
	for i, tag := range tags {
		machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machine.Name(tag.Id()))
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		instanceId, err := a.machineSvc.GetInstanceID(ctx, machineUUID)
		if err != nil {
			results[i].Error = &params.Error{Message: err.Error()}
			continue
		}
		results[i].Result = string(instanceId)
	}
	return results, nil
}

// StatusSetter implementation

func (a *modelStorageAdapter) SetStatus(ctx context.Context, args []params.EntityStatusArgs) error {
	for _, arg := range args {
		tag, err := names.ParseTag(arg.Tag)
		if err != nil {
			return errors.Trace(err)
		}
		sInfo := status.StatusInfo{
			Status:  status.Status(arg.Status),
			Message: arg.Info,
			Data:    arg.Data,
		}
		switch tag := tag.(type) {
		case names.VolumeTag:
			if err := a.statusSvc.SetVolumeStatus(ctx, tag.Id(), sInfo); err != nil {
				return errors.Trace(err)
			}
		case names.FilesystemTag:
			if err := a.statusSvc.SetFilesystemStatus(ctx, tag.Id(), sInfo); err != nil {
				return errors.Trace(err)
			}
		default:
			return errors.Errorf("invalid tag %q, expected volume or filesystem", tag)
		}
	}
	return nil
}

// newAttachmentIDWatcher creates a MachineStorageIDsWatcher that wraps a
// StringsWatcher and maps domain UUID strings to MachineStorageID values.
func newAttachmentIDWatcher(
	source watcher.StringsWatcher,
	mapper func(context.Context, ...string) ([]watcher.MachineStorageID, error),
) (watcher.MachineStorageIDsWatcher, error) {
	w := &attachmentIDWatcher{
		source: source,
		mapper: mapper,
		out:    make(chan []watcher.MachineStorageID, 1),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "attachment-watcher",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{source},
	})
	return w, internalerrors.Capture(err)
}

type attachmentIDWatcher struct {
	catacomb catacomb.Catacomb
	source   watcher.StringsWatcher
	mapper   func(context.Context, ...string) ([]watcher.MachineStorageID, error)
	out      chan []watcher.MachineStorageID
}

func (w *attachmentIDWatcher) loop() error {
	defer close(w.out)
	var (
		changes []watcher.MachineStorageID
		out     chan []watcher.MachineStorageID
		initial = true
	)
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case events, ok := <-w.source.Changes():
			if !ok {
				return errors.Errorf("source watcher closed")
			}
			if !initial && len(events) == 0 {
				continue
			}
			results, err := w.mapper(w.catacomb.Context(context.Background()), events...)
			if err != nil {
				return errors.Errorf("processing changes: %v", err)
			}
			if !initial && len(results) == 0 {
				continue
			}
			if changes == nil {
				changes = results
			} else {
				changes = append(changes, results...)
			}
			out = w.out
		case out <- changes:
			changes = nil
			out = nil
			initial = false
		}
	}
}

func (w *attachmentIDWatcher) Changes() <-chan []watcher.MachineStorageID {
	return w.out
}

func (w *attachmentIDWatcher) Kill() {
	w.catacomb.Kill(nil)
}

func (w *attachmentIDWatcher) Wait() error {
	return w.catacomb.Wait()
}

func (w *attachmentIDWatcher) Err() error {
	return w.catacomb.Err()
}
