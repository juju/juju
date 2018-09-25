// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storagecommon_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

type VolumeStorageAttachmentInfoSuite struct {
	machineTag           names.MachineTag
	volumeTag            names.VolumeTag
	storageTag           names.StorageTag
	st                   *fakeStorage
	storageInstance      *fakeStorageInstance
	storageAttachment    *fakeStorageAttachment
	volume               *fakeVolume
	volumeAttachment     *fakeVolumeAttachment
	volumeAttachmentPlan *fakeVolumeAttachmentPlan
	blockDevices         []state.BlockDeviceInfo
}

var _ = gc.Suite(&VolumeStorageAttachmentInfoSuite{})

func (s *VolumeStorageAttachmentInfoSuite) SetUpTest(c *gc.C) {
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
	s.blockDevices = []state.BlockDeviceInfo{{
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
		blockDevices: func(m names.MachineTag) ([]state.BlockDeviceInfo, error) {
			return s.blockDevices, nil
		},
		volumeAttachmentPlan: func(names.Tag, names.VolumeTag) (state.VolumeAttachmentPlan, error) {
			return s.volumeAttachmentPlan, nil
		},
	}
}

func (s *VolumeStorageAttachmentInfoSuite) TestStorageAttachmentPlanInfoDeviceNameSet(c *gc.C) {
	// Plan block info takes precedence to attachment info, planInfo is observed directly
	// on the machine itself, as opposed to volumeInfo which is "guessed" by the provider
	s.volumeAttachmentPlan.blockInfo.DeviceName = "sdb"
	s.volumeAttachmentPlan.err = nil
	s.volumeAttachment.info.DeviceName = "sda"
	info, err := storagecommon.StorageAttachmentInfo(s.st, s.st, s.st, s.storageAttachment, s.machineTag)
	c.Assert(err, jc.ErrorIsNil)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceVolume", "VolumeAttachment", "VolumeAttachmentPlan", "BlockDevices")
	c.Assert(info, jc.DeepEquals, &storage.StorageAttachmentInfo{
		Kind:     storage.StorageKindBlock,
		Location: "/dev/sdb",
	})
}

func (s *VolumeStorageAttachmentInfoSuite) TestStorageAttachmentInfoPersistentDeviceName(c *gc.C) {
	s.volumeAttachment.info.DeviceName = "sda"
	info, err := storagecommon.StorageAttachmentInfo(s.st, s.st, s.st, s.storageAttachment, s.machineTag)
	c.Assert(err, jc.ErrorIsNil)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceVolume", "VolumeAttachment", "VolumeAttachmentPlan", "BlockDevices")
	c.Assert(info, jc.DeepEquals, &storage.StorageAttachmentInfo{
		Kind:     storage.StorageKindBlock,
		Location: "/dev/sda",
	})
}

func (s *VolumeStorageAttachmentInfoSuite) TestStorageAttachmentInfoMissingBlockDevice(c *gc.C) {
	// If the block device has not shown up yet,
	// then we should get a NotProvisioned error.
	s.blockDevices = nil
	s.volumeAttachment.info.DeviceName = "sda"
	_, err := storagecommon.StorageAttachmentInfo(s.st, s.st, s.st, s.storageAttachment, s.machineTag)
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceVolume", "VolumeAttachment", "VolumeAttachmentPlan", "BlockDevices")
}

func (s *VolumeStorageAttachmentInfoSuite) TestStorageAttachmentInfoPersistentDeviceLink(c *gc.C) {
	s.volumeAttachment.info.DeviceLink = "/dev/disk/by-id/verbatim"
	info, err := storagecommon.StorageAttachmentInfo(s.st, s.st, s.st, s.storageAttachment, s.machineTag)
	c.Assert(err, jc.ErrorIsNil)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceVolume", "VolumeAttachment", "VolumeAttachmentPlan", "BlockDevices")
	c.Assert(info, jc.DeepEquals, &storage.StorageAttachmentInfo{
		Kind:     storage.StorageKindBlock,
		Location: "/dev/disk/by-id/verbatim",
	})
}

func (s *VolumeStorageAttachmentInfoSuite) TestStorageAttachmentInfoPersistentHardwareId(c *gc.C) {
	s.volume.info.HardwareId = "whatever"
	info, err := storagecommon.StorageAttachmentInfo(s.st, s.st, s.st, s.storageAttachment, s.machineTag)
	c.Assert(err, jc.ErrorIsNil)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceVolume", "VolumeAttachment", "VolumeAttachmentPlan", "BlockDevices")
	c.Assert(info, jc.DeepEquals, &storage.StorageAttachmentInfo{
		Kind:     storage.StorageKindBlock,
		Location: "/dev/disk/by-id/whatever",
	})
}

func (s *VolumeStorageAttachmentInfoSuite) TestStorageAttachmentInfoPersistentWWN(c *gc.C) {
	s.volume.info.WWN = "drbr"
	info, err := storagecommon.StorageAttachmentInfo(s.st, s.st, s.st, s.storageAttachment, s.machineTag)
	c.Assert(err, jc.ErrorIsNil)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceVolume", "VolumeAttachment", "VolumeAttachmentPlan", "BlockDevices")
	c.Assert(info, jc.DeepEquals, &storage.StorageAttachmentInfo{
		Kind:     storage.StorageKindBlock,
		Location: "/dev/disk/by-id/wwn-drbr",
	})
}

func (s *VolumeStorageAttachmentInfoSuite) TestStorageAttachmentInfoMatchingBlockDevice(c *gc.C) {
	// The bus address alone is not enough to produce a path to the block
	// device; we need to find a published block device with the matching
	// bus address.
	s.volumeAttachment.info.BusAddress = "scsi@1:2.3.4"
	s.blockDevices = []state.BlockDeviceInfo{{
		DeviceName: "sda",
	}, {
		DeviceName: "sdb",
		BusAddress: s.volumeAttachment.info.BusAddress,
	}}
	info, err := storagecommon.StorageAttachmentInfo(s.st, s.st, s.st, s.storageAttachment, s.machineTag)
	c.Assert(err, jc.ErrorIsNil)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceVolume", "VolumeAttachment", "VolumeAttachmentPlan", "BlockDevices")
	c.Assert(info, jc.DeepEquals, &storage.StorageAttachmentInfo{
		Kind:     storage.StorageKindBlock,
		Location: "/dev/sdb",
	})
}

func (s *VolumeStorageAttachmentInfoSuite) TestStorageAttachmentInfoNoBlockDevice(c *gc.C) {
	// Neither the volume nor the volume attachment has enough information
	// to persistently identify the path, so we must enquire about block
	// devices; there are none (yet), so NotProvisioned is returned.
	s.volumeAttachment.info.BusAddress = "scsi@1:2.3.4"
	_, err := storagecommon.StorageAttachmentInfo(s.st, s.st, s.st, s.storageAttachment, s.machineTag)
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceVolume", "VolumeAttachment", "VolumeAttachmentPlan", "BlockDevices")
}

func (s *VolumeStorageAttachmentInfoSuite) TestStorageAttachmentInfoVolumeNotFound(c *gc.C) {
	s.st.storageInstanceVolume = func(tag names.StorageTag) (state.Volume, error) {
		return nil, errors.NotFoundf("volume for storage %s", tag.Id())
	}
	_, err := storagecommon.StorageAttachmentInfo(s.st, s.st, s.st, s.storageAttachment, s.machineTag)
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
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

var _ = gc.Suite(&FilesystemStorageAttachmentInfoSuite{})

func (s *FilesystemStorageAttachmentInfoSuite) SetUpTest(c *gc.C) {
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

func (s *FilesystemStorageAttachmentInfoSuite) TestStorageAttachmentInfo(c *gc.C) {
	s.filesystemAttachment.info.MountPoint = "/path/to/here"
	info, err := storagecommon.StorageAttachmentInfo(s.st, s.st, s.st, s.storageAttachment, s.hostTag)
	c.Assert(err, jc.ErrorIsNil)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceFilesystem", "FilesystemAttachment")
	c.Assert(info, jc.DeepEquals, &storage.StorageAttachmentInfo{
		Kind:     storage.StorageKindFilesystem,
		Location: "/path/to/here",
	})
}

func (s *FilesystemStorageAttachmentInfoSuite) TestStorageAttachmentInfoFilesystemNotFound(c *gc.C) {
	s.st.storageInstanceFilesystem = func(tag names.StorageTag) (state.Filesystem, error) {
		return nil, errors.NotFoundf("filesystem for storage %s", tag.Id())
	}
	_, err := storagecommon.StorageAttachmentInfo(s.st, s.st, s.st, s.storageAttachment, s.hostTag)
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceFilesystem")
}
