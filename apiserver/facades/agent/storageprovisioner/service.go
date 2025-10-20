// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"context"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	domainblockdevice "github.com/juju/juju/domain/blockdevice"
	domainlife "github.com/juju/juju/domain/life"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/environs/config"
)

// ControllerConfigService provides access to the controller configuration.
type ControllerConfigService interface {
	// ControllerConfig returns the config values for the controller.
	ControllerConfig(context.Context) (controller.Config, error)
}

// ModelConfigService is the interface that the provisioner facade uses to get
// the model config.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
}

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(ctx context.Context, machineName machine.Name) (machine.UUID, error)
	// GetInstanceID returns the cloud specific instance id for this machine.
	GetInstanceID(ctx context.Context, machineUUID machine.UUID) (instance.Id, error)
	// GetInstanceIDAndName returns the cloud specific instance ID and display
	// name for this machine.
	GetInstanceIDAndName(ctx context.Context, machineUUID machine.UUID) (instance.Id, string, error)
	// GetHardwareCharacteristics returns the hardware characteristics of the
	// specified machine.
	GetHardwareCharacteristics(ctx context.Context, machineUUID machine.UUID) (*instance.HardwareCharacteristics, error)
	// GetMachineLife returns the lifecycle state of the machine with the
	// specified UUID.
	GetMachineLife(ctx context.Context, machineName machine.Name) (life.Value, error)
	// WatchMachineCloudInstances returns a NotifyWatcher that is subscribed to
	// the changes in the machine_cloud_instance table in the model, for the given
	// machine UUID.
	WatchMachineCloudInstances(ctx context.Context, machineUUID machine.UUID) (watcher.NotifyWatcher, error)
}

// BlockDeviceService instances can fetch and watch block devices on a machine.
type BlockDeviceService interface {
	// GetBlockDevice retrieves a block device by uuid.
	GetBlockDevice(
		ctx context.Context, uuid domainblockdevice.BlockDeviceUUID,
	) (blockdevice.BlockDevice, error)

	// GetBlockDeviceForMachine returns the BlockDevices for the specified
	// machine.
	GetBlockDevicesForMachine(
		ctx context.Context, machineUUID machine.UUID,
	) ([]blockdevice.BlockDevice, error)

	// MatchOrCreateBlockDevice matches an existing block device to the provided
	// block device, otherwise it creates one that matches the existing device.
	// It returns the UUID of the block device.
	MatchOrCreateBlockDevice(
		ctx context.Context, machineUUID machine.UUID,
		device blockdevice.BlockDevice,
	) (domainblockdevice.BlockDeviceUUID, error)

	// WatchBlockDevicesForMachine returns a new NotifyWatcher watching for
	// changes to block devices associated with the specified machine.
	WatchBlockDevicesForMachine(
		ctx context.Context, machineUUID machine.UUID,
	) (watcher.NotifyWatcher, error)
}

// StoragePoolGetter instances get a storage pool by name.
type StoragePoolGetter interface {
	// GetStoragePoolByName returns the storage pool with the specified name.
	GetStoragePoolByName(ctx context.Context, name string) (domainstorage.StoragePool, error)
}

// ApplicationService is an interface for the application domain service.
type ApplicationService interface {
	// GetUnitLife returns the life status of a unit identified by its name.
	GetUnitLife(ctx context.Context, unitName coreunit.Name) (life.Value, error)
	// GetApplicationLifeByName looks up the life of the specified application.
	GetApplicationLifeByName(ctx context.Context, appName string) (life.Value, error)
	// GetUnitUUID returns the UUID for the named unit.
	GetUnitUUID(ctx context.Context, unitName coreunit.Name) (coreunit.UUID, error)
}

// RemovalService provides removal operations to progress the removal of
// volume attachments, filesystem attachment and volume attachment plans.
type RemovalService interface {
	// MarkFilesystemAttachmentAsDead marks the filesystem attachment as dead.
	MarkFilesystemAttachmentAsDead(
		ctx context.Context, uuid storageprovisioning.FilesystemAttachmentUUID,
	) error

	// MarkVolumeAttachmentAsDead marks the volume attachment as dead.
	MarkVolumeAttachmentAsDead(
		ctx context.Context, uuid storageprovisioning.VolumeAttachmentUUID,
	) error

	// MarkVolumeAttachmentPlanAsDead marks the volume attachment plan as dead.
	MarkVolumeAttachmentPlanAsDead(
		ctx context.Context, uuid storageprovisioning.VolumeAttachmentPlanUUID,
	) error

	// RemoveDeadFilesystem is to be called from the storage provisoner to
	// finally remove a dead filesystem that it has been gracefully cleaned up.
	RemoveDeadFilesystem(
		ctx context.Context, uuid storageprovisioning.FilesystemUUID,
	) error

	// RemoveDeadVolume is to be called from the storage provisoner to finally
	// remove a dead volume that it has been gracefully cleaned up.
	RemoveDeadVolume(
		ctx context.Context, uuid storageprovisioning.VolumeUUID,
	) error
}

// StorageStatusService provides methods to set filesystem and volume status.
type StorageStatusService interface {
	// SetFilesystemStatus saves the given filesystem status, overwriting any
	// current status data. If returns an error satisfying
	// [storageerrors.FilesystemNotFound] if the filesystem doesn't exist.
	SetFilesystemStatus(ctx context.Context, filesystemID string, statusInfo corestatus.StatusInfo) error

	// SetVolumeStatus saves the given volume status, overwriting any
	// current status data. If returns an error satisfying
	// [storageerrors.VolumeNotFound] if the volume doesn't exist.
	SetVolumeStatus(ctx context.Context, volumeID string, statusInfo corestatus.StatusInfo) error
}

// StorageProvisioningService provides methods to watch and manage storage
// provisioning related resources.
type StorageProvisioningService interface {
	// GetFilesystemUUIDForID returns the UUID for a filesystem with the
	// supplied id.
	GetFilesystemUUIDForID(
		ctx context.Context, filesystemID string,
	) (storageprovisioning.FilesystemUUID, error)

	// GetFilesystemAttachmentParams retrieves the attachment parameters for a
	// given filesystem attachment.
	GetFilesystemAttachmentParams(
		ctx context.Context,
		filesystemUUID storageprovisioning.FilesystemAttachmentUUID,
	) (storageprovisioning.FilesystemAttachmentParams, error)

	// GetFilesystemLife returns the current life value for a filesystem UUID.
	GetFilesystemLife(
		ctx context.Context, uuid storageprovisioning.FilesystemUUID,
	) (domainlife.Life, error)

	// GetFilesystemParams returns the filesystem params for the supplied uuid.
	GetFilesystemParams(
		ctx context.Context, uuid storageprovisioning.FilesystemUUID,
	) (storageprovisioning.FilesystemParams, error)

	// GetFilesystemRemovalParams returns the filesystem removal params for the
	// supplied uuid.
	GetFilesystemRemovalParams(
		ctx context.Context, uuid storageprovisioning.FilesystemUUID,
	) (storageprovisioning.FilesystemRemovalParams, error)

	// CheckFilesystemForIDExists checks if a filesystem exists for the supplied
	// filesystem ID. True is returned when a filesystem exists.
	CheckFilesystemForIDExists(context.Context, string) (bool, error)

	// GetFilesystemAttachmentUUIDForFilesystemIDUnit returns the filesystem
	// attachment UUID for the supplied filesystem ID which is attached to the
	// unit.
	GetFilesystemAttachmentUUIDForFilesystemIDUnit(
		ctx context.Context, filesystemID string, unitUUID coreunit.UUID,
	) (storageprovisioning.FilesystemAttachmentUUID, error)

	// GetFilesystemAttachmentUUIDForFilesystemIDMachine returns the filesystem
	// attachment UUID for the supplied filesystem id which is attached to the
	// machine.
	GetFilesystemAttachmentUUIDForFilesystemIDMachine(
		ctx context.Context,
		filesystemID string, machineUUID machine.UUID,
	) (storageprovisioning.FilesystemAttachmentUUID, error)

	// GetFilesystemAttachmentForMachine retrieves the FilesystemAttachment
	// for the supplied machine UUID and filesystem ID.
	GetFilesystemAttachmentForMachine(
		ctx context.Context, filesystemID string, machineUUID machine.UUID,
	) (storageprovisioning.FilesystemAttachment, error)

	// GetFilesystemAttachmentForUnit retrieves the FilesystemAttachment
	// for the supplied unit UUID and filesystem ID.
	GetFilesystemAttachmentForUnit(
		ctx context.Context, filesystemID string, unitUUID coreunit.UUID,
	) (storageprovisioning.FilesystemAttachment, error)

	// GetFilesystemAttachmentIDs returns the FilesystemAttachmentID information
	// for each of the supplied filesystem attachment UUIDs. If a filesystem
	// attachment does exist for a supplied UUID or if a filesystem attachment
	// is not attached to either a machine or unit then this UUID will be left
	// out of the final result.
	GetFilesystemAttachmentIDs(
		ctx context.Context, filesystemAttachmentUUIDs []string,
	) (map[string]storageprovisioning.FilesystemAttachmentID, error)

	// GetFilesystemForID retrieves the [storageprovisioning.Filesystem] for the
	// supplied filesystem ID.
	GetFilesystemForID(
		ctx context.Context, filesystemID string,
	) (storageprovisioning.Filesystem, error)

	// GetFilesystemAttachmentLife returns the current life value for a
	// filesystem attachment UUID.
	GetFilesystemAttachmentLife(
		ctx context.Context, uuid storageprovisioning.FilesystemAttachmentUUID,
	) (domainlife.Life, error)

	// GetStorageResourceTagsForModel returns the tags to apply to storage in
	// this model.
	GetStorageResourceTagsForModel(
		ctx context.Context,
	) (map[string]string, error)

	// GetVolumeAttachmentIDs returns the VolumeAttachmentID information for
	// each volume attachment UUID supplied. If a UUID does not exist or isn't
	// attached to either a machine or a unit then it will not exist in the
	// result.
	GetVolumeAttachmentIDs(
		ctx context.Context, volumeAttachmentUUIDs []string,
	) (map[string]storageprovisioning.VolumeAttachmentID, error)

	// GetVolumeAttachmentUUIDForVolumeIDMachine returns the volume attachment
	// UUID for the supplied volume ID which is attached to the machine.
	GetVolumeAttachmentUUIDForVolumeIDMachine(
		ctx context.Context, volumeID string, machineUUID machine.UUID,
	) (storageprovisioning.VolumeAttachmentUUID, error)

	// GetVolumeAttachmentUUIDForVolumeUnit returns the volume attachment UUID
	// for the supplied volume ID which is attached to the unit.
	GetVolumeAttachmentUUIDForVolumeIDUnit(
		ctx context.Context, volumeID string, unitUUID coreunit.UUID,
	) (storageprovisioning.VolumeAttachmentUUID, error)

	// GetVolumeParams returns the volume params for the supplied uuid.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/storageprovisioning/errors.VolumeNotFound]
	// when no volume attachment exists for the supplied values.
	GetVolumeParams(
		ctx context.Context, uuid storageprovisioning.VolumeUUID,
	) (storageprovisioning.VolumeParams, error)

	// GetVolumeRemovalParams returns the volume removal params for the supplied
	// uuid.
	GetVolumeRemovalParams(
		ctx context.Context, uuid storageprovisioning.VolumeUUID,
	) (storageprovisioning.VolumeRemovalParams, error)

	// CheckVolumeForIDExists checks if a volume exists for the supplied volume
	// ID. True is returned when a volume exists.
	CheckVolumeForIDExists(context.Context, string) (bool, error)

	// GetVolumeAttachmentParams retrieves the attachment parameters for a given
	// volume attachment.
	GetVolumeAttachmentParams(
		ctx context.Context,
		volumeAttachmentUUID storageprovisioning.VolumeAttachmentUUID,
	) (storageprovisioning.VolumeAttachmentParams, error)

	// GetVolumeAttachmentLife returns the current life value for a volume
	// attachment uuid.
	GetVolumeAttachmentLife(
		ctx context.Context, uuid storageprovisioning.VolumeAttachmentUUID,
	) (domainlife.Life, error)

	// GetVolumeAttachment returns information about a volume attachment.
	GetVolumeAttachment(
		ctx context.Context, uuid storageprovisioning.VolumeAttachmentUUID,
	) (storageprovisioning.VolumeAttachment, error)

	// GetVolumeLife returns the current life value for a volume UUID.
	GetVolumeLife(
		ctx context.Context, uuid storageprovisioning.VolumeUUID,
	) (domainlife.Life, error)

	// GetVolumeUUIDForID returns the UUID for a volume with the supplied
	// id.
	GetVolumeUUIDForID(
		ctx context.Context, volumeID string,
	) (storageprovisioning.VolumeUUID, error)

	// GetVolumeByID retrieves the [storageprovisioning.Volume] for the given
	// volume ID.
	GetVolumeByID(
		ctx context.Context, volumeID string,
	) (storageprovisioning.Volume, error)

	// GetBlockDeviceForVolumeAttachment returns the uuid of the block device
	// set for the specified volume attachment.
	GetBlockDeviceForVolumeAttachment(
		ctx context.Context, uuid storageprovisioning.VolumeAttachmentUUID,
	) (domainblockdevice.BlockDeviceUUID, error)

	// WatchMachineProvisionedFilesystems returns a watcher that emits
	// filesystem IDs, whenever the given machine's provisioned filsystem's life
	// changes.
	WatchMachineProvisionedFilesystems(
		ctx context.Context, machineUUID machine.UUID,
	) (watcher.StringsWatcher, error)

	// WatchModelProvisionedFilesystems returns a watcher that emits filesystem
	// IDs, whenever a model provisioned filsystem's life changes.
	WatchModelProvisionedFilesystems(
		ctx context.Context,
	) (watcher.StringsWatcher, error)

	// WatchModelProvisionedVolumes returns a watcher that emits volume IDs,
	// whenever a model provisioned volume's life changes.
	WatchModelProvisionedVolumes(
		ctx context.Context,
	) (watcher.StringsWatcher, error)

	// WatchMachineProvisionedVolumes returns a watcher that emits volume IDs,
	// whenever the given machine's provisioned volume life changes.
	WatchMachineProvisionedVolumes(
		ctx context.Context, machineUUID machine.UUID,
	) (watcher.StringsWatcher, error)

	// WatchVolumeAttachmentPlans returns a watcher that emits volume attachment
	// plan volume ids, whenever the given machine's volume attachment plan life
	// changes.
	WatchVolumeAttachmentPlans(
		ctx context.Context, machineUUID machine.UUID,
	) (watcher.StringsWatcher, error)

	// WatchModelProvisionedVolumeAttachments returns a watcher that emits
	// volume attachment UUIDs, whenever a model provisioned volume attachment's
	// life changes.
	WatchModelProvisionedVolumeAttachments(
		ctx context.Context,
	) (watcher.StringsWatcher, error)

	// WatchMachineProvisionedVolumeAttachments returns a watcher that emits
	// volume attachment UUIDs, whenever the given machine's provisioned volume
	// attachment's life changes.
	WatchMachineProvisionedVolumeAttachments(
		ctx context.Context, machineUUID machine.UUID,
	) (watcher.StringsWatcher, error)

	// WatchModelProvisionedFilesystemAttachments returns a watcher that emits
	// filesystem attachment UUIDs, whenever a model provisioned filsystem
	// attachment's life changes.
	WatchModelProvisionedFilesystemAttachments(
		ctx context.Context,
	) (watcher.StringsWatcher, error)

	// WatchMachineProvisionedFilesystemAttachments returns a watcher that emits
	// filesystem attachment UUIDs, whenever the given machine's provisioned
	// filsystem attachment's life changes.
	WatchMachineProvisionedFilesystemAttachments(
		ctx context.Context, machineUUID machine.UUID,
	) (watcher.StringsWatcher, error)

	// SetFilesystemProvisionedInfo sets on the provided filesystem the
	// information about the provisioned filesystem.
	SetFilesystemProvisionedInfo(
		ctx context.Context, filesystemID string,
		info storageprovisioning.FilesystemProvisionedInfo,
	) error

	// SetFilesystemAttachmentProvisionedInfoForMachine sets on the provided
	// filesystem the information about the provisioned filesystem attachment.
	SetFilesystemAttachmentProvisionedInfoForMachine(
		ctx context.Context, filesystemID string, machineUUID machine.UUID,
		info storageprovisioning.FilesystemAttachmentProvisionedInfo,
	) error

	// SetFilesystemAttachmentProvisionedInfoForUnit sets on the provided
	// filesystem the information about the provisioned filesystem attachment.
	SetFilesystemAttachmentProvisionedInfoForUnit(
		ctx context.Context, filesystemID string, unitUUID coreunit.UUID,
		info storageprovisioning.FilesystemAttachmentProvisionedInfo,
	) error

	// SetVolumeProvisionedInfo sets on the provided volume the information
	// about the provisioned volume.
	SetVolumeProvisionedInfo(
		ctx context.Context, volumeID string,
		info storageprovisioning.VolumeProvisionedInfo,
	) error

	// SetVolumeAttachmentProvisionedInfo sets on the provided volume the
	// information about the provisioned volume attachment.
	SetVolumeAttachmentProvisionedInfo(
		ctx context.Context,
		volumeAttachmentUUID storageprovisioning.VolumeAttachmentUUID,
		info storageprovisioning.VolumeAttachmentProvisionedInfo,
	) error

	// GetVolumeAttachmentPlan gets the volume attachment plan for the provided
	// uuid.
	GetVolumeAttachmentPlan(
		ctx context.Context, uuid storageprovisioning.VolumeAttachmentPlanUUID,
	) (storageprovisioning.VolumeAttachmentPlan, error)

	// GetVolumeAttachmentPlanUUIDForVolumeIDMachine returns the volume attachment
	// plan uuid for the supplied volume ID which is attached to the machine.
	GetVolumeAttachmentPlanUUIDForVolumeIDMachine(
		ctx context.Context,
		volumeID string,
		machineUUID machine.UUID,
	) (storageprovisioning.VolumeAttachmentPlanUUID, error)

	// CreateVolumeAttachmentPlan creates a volume attachment plan for the
	// provided volume attachment uuid. Returned is the new uuid for the volume
	// attachment plan in the model.
	CreateVolumeAttachmentPlan(
		ctx context.Context,
		attachmentUUID storageprovisioning.VolumeAttachmentUUID,
		deviceType storageprovisioning.PlanDeviceType,
		attrs map[string]string,
	) (storageprovisioning.VolumeAttachmentPlanUUID, error)

	// SetVolumeAttachmentPlanProvisionedInfo sets on the provided volume the
	// information about the provisioned volume attachment plan.
	SetVolumeAttachmentPlanProvisionedInfo(
		ctx context.Context,
		uuid storageprovisioning.VolumeAttachmentPlanUUID,
		info storageprovisioning.VolumeAttachmentPlanProvisionedInfo,
	) error

	// SetVolumeAttachmentPlanProvisionedBlockDevice sets on the provided volume
	// attachment plan the information about the provisioned block device.
	SetVolumeAttachmentPlanProvisionedBlockDevice(
		ctx context.Context,
		uuid storageprovisioning.VolumeAttachmentPlanUUID,
		blockDeviceUUID domainblockdevice.BlockDeviceUUID,
	) error
}
