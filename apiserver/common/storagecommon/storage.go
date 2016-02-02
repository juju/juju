// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package storagecommon provides common storage-related services
// for API server facades.
package storagecommon

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

// StorageInterface is an interface for obtaining information about storage
// instances and related entities.
type StorageInterface interface {
	// StorageInstance returns the state.StorageInstance corresponding
	// to the specified storage tag.
	StorageInstance(names.StorageTag) (state.StorageInstance, error)

	// StorageInstanceFilesystem returns the state.Filesystem assigned
	// to the storage instance with the specified storage tag.
	StorageInstanceFilesystem(names.StorageTag) (state.Filesystem, error)

	// StorageInstanceVolume returns the state.Volume assigned to the
	// storage instance with the specified storage tag.
	StorageInstanceVolume(names.StorageTag) (state.Volume, error)

	// FilesystemAttachment returns the state.FilesystemAttachment
	// corresponding to the identified machine and filesystem.
	FilesystemAttachment(names.MachineTag, names.FilesystemTag) (state.FilesystemAttachment, error)

	// VolumeAttachment returns the state.VolumeAttachment corresponding
	// to the identified machine and volume.
	VolumeAttachment(names.MachineTag, names.VolumeTag) (state.VolumeAttachment, error)

	// WatchStorageAttachment watches for changes to the storage attachment
	// corresponding to the identfified unit and storage instance.
	WatchStorageAttachment(names.StorageTag, names.UnitTag) state.NotifyWatcher

	// WatchFilesystemAttachment watches for changes to the filesystem
	// attachment corresponding to the identfified machine and filesystem.
	WatchFilesystemAttachment(names.MachineTag, names.FilesystemTag) state.NotifyWatcher

	// WatchVolumeAttachment watches for changes to the volume attachment
	// corresponding to the identfified machine and volume.
	WatchVolumeAttachment(names.MachineTag, names.VolumeTag) state.NotifyWatcher

	// WatchBlockDevices watches for changes to block devices associated
	// with the specified machine.
	WatchBlockDevices(names.MachineTag) state.NotifyWatcher

	// BlockDevices returns information about block devices published
	// for the specified machine.
	BlockDevices(names.MachineTag) ([]state.BlockDeviceInfo, error)
}

// StorageAttachmentInfo returns the StorageAttachmentInfo for the specified
// StorageAttachment by gathering information from related entities (volumes,
// filesystems).
//
// StorageAttachmentInfo returns an error satisfying errors.IsNotProvisioned
// if the storage attachment is not yet fully provisioned and ready for use
// by a charm.
func StorageAttachmentInfo(
	st StorageInterface,
	att state.StorageAttachment,
	machineTag names.MachineTag,
) (*storage.StorageAttachmentInfo, error) {
	storageInstance, err := st.StorageInstance(att.StorageInstance())
	if err != nil {
		return nil, errors.Annotate(err, "getting storage instance")
	}
	switch storageInstance.Kind() {
	case state.StorageKindBlock:
		return volumeStorageAttachmentInfo(st, storageInstance, machineTag)
	case state.StorageKindFilesystem:
		return filesystemStorageAttachmentInfo(st, storageInstance, machineTag)
	}
	return nil, errors.Errorf("invalid storage kind %v", storageInstance.Kind())
}

func volumeStorageAttachmentInfo(
	st StorageInterface,
	storageInstance state.StorageInstance,
	machineTag names.MachineTag,
) (*storage.StorageAttachmentInfo, error) {
	storageTag := storageInstance.StorageTag()
	volume, err := st.StorageInstanceVolume(storageTag)
	if err != nil {
		return nil, errors.Annotate(err, "getting volume")
	}
	volumeInfo, err := volume.Info()
	if err != nil {
		return nil, errors.Annotate(err, "getting volume info")
	}
	volumeAttachment, err := st.VolumeAttachment(machineTag, volume.VolumeTag())
	if err != nil {
		return nil, errors.Annotate(err, "getting volume attachment")
	}
	volumeAttachmentInfo, err := volumeAttachment.Info()
	if err != nil {
		return nil, errors.Annotate(err, "getting volume attachment info")
	}
	blockDevices, err := st.BlockDevices(machineTag)
	if err != nil {
		return nil, errors.Annotate(err, "getting block devices")
	}
	blockDevice, ok := MatchingBlockDevice(
		blockDevices,
		volumeInfo,
		volumeAttachmentInfo,
	)
	if !ok {
		// We must not say that a block-kind storage attachment is
		// provisioned until its block device has shown up on the
		// machine, otherwise the charm may attempt to use it and
		// fail.
		return nil, errors.NotProvisionedf("%v", names.ReadableString(storageTag))
	}
	devicePath, err := volumeAttachmentDevicePath(
		volumeInfo,
		volumeAttachmentInfo,
		*blockDevice,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &storage.StorageAttachmentInfo{
		storage.StorageKindBlock,
		devicePath,
	}, nil
}

func filesystemStorageAttachmentInfo(
	st StorageInterface,
	storageInstance state.StorageInstance,
	machineTag names.MachineTag,
) (*storage.StorageAttachmentInfo, error) {
	storageTag := storageInstance.StorageTag()
	filesystem, err := st.StorageInstanceFilesystem(storageTag)
	if err != nil {
		return nil, errors.Annotate(err, "getting filesystem")
	}
	filesystemAttachment, err := st.FilesystemAttachment(machineTag, filesystem.FilesystemTag())
	if err != nil {
		return nil, errors.Annotate(err, "getting filesystem attachment")
	}
	filesystemAttachmentInfo, err := filesystemAttachment.Info()
	if err != nil {
		return nil, errors.Annotate(err, "getting filesystem attachment info")
	}
	return &storage.StorageAttachmentInfo{
		storage.StorageKindFilesystem,
		filesystemAttachmentInfo.MountPoint,
	}, nil
}

// WatchStorageAttachment returns a state.NotifyWatcher that reacts to changes
// to the VolumeAttachmentInfo or FilesystemAttachmentInfo corresponding to the
// tags specified.
func WatchStorageAttachment(
	st StorageInterface,
	storageTag names.StorageTag,
	machineTag names.MachineTag,
	unitTag names.UnitTag,
) (state.NotifyWatcher, error) {
	storageInstance, err := st.StorageInstance(storageTag)
	if err != nil {
		return nil, errors.Annotate(err, "getting storage instance")
	}
	var watchers []state.NotifyWatcher
	switch storageInstance.Kind() {
	case state.StorageKindBlock:
		volume, err := st.StorageInstanceVolume(storageTag)
		if err != nil {
			return nil, errors.Annotate(err, "getting storage volume")
		}
		// We need to watch both the volume attachment, and the
		// machine's block devices. A volume attachment's block
		// device could change (most likely, become present).
		watchers = []state.NotifyWatcher{
			st.WatchVolumeAttachment(machineTag, volume.VolumeTag()),
			// TODO(axw) 2015-09-30 #1501203
			// We should filter the events to only those relevant
			// to the volume attachment. This means we would need
			// to either start th block device watcher after we
			// have provisioned the volume attachment (cleaner?),
			// or have the filter ignore changes until the volume
			// attachment is provisioned.
			st.WatchBlockDevices(machineTag),
		}
	case state.StorageKindFilesystem:
		filesystem, err := st.StorageInstanceFilesystem(storageTag)
		if err != nil {
			return nil, errors.Annotate(err, "getting storage filesystem")
		}
		watchers = []state.NotifyWatcher{
			st.WatchFilesystemAttachment(machineTag, filesystem.FilesystemTag()),
		}
	default:
		return nil, errors.Errorf("invalid storage kind %v", storageInstance.Kind())
	}
	watchers = append(watchers, st.WatchStorageAttachment(storageTag, unitTag))
	return common.NewMultiNotifyWatcher(watchers...), nil
}

// volumeAttachmentDevicePath returns the absolute device path for
// a volume attachment. The value is only meaningful in the context
// of the machine that the volume is attached to.
func volumeAttachmentDevicePath(
	volumeInfo state.VolumeInfo,
	volumeAttachmentInfo state.VolumeAttachmentInfo,
	blockDevice state.BlockDeviceInfo,
) (string, error) {
	if volumeInfo.HardwareId != "" || volumeAttachmentInfo.DeviceName != "" || volumeAttachmentInfo.DeviceLink != "" {
		// Prefer the volume attachment's information over what is
		// in the published block device information.
		var deviceLinks []string
		if volumeAttachmentInfo.DeviceLink != "" {
			deviceLinks = []string{volumeAttachmentInfo.DeviceLink}
		}
		return storage.BlockDevicePath(storage.BlockDevice{
			HardwareId:  volumeInfo.HardwareId,
			DeviceName:  volumeAttachmentInfo.DeviceName,
			DeviceLinks: deviceLinks,
		})
	}
	return storage.BlockDevicePath(BlockDeviceFromState(blockDevice))
}

// MaybeAssignedStorageInstance calls the provided function to get a
// StorageTag, and returns the corresponding state.StorageInstance if
// it didn't return an errors.IsNotAssigned error, or nil if it did.
func MaybeAssignedStorageInstance(
	getTag func() (names.StorageTag, error),
	getStorageInstance func(names.StorageTag) (state.StorageInstance, error),
) (state.StorageInstance, error) {
	tag, err := getTag()
	if err == nil {
		return getStorageInstance(tag)
	} else if errors.IsNotAssigned(err) {
		return nil, nil
	}
	return nil, errors.Trace(err)
}

// storageTags returns the tags that should be set on a volume or filesystem,
// if the provider supports them.
func storageTags(
	storageInstance state.StorageInstance,
	cfg *config.Config,
) (map[string]string, error) {
	uuid, _ := cfg.UUID()
	storageTags := tags.ResourceTags(names.NewModelTag(uuid), cfg)
	if storageInstance != nil {
		storageTags[tags.JujuStorageInstance] = storageInstance.Tag().Id()
		storageTags[tags.JujuStorageOwner] = storageInstance.Owner().Id()
	}
	return storageTags, nil
}
