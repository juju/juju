// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storagecommon_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/state"
)

type VolumeStorageAttachmentInfoSuite struct {
	machineTag           names.MachineTag
	volumeTag            names.VolumeTag
	storageTag           names.StorageTag
	st                   *fakeStorage
	blockDeviceGetter    *fakeBlockDeviceGetter
	storageInstance      *fakeStorageInstance
	storageAttachment    *fakeStorageAttachment
	volume               *fakeVolume
	volumeAttachment     *fakeVolumeAttachment
	volumeAttachmentPlan *fakeVolumeAttachmentPlan
	blockDevices         []blockdevice.BlockDevice
}

var _ = tc.Suite(&VolumeStorageAttachmentInfoSuite{})

func (s *VolumeStorageAttachmentInfoSuite) SetUpTest(c *tc.C) {
	s.machineTag = names.NewMachineTag("0")
	s.volumeTag = names.NewVolumeTag("0")
	s.storageTag = names.NewStorageTag("osd-devices/0")
	s.storageInstance = &fakeStorageInstance{
		tag:   s.storageTag,
		owner: s.machineTag,
		kind:  state.StorageKindBlock,
	}
	s.storageAttachment = &fakeStorageAttachment{
		storageTag: s.storageTag,
	}
	s.volume = &fakeVolume{
		tag: s.volumeTag,
		info: &state.VolumeInfo{
			VolumeId: "vol-ume",
			Pool:     "radiance",
			Size:     1024,
		},
	}
	s.volumeAttachment = &fakeVolumeAttachment{
		info: &state.VolumeAttachmentInfo{},
	}
	s.volumeAttachmentPlan = &fakeVolumeAttachmentPlan{
		err:       errors.NotFoundf("volume attachment plans"),
		blockInfo: &state.BlockDeviceInfo{},
	}
	s.blockDevices = []blockdevice.BlockDevice{{
		DeviceName:  "sda",
		DeviceLinks: []string{"/dev/disk/by-id/verbatim"},
		HardwareId:  "whatever",
		WWN:         "drbr",
	}, {
		DeviceName:  "sdb",
		DeviceLinks: []string{"/dev/disk/by-id/a-second-device"},
		HardwareId:  "whatever",
		WWN:         "drbr",
	},
	}
	s.st = &fakeStorage{
		storageInstance: func(tag names.StorageTag) (state.StorageInstance, error) {
			return s.storageInstance, nil
		},
		storageInstanceVolume: func(tag names.StorageTag) (state.Volume, error) {
			return s.volume, nil
		},
		volumeAttachment: func(m names.Tag, v names.VolumeTag) (state.VolumeAttachment, error) {
			return s.volumeAttachment, nil
		},
		volumeAttachmentPlan: func(names.Tag, names.VolumeTag) (state.VolumeAttachmentPlan, error) {
			return s.volumeAttachmentPlan, nil
		},
	}
	s.blockDeviceGetter = &fakeBlockDeviceGetter{
		blockDevices: func(machineId string) ([]blockdevice.BlockDevice, error) {
			return s.blockDevices, nil
		},
	}
}

func (s *VolumeStorageAttachmentInfoSuite) TestStorageAttachmentPlanInfoDeviceNameSet(c *tc.C) {
	// Plan block info takes precedence to attachment info, planInfo is observed directly
	// on the machine itself, as opposed to volumeInfo which is "guessed" by the provider
	s.volumeAttachmentPlan.blockInfo.DeviceName = "sdb"
	s.volumeAttachmentPlan.err = nil
	s.blockDevices = []blockdevice.BlockDevice{{
		DeviceName: "sda",
	}, {
		DeviceName: "sdb",
	}}
	s.volumeAttachment.info.DeviceName = "sda"
	info, err := storagecommon.StorageAttachmentInfo(c.Context(), s.st, s.st, s.st, s.blockDeviceGetter, s.storageAttachment, s.machineTag)
	c.Assert(err, tc.ErrorIsNil)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceVolume", "VolumeAttachment", "VolumeAttachmentPlan")
	c.Assert(info, tc.DeepEquals, &storage.StorageAttachmentInfo{
		Kind:     storage.StorageKindBlock,
		Location: "/dev/sdb",
	})
	s.blockDeviceGetter.CheckCallNames(c, "BlockDevices")
}

func (s *VolumeStorageAttachmentInfoSuite) TestStorageAttachmentInfoPersistentDeviceName(c *tc.C) {
	s.volumeAttachment.info.DeviceName = "sda"
	info, err := storagecommon.StorageAttachmentInfo(c.Context(), s.st, s.st, s.st, s.blockDeviceGetter, s.storageAttachment, s.machineTag)
	c.Assert(err, tc.ErrorIsNil)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceVolume", "VolumeAttachment", "VolumeAttachmentPlan")
	c.Assert(info, tc.DeepEquals, &storage.StorageAttachmentInfo{
		Kind:     storage.StorageKindBlock,
		Location: "/dev/sda",
	})
	s.blockDeviceGetter.CheckCallNames(c, "BlockDevices")
}

func (s *VolumeStorageAttachmentInfoSuite) TestStorageAttachmentInfoMissingBlockDevice(c *tc.C) {
	// If the block device has not shown up yet,
	// then we should get a NotProvisioned error.
	s.blockDevices = nil
	s.volumeAttachment.info.DeviceName = "sda"
	_, err := storagecommon.StorageAttachmentInfo(c.Context(), s.st, s.st, s.st, s.blockDeviceGetter, s.storageAttachment, s.machineTag)
	c.Assert(err, tc.ErrorIs, errors.NotProvisioned)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceVolume", "VolumeAttachment", "VolumeAttachmentPlan")
	s.blockDeviceGetter.CheckCallNames(c, "BlockDevices")
}

func (s *VolumeStorageAttachmentInfoSuite) TestStorageAttachmentInfoPersistentDeviceNameIgnoresEmptyLinks(c *tc.C) {
	s.volumeAttachment.info.DeviceLink = "/dev/disk/by-id/verbatim"
	s.volumeAttachment.info.DeviceName = "sda"
	// Clear the machine block device link to force a match on name.
	s.blockDevices[0].DeviceLinks = nil
	info, err := storagecommon.StorageAttachmentInfo(c.Context(), s.st, s.st, s.st, s.blockDeviceGetter, s.storageAttachment, s.machineTag)
	c.Assert(err, tc.ErrorIsNil)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceVolume", "VolumeAttachment", "VolumeAttachmentPlan")
	c.Assert(info, tc.DeepEquals, &storage.StorageAttachmentInfo{
		Kind:     storage.StorageKindBlock,
		Location: "/dev/sda",
	})
	s.blockDeviceGetter.CheckCallNames(c, "BlockDevices")
}

func (s *VolumeStorageAttachmentInfoSuite) TestStorageAttachmentInfoPersistentDeviceLink(c *tc.C) {
	s.volumeAttachment.info.DeviceLink = "/dev/disk/by-id/verbatim"
	info, err := storagecommon.StorageAttachmentInfo(c.Context(), s.st, s.st, s.st, s.blockDeviceGetter, s.storageAttachment, s.machineTag)
	c.Assert(err, tc.ErrorIsNil)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceVolume", "VolumeAttachment", "VolumeAttachmentPlan")
	c.Assert(info, tc.DeepEquals, &storage.StorageAttachmentInfo{
		Kind:     storage.StorageKindBlock,
		Location: "/dev/disk/by-id/verbatim",
	})
	s.blockDeviceGetter.CheckCallNames(c, "BlockDevices")
}

func (s *VolumeStorageAttachmentInfoSuite) TestStorageAttachmentInfoPersistentHardwareId(c *tc.C) {
	s.volume.info.HardwareId = "whatever"
	info, err := storagecommon.StorageAttachmentInfo(c.Context(), s.st, s.st, s.st, s.blockDeviceGetter, s.storageAttachment, s.machineTag)
	c.Assert(err, tc.ErrorIsNil)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceVolume", "VolumeAttachment", "VolumeAttachmentPlan")
	c.Assert(info, tc.DeepEquals, &storage.StorageAttachmentInfo{
		Kind:     storage.StorageKindBlock,
		Location: "/dev/disk/by-id/whatever",
	})
	s.blockDeviceGetter.CheckCallNames(c, "BlockDevices")
}

func (s *VolumeStorageAttachmentInfoSuite) TestStorageAttachmentInfoPersistentWWN(c *tc.C) {
	s.volume.info.WWN = "drbr"
	info, err := storagecommon.StorageAttachmentInfo(c.Context(), s.st, s.st, s.st, s.blockDeviceGetter, s.storageAttachment, s.machineTag)
	c.Assert(err, tc.ErrorIsNil)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceVolume", "VolumeAttachment", "VolumeAttachmentPlan")
	c.Assert(info, tc.DeepEquals, &storage.StorageAttachmentInfo{
		Kind:     storage.StorageKindBlock,
		Location: "/dev/disk/by-id/wwn-drbr",
	})
	s.blockDeviceGetter.CheckCallNames(c, "BlockDevices")
}

func (s *VolumeStorageAttachmentInfoSuite) TestStorageAttachmentInfoMatchingBlockDevice(c *tc.C) {
	// The bus address alone is not enough to produce a path to the block
	// device; we need to find a published block device with the matching
	// bus address.
	s.volumeAttachment.info.BusAddress = "scsi@1:2.3.4"
	s.blockDevices = []blockdevice.BlockDevice{{
		DeviceName: "sda",
	}, {
		DeviceName: "sdb",
		BusAddress: s.volumeAttachment.info.BusAddress,
	}}
	info, err := storagecommon.StorageAttachmentInfo(c.Context(), s.st, s.st, s.st, s.blockDeviceGetter, s.storageAttachment, s.machineTag)
	c.Assert(err, tc.ErrorIsNil)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceVolume", "VolumeAttachment", "VolumeAttachmentPlan")
	c.Assert(info, tc.DeepEquals, &storage.StorageAttachmentInfo{
		Kind:     storage.StorageKindBlock,
		Location: "/dev/sdb",
	})
	s.blockDeviceGetter.CheckCallNames(c, "BlockDevices")
}

func (s *VolumeStorageAttachmentInfoSuite) TestStorageAttachmentInfoNoBlockDevice(c *tc.C) {
	// Neither the volume nor the volume attachment has enough information
	// to persistently identify the path, so we must enquire about block
	// devices; there are none (yet), so NotProvisioned is returned.
	s.volumeAttachment.info.BusAddress = "scsi@1:2.3.4"
	_, err := storagecommon.StorageAttachmentInfo(c.Context(), s.st, s.st, s.st, s.blockDeviceGetter, s.storageAttachment, s.machineTag)
	c.Assert(err, tc.ErrorIs, errors.NotProvisioned)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceVolume", "VolumeAttachment", "VolumeAttachmentPlan")
	s.blockDeviceGetter.CheckCallNames(c, "BlockDevices")
}

func (s *VolumeStorageAttachmentInfoSuite) TestStorageAttachmentInfoVolumeNotFound(c *tc.C) {
	s.st.storageInstanceVolume = func(tag names.StorageTag) (state.Volume, error) {
		return nil, errors.NotFoundf("volume for storage %s", tag.Id())
	}
	_, err := storagecommon.StorageAttachmentInfo(c.Context(), s.st, s.st, s.st, s.blockDeviceGetter, s.storageAttachment, s.machineTag)
	c.Assert(err, tc.ErrorIs, errors.NotProvisioned)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceVolume")
}

type FilesystemStorageAttachmentInfoSuite struct {
	hostTag              names.Tag
	filsystemTag         names.FilesystemTag
	storageTag           names.StorageTag
	st                   *fakeStorage
	storageInstance      *fakeStorageInstance
	storageAttachment    *fakeStorageAttachment
	filesystem           *fakeFilesystem
	filesystemAttachment *fakeFilesystemAttachment
}

var _ = tc.Suite(&FilesystemStorageAttachmentInfoSuite{})

func (s *FilesystemStorageAttachmentInfoSuite) SetUpTest(c *tc.C) {
	s.hostTag = names.NewUnitTag("mysql/0")
	s.filsystemTag = names.NewFilesystemTag("0")
	s.storageTag = names.NewStorageTag("data/0")
	s.storageInstance = &fakeStorageInstance{
		tag:   s.storageTag,
		owner: s.hostTag,
		kind:  state.StorageKindFilesystem,
	}
	s.storageAttachment = &fakeStorageAttachment{
		storageTag: s.storageTag,
	}
	s.filesystem = &fakeFilesystem{
		tag: s.filsystemTag,
		info: &state.FilesystemInfo{
			FilesystemId: "file-system",
			Pool:         "radiance",
			Size:         1024,
		},
	}
	s.filesystemAttachment = &fakeFilesystemAttachment{
		info: &state.FilesystemAttachmentInfo{},
	}
	s.st = &fakeStorage{
		storageInstance: func(tag names.StorageTag) (state.StorageInstance, error) {
			return s.storageInstance, nil
		},
		storageInstanceFilesystem: func(tag names.StorageTag) (state.Filesystem, error) {
			return s.filesystem, nil
		},
		filesystemAttachment: func(m names.Tag, fs names.FilesystemTag) (state.FilesystemAttachment, error) {
			return s.filesystemAttachment, nil
		},
	}
}

func (s *FilesystemStorageAttachmentInfoSuite) TestStorageAttachmentInfo(c *tc.C) {
	s.filesystemAttachment.info.MountPoint = "/path/to/here"
	info, err := storagecommon.StorageAttachmentInfo(c.Context(), s.st, s.st, s.st, &fakeBlockDeviceGetter{}, s.storageAttachment, s.hostTag)
	c.Assert(err, tc.ErrorIsNil)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceFilesystem", "FilesystemAttachment")
	c.Assert(info, tc.DeepEquals, &storage.StorageAttachmentInfo{
		Kind:     storage.StorageKindFilesystem,
		Location: "/path/to/here",
	})
}

func (s *FilesystemStorageAttachmentInfoSuite) TestStorageAttachmentInfoFilesystemNotFound(c *tc.C) {
	s.st.storageInstanceFilesystem = func(tag names.StorageTag) (state.Filesystem, error) {
		return nil, errors.NotFoundf("filesystem for storage %s", tag.Id())
	}
	_, err := storagecommon.StorageAttachmentInfo(c.Context(), s.st, s.st, s.st, &fakeBlockDeviceGetter{}, s.storageAttachment, s.hostTag)
	c.Assert(err, tc.ErrorIs, errors.NotProvisioned)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceFilesystem")
}
