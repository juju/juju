// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storagecommon

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

// StorageAccess is an interface for obtaining information about storage
// instances and any associated volume and/or filesystem instances.
type StorageAccess interface {
	// StorageInstance returns the state.StorageInstance corresponding
	// to the specified storage tag.
	StorageInstance(names.StorageTag) (state.StorageInstance, error)

	// UnitStorageAttachments returns the storage attachments for the
	// specified unit.
	UnitStorageAttachments(names.UnitTag) ([]state.StorageAttachment, error)
}

// VolumeAccess is an interface for obtaining information about
// block storage instances and related entities.
type VolumeAccess interface {
	// StorageInstanceVolume returns the state.Volume assigned to the
	// storage instance with the specified storage tag.
	StorageInstanceVolume(names.StorageTag) (state.Volume, error)

	// VolumeAttachment returns the state.VolumeAttachment corresponding
	// to the specified host and volume.
	VolumeAttachment(names.Tag, names.VolumeTag) (state.VolumeAttachment, error)

	// VolumeAttachmentPlan returns state.VolumeAttachmentPlan corresponding
	// to the specified machine and volume
	VolumeAttachmentPlan(names.Tag, names.VolumeTag) (state.VolumeAttachmentPlan, error)

	// BlockDevices returns information about block devices published
	// for the specified machine.
	BlockDevices(names.MachineTag) ([]state.BlockDeviceInfo, error)
}

// FilesystemAccess is an interface for obtaining information about
// filesystem storage instances and related entities.
type FilesystemAccess interface {
	// StorageInstanceFilesystem returns the state.Filesystem assigned
	// to the storage instance with the specified storage tag.
	StorageInstanceFilesystem(names.StorageTag) (state.Filesystem, error)

	// FilesystemAttachment returns the state.FilesystemAttachment
	// corresponding to the specified host and filesystem.
	FilesystemAttachment(names.Tag, names.FilesystemTag) (state.FilesystemAttachment, error)
}

// StorageAttachmentInfo is called by the uniter facade to get info needed to
// run storage hooks and also the client facade to display storage info.

// StorageAttachmentInfo returns the StorageAttachmentInfo for the specified
// StorageAttachment by gathering information from related entities (volumes,
// filesystems).
//
// StorageAttachmentInfo returns an error satisfying errors.IsNotProvisioned
// if the storage attachment is not yet fully provisioned and ready for use
// by a charm.
func StorageAttachmentInfo(
	st StorageAccess,
	stVolume VolumeAccess,
	stFile FilesystemAccess,
	att state.StorageAttachment,
	hostTag names.Tag,
) (*storage.StorageAttachmentInfo, error) {
	storageInstance, err := st.StorageInstance(att.StorageInstance())
	if err != nil {
		return nil, errors.Annotate(err, "getting storage instance")
	}
	switch storageInstance.Kind() {
	case state.StorageKindBlock:
		if stVolume == nil {
			return nil, errors.NotImplementedf("BlockStorage instance")
		}
		return volumeStorageAttachmentInfo(stVolume, storageInstance, hostTag)
	case state.StorageKindFilesystem:
		if stFile == nil {
			return nil, errors.NotImplementedf("FilesystemStorage instance")
		}
		return filesystemStorageAttachmentInfo(stFile, storageInstance, hostTag)
	}
	return nil, errors.Errorf("invalid storage kind %v", storageInstance.Kind())
}

func volumeStorageAttachmentInfo(
	st VolumeAccess,
	storageInstance state.StorageInstance,
	hostTag names.Tag,
) (*storage.StorageAttachmentInfo, error) {
	storageTag := storageInstance.StorageTag()
	volume, err := st.StorageInstanceVolume(storageTag)
	if errors.IsNotFound(err) {
		// If the unit of the storage attachment is not
		// assigned to a machine, there will be no volume
		// yet. Handle this gracefully by saying that the
		// volume is not yet provisioned.
		return nil, errors.NotProvisionedf("volume for storage %q", storageTag.Id())
	} else if err != nil {
		return nil, errors.Annotate(err, "getting volume")
	}
	volumeInfo, err := volume.Info()
	if err != nil {
		return nil, errors.Annotate(err, "getting volume info")
	}
	volumeAttachment, err := st.VolumeAttachment(hostTag, volume.VolumeTag())
	if err != nil {
		return nil, errors.Annotate(err, "getting volume attachment")
	}
	volumeAttachmentInfo, err := volumeAttachment.Info()
	if err != nil {
		return nil, errors.Annotate(err, "getting volume attachment info")
	}

	blockDeviceInfo := state.BlockDeviceInfo{}
	volumeAttachmentPlan, err := st.VolumeAttachmentPlan(hostTag, volume.VolumeTag())
	logger.Infof("alvin volumeStorageAttachmentInfo volumeAttachmentPlan: %+v", blockDeviceInfo)

	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, errors.Annotate(err, "getting attachment plans")
		}
	} else {
		blockDeviceInfo, err = volumeAttachmentPlan.BlockDeviceInfo()
		if err != nil {
			if !errors.IsNotFound(err) {
				return nil, errors.Annotate(err, "getting block device info")
			}
		}
	}

	logger.Infof("alvin volumeStorageAttachmentInfo blockDeviceInfo: %+v", blockDeviceInfo)
	// TODO(caas) - we currently only support block devices on machines.
	if hostTag.Kind() != names.MachineTagKind {
		return nil, errors.NotProvisionedf("%v", names.ReadableString(storageTag))
	}
	blockDevices, err := st.BlockDevices(hostTag.(names.MachineTag))
	if err != nil {
		return nil, errors.Annotate(err, "getting block devices")
	}
	blockDevice, ok := MatchingVolumeBlockDevice(
		blockDevices,
		volumeInfo,
		volumeAttachmentInfo,
		blockDeviceInfo,
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
	st FilesystemAccess,
	storageInstance state.StorageInstance,
	hostTag names.Tag,
) (*storage.StorageAttachmentInfo, error) {
	storageTag := storageInstance.StorageTag()
	filesystem, err := st.StorageInstanceFilesystem(storageTag)
	if errors.IsNotFound(err) {
		// If the unit of the storage attachment is not
		// assigned to a machine, there will be no filesystem
		// yet. Handle this gracefully by saying that the
		// filesystem is not yet provisioned.
		return nil, errors.NotProvisionedf("filesystem for storage %q", storageTag.Id())
	} else if err != nil {
		return nil, errors.Annotate(err, "getting filesystem")
	}
	filesystemAttachment, err := st.FilesystemAttachment(hostTag, filesystem.FilesystemTag())
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

// volumeAttachmentDevicePath returns the absolute device path for
// a volume attachment. The value is only meaningful in the context
// of the machine that the volume is attached to.
func volumeAttachmentDevicePath(
	volumeInfo state.VolumeInfo,
	volumeAttachmentInfo state.VolumeAttachmentInfo,
	blockDevice state.BlockDeviceInfo,
) (string, error) {
	if volumeInfo.HardwareId != "" ||
		volumeInfo.WWN != "" ||
		volumeAttachmentInfo.DeviceName != "" ||
		volumeAttachmentInfo.DeviceLink != "" {
		// Prefer the volume attachment's information over what is
		// in the published block device information, but only if the
		// block device information actually has any device links. In
		// some cases, the block device has very little hw info published.
		var deviceLinks []string
		if volumeAttachmentInfo.DeviceLink != "" && len(blockDevice.DeviceLinks) > 0 {
			deviceLinks = []string{volumeAttachmentInfo.DeviceLink}
		}
		var deviceName string
		if blockDevice.DeviceName != "" {
			deviceName = blockDevice.DeviceName
		} else {
			deviceName = volumeAttachmentInfo.DeviceName
		}
		return storage.BlockDevicePath(storage.BlockDevice{
			HardwareId:  volumeInfo.HardwareId,
			WWN:         volumeInfo.WWN,
			UUID:        blockDevice.UUID,
			DeviceName:  deviceName,
			DeviceLinks: deviceLinks,
		})
	}
	return storage.BlockDevicePath(BlockDeviceFromState(blockDevice))
}

// Called by agent/provisioner and storageprovisioner.
// agent/provisioner so that params used to create a machine
// are augmented with the volumes to be attached at creation time.
// storageprovisioner to provide resource tags for new volumes.

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

// StorageTags returns the tags that should be set on a volume or filesystem,
// if the provider supports them.
func StorageTags(
	storageInstance state.StorageInstance,
	modelUUID, controllerUUID string,
	tagger tags.ResourceTagger,
) (map[string]string, error) {
	storageTags := tags.ResourceTags(
		names.NewModelTag(modelUUID),
		names.NewControllerTag(controllerUUID),
		tagger,
	)
	if storageInstance != nil {
		storageTags[tags.JujuStorageInstance] = storageInstance.Tag().Id()
		if owner, ok := storageInstance.Owner(); ok {
			storageTags[tags.JujuStorageOwner] = owner.Id()
		}
	}
	return storageTags, nil
}

// These methods are used by ModelManager and Application facades
// when destroying models and applications/units.

// UnitStorage returns the storage instances attached to the specified unit.
func UnitStorage(st StorageAccess, unit names.UnitTag) ([]state.StorageInstance, error) {
	attachments, err := st.UnitStorageAttachments(unit)
	if err != nil {
		return nil, errors.Trace(err)
	}
	instances := make([]state.StorageInstance, 0, len(attachments))
	for _, attachment := range attachments {
		instance, err := st.StorageInstance(attachment.StorageInstance())
		if errors.IsNotFound(err) {
			continue
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		instances = append(instances, instance)
	}
	return instances, nil
}

// ClassifyDetachedStorage classifies storage instances into those that will
// be destroyed, and those that will be detached, when their attachment is
// removed. Any storage that is not found will be omitted.
func ClassifyDetachedStorage(
	stVolume VolumeAccess,
	stFile FilesystemAccess,
	storage []state.StorageInstance,
) (destroyed, detached []params.Entity, _ error) {
	for _, storage := range storage {
		var detachable bool
		switch storage.Kind() {
		case state.StorageKindFilesystem:
			if stFile == nil {
				return nil, nil, errors.NotImplementedf("FilesystemStorage instance")
			}
			f, err := stFile.StorageInstanceFilesystem(storage.StorageTag())
			if errors.IsNotFound(err) {
				continue
			} else if err != nil {
				return nil, nil, err
			}
			detachable = f.Detachable()
		case state.StorageKindBlock:
			if stVolume == nil {
				return nil, nil, errors.NotImplementedf("BlockStorage instance")
			}
			v, err := stVolume.StorageInstanceVolume(storage.StorageTag())
			if errors.IsNotFound(err) {
				continue
			} else if err != nil {
				return nil, nil, err
			}
			detachable = v.Detachable()
		default:
			return nil, nil, errors.NotValidf("storage kind %s", storage.Kind())
		}
		entity := params.Entity{storage.StorageTag().String()}
		if detachable {
			detached = append(detached, entity)
		} else {
			destroyed = append(destroyed, entity)
		}
	}
	return destroyed, detached, nil
}
