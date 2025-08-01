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
}

// BlockDeviceService instances can fetch and watch block devices on a machine.
type BlockDeviceService interface {
	// BlockDevices returns the block devices for a specified machine.
	BlockDevices(ctx context.Context, machineId string) ([]blockdevice.BlockDevice, error)
	// WatchBlockDevices returns a new NotifyWatcher watching for
	// changes to block devices associated with the specified machine.
	WatchBlockDevices(ctx context.Context, machineId string) (watcher.NotifyWatcher, error)
}

// StoragePoolGetter instances get a storage pool by name.
type StoragePoolGetter interface {
	// GetStoragePoolByName returns the storage pool with the specified name.
	// The following errors can be expected:
	// - [storageerrors.PoolNotFoundError] if a pool with the specified name does not exist.
	GetStoragePoolByName(ctx context.Context, name string) (domainstorage.StoragePool, error)
}

// ApplicationService is an interface for the application domain service.
type ApplicationService interface {
	// GetUnitLife returns the life status of a unit identified by its name.
	GetUnitLife(ctx context.Context, unitName coreunit.Name) (life.Value, error)
	// GetApplicationLifeByName looks up the life of the specified application, returning
	// an error satisfying [applicationerrors.ApplicationNotFoundError] if the
	// application is not found.
	GetApplicationLifeByName(ctx context.Context, appName string) (life.Value, error)

	// GetUnitUUID returns the UUID for the named unit.
	//
	// The following errors may be returned:
	// - [coreunit.InvalidUnitName] if the unit name is invalid.
	// - [github.com/juju/juju/domain/application/errors.UnitNotFound] if the unit doesn't exist.
	GetUnitUUID(ctx context.Context, unitName coreunit.Name) (coreunit.UUID, error)
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
	// GetFilesystemUUIDForID returns the UUID for a filesystem with the supplied
	// id.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemNotFound]
	// when no filesystem exists for the provided filesystem UUID.
	GetFilesystemUUIDForID(
		ctx context.Context, filesystemID string,
	) (storageprovisioning.FilesystemUUID, error)

	// GetVolumeUUIDForID returns the UUID for a volume with the supplied
	// id.
	//
	// The following errors may be returned:
	// - [corestorage.InvalidStorageID] when the provided id is not valid.
	// - [github.com/juju/juju/domain/storageprovisioning/errors.VolumeNotFound]
	// when no volume exists for the provided volume UUID.
	GetVolumeUUIDForID(
		ctx context.Context, volumeID string,
	) (storageprovisioning.VolumeUUID, error)

	// GetFilesystemLife returns the current life value for a filesystem UUID.
	//
	// The following errors may be returned:
	// - [coreerrors.NotValid] when the filesystem UUID is not valid.
	// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemNotFound]
	// when no filesystem exists for the provided filesystem UUID.
	GetFilesystemLife(
		ctx context.Context, uuid storageprovisioning.FilesystemUUID,
	) (domainlife.Life, error)

	// GetVolumeLife returns the current life value for a volume UUID.
	//
	// The following errors may be returned:
	// - [coreerrors.NotValid] when the volume UUID is not valid.
	// - [github.com/juju/juju/domain/storageprovisioning/errors.VolumeNotFound]
	// when no volume exists for the provided volume UUID.
	GetVolumeLife(
		ctx context.Context, uuid storageprovisioning.VolumeUUID,
	) (domainlife.Life, error)

	// CheckFilesystemForIDExists checks if a filesystem exists for the supplied
	// filesystem ID. True is returned when a filesystem exists.
	CheckFilesystemForIDExists(context.Context, string) (bool, error)

	// GetFilesystemAttachmentUUIDForFilesystemIDUnit returns the filesystem attachment UUID
	// for the supplied filesystem ID which is attached to the unit.
	//
	// The following errors may be returned:
	// - [coreerrors.NotValid] when the provided unit UUID is not valid.
	// - [storageprovisioningerrors.FilesystemNotFound] when no fileystem exists
	// for the supplied id.
	// - [storageprovisioningerrors.FilesystemAttachmentNotFound] when no filesystem
	// attachment exists for the supplied values.
	// - [applicationerrors.UnitNotFound] when no unit exists for the provided unit
	// UUID.
	GetFilesystemAttachmentUUIDForFilesystemIDUnit(
		ctx context.Context, filesystemID string, unitUUID coreunit.UUID,
	) (storageprovisioning.FilesystemAttachmentUUID, error)

	// GetFilesystemAttachmentUUIDForFilesystemIDMachine returns the filesystem attachment
	// UUID for the supplied filesystem id which is attached to the machine.
	//
	// The following errors may be returned:
	// - [coreerrors.NotValid] when the provided unit UUID is not valid.
	// - [storageprovisioningerrors.FilesystemNotFound] when no filesystem exists
	// for the supplied id.
	// - [storageprovisioningerrors.FilesystemAttachmentNotFound] when no filesystem
	// attachment exists for the supplied values.
	// - [machineerrors.MachineNotFound] when no machine exists for the provided
	// machine UUID.
	GetFilesystemAttachmentUUIDForFilesystemIDMachine(
		ctx context.Context,
		filesystemID string, machineUUID machine.UUID,
	) (storageprovisioning.FilesystemAttachmentUUID, error)

	// GetFilesystemAttachmentForMachine retrieves the [storageprovisioning.FilesystemAttachment]
	// for the supplied machine UUID and filesystem ID.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/core/errors.NotValid] when the provided machine
	// UUID is not valid.
	// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
	// machine exists for the provided machine UUID.
	// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemAttachmentNotFound] when no filesystem attachment
	// exists for the provided filesystem ID.
	// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemNotFound] when no filesystem exists for
	// the provided filesystem ID.
	GetFilesystemAttachmentForMachine(
		ctx context.Context, filesystemID string, machineUUID machine.UUID,
	) (storageprovisioning.FilesystemAttachment, error)

	// GetFilesystemAttachmentForUnit retrieves the [storageprovisioning.FilesystemAttachment]
	// for the supplied unit UUID and filesystem ID.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/core/errors.NotValid] when the provided unit UUID
	// is not valid.
	// - [github.com/juju/juju/domain/application/errors.UnitNotFound] when no
	// unit exists for the supplied unit UUID.
	// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemAttachmentNotFound] when no filesystem attachment
	// exists for the provided filesystem ID.
	// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemNotFound] when no filesystem exists for
	// the provided filesystem ID.
	GetFilesystemAttachmentForUnit(
		ctx context.Context, filesystemID string, unitUUID coreunit.UUID,
	) (storageprovisioning.FilesystemAttachment, error)

	// GetFilesystemAttachmentIDs returns the
	// [storageprovisioning.FilesystemAttachmentID] information for each of the
	// supplied filesystem attachment UUIDs. If a filesystem attachment does exist
	// for a supplied UUID or if a filesystem attachment is not attached to either a
	// machine or unit then this UUID will be left out of the final result.
	//
	// It is not considered an error if a filesystem attachment UUID no longer
	// exists as it is expected the caller has already satisfied this requirement
	// themselves.
	//
	// This function exists to help keep supporting storage provisioning facades
	// that have a very week data model about what a filesystem attachment is
	// attached to.
	//
	// All returned values will have either the machine name or unit name value
	// filled out in the [storageprovisioning.FilesystemAttachmentID] struct.
	GetFilesystemAttachmentIDs(
		ctx context.Context, filesystemAttachmentUUIDs []string,
	) (map[string]storageprovisioning.FilesystemAttachmentID, error)

	// GetFilesystemForID retrieves the [storageprovisioning.Filesystem] for the
	// supplied filesystem ID.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemNotFound]
	// when no filesystem exists for the provided filesystem ID.
	GetFilesystemForID(ctx context.Context, filesystemID string) (storageprovisioning.Filesystem, error)

	// GetVolumeAttachmentIDs returns the [storageprovisioning.VolumeAttachmentID]
	// information for each volume attachment UUID supplied. If a UUID does not
	// exist or isn't attached to either a machine or a unit then it will not exist
	// in the result.
	GetVolumeAttachmentIDs(
		ctx context.Context, volumeAttachmentUUIDs []string,
	) (map[string]storageprovisioning.VolumeAttachmentID, error)

	// GetFilesystemAttachmentLife returns the current life value for a filesystem
	// attachment UUID.
	//
	// The following errors may be returned:
	// - [coreerrors.NotValid] when the filesystem attachment UUID is not valid.
	// - [storageprovisioningerrors.FilesystemAttachmentNotFound] when no filesystem
	// attachment exists for the provided UUID.
	GetFilesystemAttachmentLife(
		ctx context.Context, uuid storageprovisioning.FilesystemAttachmentUUID,
	) (domainlife.Life, error)

	// GetVolumeAttachmentUUIDForVolumeIDMachine returns the volume attachment
	// UUID for the supplied volume ID which is attached to the machine.
	//
	// The following errors may be returned:
	// - [corestorage.InvalidStorageID] when the provided id is not valid.
	// - [coreerrors.NotValid] when the provided machine UUID is not valid.
	// - [storageprovisioningerrors.VolumeNotFound] when no volume exists for the
	// supplied id.
	// - [storageprovisioningerrors.VolumeAttachmentNotFound] when no volume
	// attachment exists for the supplied values.
	// - [machineerrors.MachineNotFound] when no machine exists for the provided
	// machine UUID.
	GetVolumeAttachmentUUIDForVolumeIDMachine(
		ctx context.Context, volumeID string, machineUUID machine.UUID,
	) (storageprovisioning.VolumeAttachmentUUID, error)

	// GetVolumeAttachmentUUIDForVolumeUnit returns the volume attachment UUID
	// for the supplied volume ID which is attached to the unit.
	//
	// The following errors may be returned:
	// - [corestorage.InvalidStorageID] when the provided id is not valid.
	// - [coreerrors.NotValid] when the provided unit UUID is not valid.
	// - [storageprovisioningerrors.VolumeNotFound] when no volume exists for the
	// supplied id.
	// - [storageprovisioningerrors.VolumeAttachmentNotFound] when no volume
	// attachment exists for the supplied values.
	// - [applicationerrors.UnitNotFound] when no unit exists for the provided unit
	// UUID.
	GetVolumeAttachmentUUIDForVolumeIDUnit(
		ctx context.Context, volumeID string, unitUUID coreunit.UUID,
	) (storageprovisioning.VolumeAttachmentUUID, error)

	// GetVolumeAttachmentLife returns the current life value for a volume
	// attachment uuid.
	//
	// The following errors may be returned:
	// - [coreerrors.NotValid] when the volume attachment UUID is not valid.
	// - [github.com/juju/juju/domain/storageprovisioning/errors.VolumeAttachmentNotFound]
	// when no volume attachment exists for the provided UUID.
	GetVolumeAttachmentLife(
		ctx context.Context, uuid storageprovisioning.VolumeAttachmentUUID,
	) (domainlife.Life, error)

	// WatchMachineProvisionedFilesystems returns a watcher that emits filesystem IDs,
	// whenever the given machine's provisioned filsystem's life changes.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/core/errors.NotValid] when the supplied machine
	// UUID is not valid.
	// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
	// machine exists for the provided machine UUID.
	WatchMachineProvisionedFilesystems(
		ctx context.Context, machineUUID machine.UUID,
	) (watcher.StringsWatcher, error)

	// WatchModelProvisionedFilesystems returns a watcher that emits filesystem IDs,
	// whenever a model provisioned filsystem's life changes.
	WatchModelProvisionedFilesystems(ctx context.Context) (watcher.StringsWatcher, error)

	// WatchModelProvisionedVolumes returns a watcher that emits volume IDs,
	// whenever a model provisioned volume's life changes.
	WatchModelProvisionedVolumes(ctx context.Context) (watcher.StringsWatcher, error)

	// WatchMachineProvisionedVolumes returns a watcher that emits volume IDs,
	// whenever the given machine's provisioned volume life changes.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/core/errors.NotValid] when the provided machine
	// UUID is not valid.
	// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
	// machine exists for the provided machine UUID.
	WatchMachineProvisionedVolumes(ctx context.Context, machineUUID machine.UUID) (watcher.StringsWatcher, error)

	// WatchVolumeAttachmentPlans returns a watcher that emits volume attachment
	// plan volume ids, whenever the given machine's volume attachment plan life
	// changes.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/core/errors.NotValid] when the provided machine
	// UUID is not valid.
	// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
	// machine exists for the provided machine UUID.
	WatchVolumeAttachmentPlans(ctx context.Context, machineUUID machine.UUID) (watcher.StringsWatcher, error)

	// WatchModelProvisionedVolumeAttachments returns a watcher that emits volume
	// attachment UUIDs, whenever a model provisioned volume attachment's life
	// changes.
	WatchModelProvisionedVolumeAttachments(ctx context.Context) (watcher.StringsWatcher, error)

	// WatchMachineProvisionedVolumeAttachments returns a watcher that emits volume
	// attachment UUIDs, whenever the given machine's provisioned volume
	// attachment's life changes.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/core/errors.NotValid] when the provided machine
	// UUID is not valid.
	// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
	// machine exists for the provided machine UUID.
	WatchMachineProvisionedVolumeAttachments(
		ctx context.Context, machineUUID machine.UUID,
	) (watcher.StringsWatcher, error)

	// WatchModelProvisionedFilesystemAttachments returns a watcher that emits
	// filesystem attachment UUIDs, whenever a model provisioned filsystem
	// attachment's life changes.
	WatchModelProvisionedFilesystemAttachments(ctx context.Context) (watcher.StringsWatcher, error)

	// WatchMachineProvisionedFilesystemAttachments returns a watcher that emits
	// filesystem attachment UUIDs, whenever the given machine's provisioned
	// filsystem attachment's life changes.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/core/errors.NotValid] when the provided machine
	// UUID is not valid.
	// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
	// machine exists for the provided machine UUID.
	WatchMachineProvisionedFilesystemAttachments(
		ctx context.Context, machineUUID machine.UUID,
	) (watcher.StringsWatcher, error)

	// SetFilesystemProvisionedInfo sets on the provided filesystem the information
	// about the provisioned filesystem.
	// The following errors may be returned:
	// - [storageprovisioningerrors.FilesystemNotFound] when no filesystem exists
	// for the provided filesystem id.
	SetFilesystemProvisionedInfo(ctx context.Context, filesystemID string, info storageprovisioning.FilesystemProvisionedInfo) error

	// SetFilesystemAttachmentProvisionedInfoForMachine sets on the provided
	// filesystem the information about the provisioned filesystem attachment.
	// The following errors may be returned:
	// - [storageprovisioningerrors.FilesystemAttachmentNotFound] when no filesystem
	// attachment exists for the provided filesystem id.
	SetFilesystemAttachmentProvisionedInfoForMachine(ctx context.Context, filesystemID string, machineUUID machine.UUID, info storageprovisioning.FilesystemAttachmentProvisionedInfo) error

	// SetFilesystemAttachmentProvisionedInfoForUnit sets on the provided
	// filesystem the information about the provisioned filesystem attachment.
	// The following errors may be returned:
	// - [storageprovisioningerrors.FilesystemAttachmentNotFound] when no filesystem
	// attachment exists for the provided filesystem id.
	SetFilesystemAttachmentProvisionedInfoForUnit(ctx context.Context, filesystemID string, unitUUID coreunit.UUID, info storageprovisioning.FilesystemAttachmentProvisionedInfo) error

	// SetVolumeProvisionedInfo sets on the provided volume the information about
	// the provisioned volume.
	// The following errors may be returned:
	// - [storageprovisioningerrors.VolumeNotFound] when no volume exists for the
	// provided volume id.
	SetVolumeProvisionedInfo(ctx context.Context, volumeID string, info storageprovisioning.VolumeProvisionedInfo) error

	// SetVolumeAttachmentProvisionedInfo sets on the provided volume the information
	// about the provisioned volume attachment.
	// The following errors may be returned:
	// - [storageprovisioningerrors.VolumeAttachmentNotFound] when no volume
	// attachmentexists for the provided volume attachment id.
	SetVolumeAttachmentProvisionedInfo(ctx context.Context, volumeAttachmentUUID storageprovisioning.VolumeAttachmentUUID, info storageprovisioning.VolumeAttachmentProvisionedInfo) error

	// SetVolumeAttachmentPlanProvisionedInfo sets on the provided volume the
	// information about the provisioned volume attachment plan.
	// The following errors may be returned:
	// - [storageprovisioningerrors.VolumeAttachmentPlanNotFound] when no volume
	// attachment plan exists for the provided volume attachment id.
	SetVolumeAttachmentPlanProvisionedInfo(ctx context.Context, volumeID string, machineUUID machine.UUID, info storageprovisioning.VolumeAttachmentPlanProvisionedInfo) error

	// SetVolumeAttachmentPlanProvisionedBlockDevice sets on the provided volume the
	// information about the provisioned volume attachment.
	// The following errors may be returned:
	// - [storageprovisioningerrors.VolumeAttachmentPlanNotFound] when no volume
	// attachment plan exists for the provided volume attachment id.
	// - [storageprovisioningerrors.BlockDeviceNotFound] when no block device exists
	// for the provided block device uuuid.
	SetVolumeAttachmentPlanProvisionedBlockDevice(ctx context.Context, volumeID string, machineUUID machine.UUID, info blockdevice.BlockDevice) error
}
