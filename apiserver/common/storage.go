// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"path"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

// StorageInterface is an interface for obtaining information about storage
// instances and related entities.
type StorageInterface interface {
	StorageInstance(names.StorageTag) (state.StorageInstance, error)
	StorageInstanceVolume(names.StorageTag) (state.Volume, error)
	UnitAssignedMachine(names.UnitTag) (names.MachineTag, error)
	VolumeAttachment(names.MachineTag, names.VolumeTag) (state.VolumeAttachment, error)
	WatchVolumeAttachment(names.MachineTag, names.VolumeTag) state.NotifyWatcher
}

// StorageAttachmentInfo returns the StorageAttachmentInfo for the specified
// StorageAttachment by gathering information from related entities (volumes,
// filesystems).
func StorageAttachmentInfo(st StorageInterface, att state.StorageAttachment) (*storage.StorageAttachmentInfo, error) {
	machineTag, err := st.UnitAssignedMachine(att.Unit())
	if err != nil {
		return nil, errors.Trace(err)
	}
	storageInstance, err := st.StorageInstance(att.StorageInstance())
	if err != nil {
		return nil, errors.Annotate(err, "getting storage instance")
	}
	switch storageInstance.Kind() {
	case state.StorageKindBlock:
		return volumeStorageAttachmentInfo(st, storageInstance, machineTag)
	case state.StorageKindFilesystem:
		// TODO(axw) handle filesystem kind once
		// the state.Filesystem branch lands.
		return nil, errors.NotSupportedf("filesystem storage")
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
	return &storage.StorageAttachmentInfo{devicePath}, nil
}

// WatchStorageAttachment returns a state.NotifyWatcher that reacts to changes
// to the VolumeAttachment or FilesystemAttachment corresponding to the tags
// specified.
func WatchStorageAttachment(st StorageInterface, storageTag names.StorageTag, unitTag names.UnitTag) (state.NotifyWatcher, error) {
	machineTag, err := st.UnitAssignedMachine(unitTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	storageInstance, err := st.StorageInstance(storageTag)
	if err != nil {
		return nil, errors.Annotate(err, "getting storage instance")
	}
	switch storageInstance.Kind() {
	case state.StorageKindBlock:
		volume, err := st.StorageInstanceVolume(storageTag)
		if err != nil {
			return nil, errors.Annotate(err, "getting storage volume")
		}
		return st.WatchVolumeAttachment(machineTag, volume.VolumeTag()), nil
	case state.StorageKindFilesystem:
		// TODO(axw) handle filesystem kind once
		// the state.Filesystem branch lands.
		return nil, errors.NotSupportedf("filesystem storage")
	}
	return nil, errors.Errorf("invalid storage kind %v", storageInstance.Kind())
}

// MatchingBlockDevice finds the block device that matches the
// provided volume info and volume attachment info.
func MatchingBlockDevice(
	blockDevices []state.BlockDeviceInfo,
	volumeInfo state.VolumeInfo,
	attachmentInfo state.VolumeAttachmentInfo,
) (*state.BlockDeviceInfo, bool) {
	for _, dev := range blockDevices {
		if volumeInfo.Serial != "" {
			if volumeInfo.Serial == dev.Serial {
				return &dev, true
			}
		} else if attachmentInfo.DeviceName == dev.DeviceName {
			return &dev, true
		}
	}
	return nil, false
}

var errNoDevicePath = errors.New("cannot determine device path: no serial or persistent device name")

// volumeAttachmentDevicePath returns the absolute device path for
// a volume attachment. The value is only meaningful in the context
// of the machine that the volume is attached to.
func volumeAttachmentDevicePath(
	volumeInfo state.VolumeInfo,
	volumeAttachmentInfo state.VolumeAttachmentInfo,
) (string, error) {
	if volumeInfo.Serial != "" {
		return path.Join("/dev/disk/by-id", volumeInfo.Serial), nil
	} else if volumeAttachmentInfo.DeviceName != "" {
		return path.Join("/dev", volumeAttachmentInfo.DeviceName), nil
	}
	return "", errNoDevicePath
}
