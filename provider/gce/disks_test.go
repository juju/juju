// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/gce"
	"github.com/juju/juju/provider/gce/google"
	"github.com/juju/juju/storage"
)

type storageProviderSuite struct {
	gce.BaseSuite
	provider storage.Provider
}

var _ = gc.Suite(&storageProviderSuite{})

func (s *storageProviderSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.provider = gce.GCEStorageProvider()
}

func (s *storageProviderSuite) TestValidateConfig(c *gc.C) {
	// ValidateConfig performs no validation at all yet, this test
	// it is just here to make sure that the placeholder will
	// accept a config.
	cfg := &storage.Config{}
	err := s.provider.ValidateConfig(cfg)
	c.Check(err, jc.ErrorIsNil)
}

func (s *storageProviderSuite) TestBlockStorageSupport(c *gc.C) {
	supports := s.provider.Supports(storage.StorageKindBlock)
	c.Check(supports, jc.IsTrue)
}

func (s *storageProviderSuite) TestFSStorageSupport(c *gc.C) {
	supports := s.provider.Supports(storage.StorageKindFilesystem)
	c.Check(supports, jc.IsFalse)
}

func (s *storageProviderSuite) TestFSSource(c *gc.C) {
	eConfig := &config.Config{}
	sConfig := &storage.Config{}
	_, err := s.provider.FilesystemSource(eConfig, sConfig)
	c.Check(err, gc.ErrorMatches, "filesystems not supported")
}

func (s *storageProviderSuite) TestVolumeSource(c *gc.C) {
	connCfg := s.BaseSuite.Config
	storageCfg := &storage.Config{}
	_, err := s.provider.VolumeSource(connCfg, storageCfg)
	c.Check(err, jc.ErrorIsNil)
}

type volumeSourceSuite struct {
	gce.BaseSuite
	provider         storage.Provider
	source           storage.VolumeSource
	params           []storage.VolumeParams
	instId           instance.Id
	attachmentParams *storage.VolumeAttachmentParams
}

var _ = gc.Suite(&volumeSourceSuite{})

func (s *volumeSourceSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.provider = gce.GCEStorageProvider()
	var err error
	s.source, err = s.provider.VolumeSource(s.BaseSuite.Config, &storage.Config{})
	c.Check(err, jc.ErrorIsNil)

	inst := gce.NewInstance(s.BaseInstance, s.Env)
	vTag := names.NewVolumeTag("0")
	mTag := names.NewMachineTag("0")
	s.instId = inst.Id()
	s.attachmentParams = &storage.VolumeAttachmentParams{
		AttachmentParams: storage.AttachmentParams{
			Provider:   gce.GCEProviderType,
			Machine:    mTag,
			InstanceId: s.instId,
		},
		VolumeId: s.BaseDisk.Name,
		Volume:   names.NewVolumeTag("0"),
	}
	s.params = []storage.VolumeParams{{
		Tag:        vTag,
		Size:       1024,
		Provider:   gce.GCEProviderType,
		Attachment: s.attachmentParams,
	}}

}

func (s *volumeSourceSuite) TestCreateVolumesNoInstance(c *gc.C) {
	res, err := s.source.CreateVolumes(s.params)
	c.Check(err, jc.ErrorIsNil)
	c.Check(res, gc.HasLen, 1)
	expectedErr := "cannot obtain \"spam\" from instance cache: cannot attach to non-running instance spam"
	c.Assert(res[0].Error, gc.ErrorMatches, expectedErr)

}

func (s *volumeSourceSuite) TestCreateVolumesNoDiskCreated(c *gc.C) {
	s.FakeConn.Insts = []google.Instance{*s.BaseInstance}
	res, err := s.source.CreateVolumes(s.params)
	c.Check(err, jc.ErrorIsNil)
	c.Check(res, gc.HasLen, 1)
	c.Assert(res[0].Error, gc.ErrorMatches, "unexpected number of disks created: 0")

}

func (s *volumeSourceSuite) TestCreateVolumes(c *gc.C) {
	s.FakeConn.Insts = []google.Instance{*s.BaseInstance}
	s.FakeConn.GoogleDisks = []*google.Disk{s.BaseDisk}
	s.FakeConn.GoogleDisk = s.BaseDisk
	s.FakeConn.AttachedDisk = &google.AttachedDisk{
		VolumeName: s.BaseDisk.Name,
		DeviceName: "home-zone-1234567",
		Mode:       "READ_WRITE",
	}
	res, err := s.source.CreateVolumes(s.params)
	c.Check(err, jc.ErrorIsNil)
	c.Check(res, gc.HasLen, 1)
	// Volume was created
	c.Assert(res[0].Error, jc.ErrorIsNil)
	c.Assert(res[0].Volume.VolumeId, gc.Equals, s.BaseDisk.Name)

	// Volume was also attached as indicated by Attachment in params.
	c.Assert(res[0].VolumeAttachment.DeviceName, gc.Equals, "home-zone-1234567")
	c.Assert(res[0].VolumeAttachment.Machine.String(), gc.Equals, "machine-0")
	c.Assert(res[0].VolumeAttachment.ReadOnly, jc.IsFalse)
	c.Assert(res[0].VolumeAttachment.Volume.String(), gc.Equals, "volume-0")

	// Internals where properly called
	// Disk Creation
	createCalled, call := s.FakeConn.WasCalled("CreateDisks")
	c.Check(call, gc.HasLen, 1)
	c.Assert(createCalled, jc.IsTrue)
	c.Assert(call[0].ZoneName, gc.Equals, "home-zone")
	c.Assert(call[0].Disks[0].Name, jc.HasPrefix, "home-zone--")

	// Instance existence Checking
	instanceDisksCalled, call := s.FakeConn.WasCalled("InstanceDisks")
	c.Check(call, gc.HasLen, 1)
	c.Assert(instanceDisksCalled, jc.IsTrue)
	c.Assert(call[0].ZoneName, gc.Equals, "home-zone")
	c.Assert(call[0].InstanceId, gc.Equals, string(s.instId))

	// Disk Was attached
	attachCalled, call := s.FakeConn.WasCalled("AttachDisk")
	c.Check(call, gc.HasLen, 1)
	c.Assert(attachCalled, jc.IsTrue)
	c.Assert(call[0].ZoneName, gc.Equals, "home-zone")
	c.Assert(call[0].VolumeName, jc.HasPrefix, "home-zone--")
	c.Assert(call[0].InstanceId, gc.Equals, string(s.instId))
}

func (s *volumeSourceSuite) TestDestroyVolumes(c *gc.C) {
	errs, err := s.source.DestroyVolumes([]string{"a--volume-name"})
	c.Check(err, jc.ErrorIsNil)
	c.Check(errs, gc.HasLen, 1)
	c.Assert(errs[0], jc.ErrorIsNil)

	destroyCalled, call := s.FakeConn.WasCalled("RemoveDisk")
	c.Check(call, gc.HasLen, 1)
	c.Assert(destroyCalled, jc.IsTrue)
	c.Assert(call[0].ZoneName, gc.Equals, "a")
	c.Assert(call[0].ID, gc.Equals, "a--volume-name")
}

func (s *volumeSourceSuite) TestListVolumes(c *gc.C) {
	s.FakeConn.GoogleDisks = []*google.Disk{s.BaseDisk}
	s.FakeConn.Zones = []google.AvailabilityZone{google.NewZone("home-zone", "Ready", "", "")}
	vols, err := s.source.ListVolumes()
	c.Check(err, jc.ErrorIsNil)
	c.Assert(vols, gc.HasLen, 1)

	azsCalled, call := s.FakeConn.WasCalled("AvailabilityZones")
	c.Check(call, gc.HasLen, 1)
	c.Assert(azsCalled, jc.IsTrue)
	c.Assert(call[0].Region, gc.Equals, "")

	disksCalled, call := s.FakeConn.WasCalled("Disks")
	c.Check(call, gc.HasLen, 1)
	c.Assert(disksCalled, jc.IsTrue)
	c.Assert(call[0].ZoneName, gc.Equals, "home-zone")
}

func (s *volumeSourceSuite) TestDescribeVolumes(c *gc.C) {
	s.FakeConn.GoogleDisk = s.BaseDisk
	volName := "home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4"
	res, err := s.source.DescribeVolumes([]string{volName})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res[0].VolumeInfo.Size, gc.Equals, uint64(1024))
	c.Assert(res[0].VolumeInfo.VolumeId, gc.Equals, volName)

	diskCalled, call := s.FakeConn.WasCalled("Disk")
	c.Check(call, gc.HasLen, 1)
	c.Assert(diskCalled, jc.IsTrue)
	c.Assert(call[0].ZoneName, gc.Equals, "home-zone")
	c.Assert(call[0].ID, gc.Equals, volName)
}

func (s *volumeSourceSuite) TestAttachVolumes(c *gc.C) {
	volName := "home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4"
	attachments := []storage.VolumeAttachmentParams{*s.attachmentParams}
	s.FakeConn.AttachedDisk = &google.AttachedDisk{
		VolumeName: s.BaseDisk.Name,
		DeviceName: "home-zone-1234567",
		Mode:       "READ_WRITE",
	}
	res, err := s.source.AttachVolumes(attachments)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res[0].VolumeAttachment.Volume.String(), gc.Equals, "volume-0")
	c.Assert(res[0].VolumeAttachment.Machine.String(), gc.Equals, "machine-0")
	c.Assert(res[0].VolumeAttachment.VolumeAttachmentInfo.DeviceName, gc.Equals, "home-zone-1234567")

	// Disk Was attached
	attachCalled, call := s.FakeConn.WasCalled("AttachDisk")
	c.Check(call, gc.HasLen, 1)
	c.Assert(attachCalled, jc.IsTrue)
	c.Assert(call[0].ZoneName, gc.Equals, "home-zone")
	c.Assert(call[0].VolumeName, gc.Equals, volName)
	c.Assert(call[0].InstanceId, gc.Equals, string(s.instId))

}

func (s *volumeSourceSuite) TestDetachVolumes(c *gc.C) {
	volName := "home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4"
	attachments := []storage.VolumeAttachmentParams{*s.attachmentParams}
	errs, err := s.source.DetachVolumes(attachments)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(errs, gc.HasLen, 1)
	c.Assert(errs[0], jc.ErrorIsNil)

	// Disk Was detached
	attachCalled, call := s.FakeConn.WasCalled("DetachDisk")
	c.Check(call, gc.HasLen, 1)
	c.Assert(attachCalled, jc.IsTrue)
	c.Assert(call[0].ZoneName, gc.Equals, "home-zone")
	c.Assert(call[0].InstanceId, gc.Equals, string(s.instId))
	c.Assert(call[0].VolumeName, gc.Equals, volName)
}
