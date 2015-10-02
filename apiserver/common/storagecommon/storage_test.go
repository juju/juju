// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storagecommon_test

import (
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

type storageAttachmentInfoSuite struct {
	testing.Stub
	machineTag        names.MachineTag
	volumeTag         names.VolumeTag
	storageTag        names.StorageTag
	st                *fakeStorage
	storageInstance   *fakeStorageInstance
	storageAttachment *fakeStorageAttachment
	volume            *fakeVolume
	volumeAttachment  *fakeVolumeAttachment
	blockDevices      []state.BlockDeviceInfo
}

var _ = gc.Suite(&storageAttachmentInfoSuite{})

func (s *storageAttachmentInfoSuite) SetUpTest(c *gc.C) {
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
	s.blockDevices = nil
	s.st = &fakeStorage{
		storageInstance: func(tag names.StorageTag) (state.StorageInstance, error) {
			return s.storageInstance, nil
		},
		storageInstanceVolume: func(tag names.StorageTag) (state.Volume, error) {
			return s.volume, nil
		},
		volumeAttachment: func(m names.MachineTag, v names.VolumeTag) (state.VolumeAttachment, error) {
			return s.volumeAttachment, nil
		},
		blockDevices: func(m names.MachineTag) ([]state.BlockDeviceInfo, error) {
			return s.blockDevices, nil
		},
	}
}

func (s *storageAttachmentInfoSuite) TestStorageAttachmentInfoPersistentDeviceName(c *gc.C) {
	s.volumeAttachment.info.DeviceName = "sda"
	info, err := storagecommon.StorageAttachmentInfo(s.st, s.storageAttachment, s.machineTag)
	c.Assert(err, jc.ErrorIsNil)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceVolume", "VolumeAttachment")
	c.Assert(info, jc.DeepEquals, &storage.StorageAttachmentInfo{
		Kind:     storage.StorageKindBlock,
		Location: filepath.FromSlash("/dev/sda"),
	})
}

func (s *storageAttachmentInfoSuite) TestStorageAttachmentInfoPersistentDeviceLink(c *gc.C) {
	s.volumeAttachment.info.DeviceLink = "/dev/disk/by-id/verbatim"
	info, err := storagecommon.StorageAttachmentInfo(s.st, s.storageAttachment, s.machineTag)
	c.Assert(err, jc.ErrorIsNil)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceVolume", "VolumeAttachment")
	c.Assert(info, jc.DeepEquals, &storage.StorageAttachmentInfo{
		Kind:     storage.StorageKindBlock,
		Location: "/dev/disk/by-id/verbatim",
	})
}

func (s *storageAttachmentInfoSuite) TestStorageAttachmentInfoPersistentHardwareId(c *gc.C) {
	s.volume.info.HardwareId = "whatever"
	info, err := storagecommon.StorageAttachmentInfo(s.st, s.storageAttachment, s.machineTag)
	c.Assert(err, jc.ErrorIsNil)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceVolume", "VolumeAttachment")
	c.Assert(info, jc.DeepEquals, &storage.StorageAttachmentInfo{
		Kind:     storage.StorageKindBlock,
		Location: filepath.FromSlash("/dev/disk/by-id/whatever"),
	})
}

func (s *storageAttachmentInfoSuite) TestStorageAttachmentInfoMatchingBlockDevice(c *gc.C) {
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
	info, err := storagecommon.StorageAttachmentInfo(s.st, s.storageAttachment, s.machineTag)
	c.Assert(err, jc.ErrorIsNil)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceVolume", "VolumeAttachment", "BlockDevices")
	c.Assert(info, jc.DeepEquals, &storage.StorageAttachmentInfo{
		Kind:     storage.StorageKindBlock,
		Location: filepath.FromSlash("/dev/sdb"),
	})
}

func (s *storageAttachmentInfoSuite) TestStorageAttachmentInfoNoBlockDevice(c *gc.C) {
	// Neither the volume nor the volume attachment has enough information
	// to persistently identify the path, so we must enquire about block
	// devices; there are none (yet), so NotProvisioned is returned.
	s.volumeAttachment.info.BusAddress = "scsi@1:2.3.4"
	_, err := storagecommon.StorageAttachmentInfo(s.st, s.storageAttachment, s.machineTag)
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
	s.st.CheckCallNames(c, "StorageInstance", "StorageInstanceVolume", "VolumeAttachment", "BlockDevices")
}
