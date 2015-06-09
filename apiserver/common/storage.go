// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"path"

	"github.com/juju/errors"
	"github.com/juju/names"

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
	// attachment corresponding to the identfified machien and filesystem.
	WatchFilesystemAttachment(names.MachineTag, names.FilesystemTag) state.NotifyWatcher

	// WatchVolumeAttachment watches for changes to the volume attachment
	// corresponding to the identfified machien and volume.
	WatchVolumeAttachment(names.MachineTag, names.VolumeTag) state.NotifyWatcher
}

// StorageAttachmentInfo returns the StorageAttachmentInfo for the specified
// StorageAttachment by gathering information from related entities (volumes,
// filesystems).
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
	devicePath, err := volumeAttachmentDevicePath(
		volumeInfo,
		volumeAttachmentInfo,
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
// to the VolumeAttachmentInfo or FilesystemAttachmentInfo corresponding to the tags
// specified.
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
	var w state.NotifyWatcher
	switch storageInstance.Kind() {
	case state.StorageKindBlock:
		volume, err := st.StorageInstanceVolume(storageTag)
		if err != nil {
			return nil, errors.Annotate(err, "getting storage volume")
		}
		w = st.WatchVolumeAttachment(machineTag, volume.VolumeTag())
	case state.StorageKindFilesystem:
		filesystem, err := st.StorageInstanceFilesystem(storageTag)
		if err != nil {
			return nil, errors.Annotate(err, "getting storage filesystem")
		}
		w = st.WatchFilesystemAttachment(machineTag, filesystem.FilesystemTag())
	default:
		return nil, errors.Errorf("invalid storage kind %v", storageInstance.Kind())
	}
	w2 := st.WatchStorageAttachment(storageTag, unitTag)
	return newMultiNotifyWatcher(w, w2), nil
}

var errNoDevicePath = errors.New("cannot determine device path: no serial or persistent device name")

// volumeAttachmentDevicePath returns the absolute device path for
// a volume attachment. The value is only meaningful in the context
// of the machine that the volume is attached to.
func volumeAttachmentDevicePath(
	volumeInfo state.VolumeInfo,
	volumeAttachmentInfo state.VolumeAttachmentInfo,
) (string, error) {
	if volumeInfo.HardwareId != "" {
		return path.Join("/dev/disk/by-id", volumeInfo.HardwareId), nil
	} else if volumeAttachmentInfo.DeviceName != "" {
		return path.Join("/dev", volumeAttachmentInfo.DeviceName), nil
	}
	return "", errNoDevicePath
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
	storageTags := tags.ResourceTags(names.NewEnvironTag(uuid), cfg)
	if storageInstance != nil {
		storageTags[tags.JujuStorageInstance] = storageInstance.Tag().Id()
		storageTags[tags.JujuStorageOwner] = storageInstance.Owner().Id()
	}
	return storageTags, nil
}
