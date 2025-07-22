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
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
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
	GetUnitLife(ctx context.Context, unitName unit.Name) (life.Value, error)
	// GetApplicationLifeByName looks up the life of the specified application, returning
	// an error satisfying [applicationerrors.ApplicationNotFoundError] if the
	// application is not found.
	GetApplicationLifeByName(ctx context.Context, appName string) (life.Value, error)
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
	// GetFilesystem retrieves the [storageprovisioning.Filesystem] for the
	// supplied filesystem id.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemNotFound] when no filesystem
	// exists for the provided filesystem id.
	GetFilesystem(ctx context.Context, filesystemID string) (storageprovisioning.Filesystem, error)

	// GetFilesystemAttachment retrieves the [storageprovisioning.FilesystemAttachment]
	// for the supplied net node uuid and filesystem id.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/core/errors.NotValid] when the provided machine uuid
	// is not valid.
	// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
	// machine exists for the provided machine UUUID.
	// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemAttachmentNotFound] when no filesystem attachment
	// exists for the provided filesystem id.
	// - [github.com/juju/juju/domain/storageprovisioning/errors.FilesystemNotFound] when no filesystem exists for
	// the provided filesystem id.
	GetFilesystemAttachment(
		ctx context.Context, machineUUID machine.UUID, filesystemID string,
	) (storageprovisioning.FilesystemAttachment, error)

	// WatchMachineProvisionedFilesystems returns a watcher that emits filesystem IDs,
	// whenever the given machine's provisioned filsystem's life changes.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/core/errors.NotValid] when the supplied machine uuid
	// is not valid.
	// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
	// machine exists for the provided machine uuid.
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
	// - [github.com/juju/juju/core/errors.NotValid] when the provided machine uuid
	// is not valid.
	// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
	// machine exists for the provided machine UUUID.
	WatchMachineProvisionedVolumes(ctx context.Context, machineUUID machine.UUID) (watcher.StringsWatcher, error)

	// WatchVolumeAttachmentPlans returns a watcher that emits volume attachment
	// plan volume ids, whenever the given machine's volume attachment plan life
	// changes.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/core/errors.NotValid] when the provided machine uuid
	// is not valid.
	// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
	// machine exists for the provided machine UUUID.
	WatchVolumeAttachmentPlans(ctx context.Context, machineUUID machine.UUID) (watcher.StringsWatcher, error)

	// GetVolumeAttachmentIDs returns the [storageprovisioning.VolumeAttachmentID]
	// information for each volume attachment uuid supplied. If a uuid does not
	// exist or isn't attached to either a machine or a unit then it will not exist
	// in the result.
	GetVolumeAttachmentIDs(
		ctx context.Context, volumeAttachmentUUIDs []string,
	) (map[string]storageprovisioning.VolumeAttachmentID, error)

	// WatchModelProvisionedVolumeAttachments returns a watcher that emits volume
	// attachment UUIDs, whenever a model provisioned volume attachment's life
	// changes.
	WatchModelProvisionedVolumeAttachments(ctx context.Context) (watcher.StringsWatcher, error)

	// WatchMachineProvisionedVolumeAttachments returns a watcher that emits volume
	// attachment UUIDs, whenever the given machine's provisioned volume
	// attachment's life changes.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/core/errors.NotValid] when the provided machine uuid
	// is not valid.
	// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
	// machine exists for the provided machine UUUID.
	WatchMachineProvisionedVolumeAttachments(
		ctx context.Context, machineUUID machine.UUID,
	) (watcher.StringsWatcher, error)

	// GetFilesystemAttachmentIDs returns the
	// [storageprovisioning.FilesystemAttachmentID] information for each of the
	// supplied filesystem attachment uuids. If a filesystem attachment does exist
	// for a supplied uuid or if a filesystem attachment is not attached to either a
	// machine or unit then this uuid will be left out of the final result.
	//
	// It is not considered an error if a filesystem attachment uuid no longer
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

	// WatchModelProvisionedFilesystemAttachments returns a watcher that emits
	// filesystem attachment UUIDs, whenever a model provisioned filsystem
	// attachment's life changes.
	WatchModelProvisionedFilesystemAttachments(ctx context.Context) (watcher.StringsWatcher, error)

	// WatchMachineProvisionedFilesystemAttachments returns a watcher that emits
	// filesystem attachment UUIDs, whenever the given machine's provisioned
	// filsystem attachment's life changes.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/core/errors.NotValid] when the provided machine uuid
	// is not valid.
	// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
	// machine exists for the provided machine UUUID.
	WatchMachineProvisionedFilesystemAttachments(
		ctx context.Context, machineUUID machine.UUID,
	) (watcher.StringsWatcher, error)
}
