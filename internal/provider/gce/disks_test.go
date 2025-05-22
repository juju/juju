// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/internal/provider/gce"
	"github.com/juju/juju/internal/provider/gce/google"
	"github.com/juju/juju/internal/storage"
)

type storageProviderSuite struct {
	gce.BaseSuite
	provider storage.Provider
}

func TestStorageProviderSuite(t *stdtesting.T) {
	tc.Run(t, &storageProviderSuite{})
}

func (s *storageProviderSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	var err error
	s.provider, err = s.Env.StorageProvider("gce")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageProviderSuite) TestValidateConfig(c *tc.C) {
	// ValidateConfig performs no validation at all yet, this test
	// it is just here to make sure that the placeholder will
	// accept a config.
	cfg := &storage.Config{}
	err := s.provider.ValidateConfig(cfg)
	c.Check(err, tc.ErrorIsNil)
}

func (s *storageProviderSuite) TestBlockStorageSupport(c *tc.C) {
	supports := s.provider.Supports(storage.StorageKindBlock)
	c.Check(supports, tc.IsTrue)
}

func (s *storageProviderSuite) TestFSStorageSupport(c *tc.C) {
	supports := s.provider.Supports(storage.StorageKindFilesystem)
	c.Check(supports, tc.IsFalse)
}

func (s *storageProviderSuite) TestFSSource(c *tc.C) {
	sConfig := &storage.Config{}
	_, err := s.provider.FilesystemSource(sConfig)
	c.Check(err, tc.ErrorMatches, "filesystems not supported")
}

func (s *storageProviderSuite) TestVolumeSource(c *tc.C) {
	storageCfg := &storage.Config{}
	_, err := s.provider.VolumeSource(storageCfg)
	c.Check(err, tc.ErrorIsNil)
}

type volumeSourceSuite struct {
	gce.BaseSuite
	source           storage.VolumeSource
	params           []storage.VolumeParams
	instId           instance.Id
	attachmentParams *storage.VolumeAttachmentParams
}

func TestVolumeSourceSuite(t *stdtesting.T) {
	tc.Run(t, &volumeSourceSuite{})
}

func (s *volumeSourceSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	provider, err := s.Env.StorageProvider("gce")
	c.Assert(err, tc.ErrorIsNil)
	s.source, err = provider.VolumeSource(&storage.Config{})
	c.Check(err, tc.ErrorIsNil)

	inst := gce.NewInstance(s.BaseInstance, s.Env)
	vTag := names.NewVolumeTag("0")
	mTag := names.NewMachineTag("0")
	s.instId = inst.Id()
	s.attachmentParams = &storage.VolumeAttachmentParams{
		AttachmentParams: storage.AttachmentParams{
			Provider:   "gce",
			Machine:    mTag,
			InstanceId: s.instId,
		},
		VolumeId: s.BaseDisk.Name,
		Volume:   names.NewVolumeTag("0"),
	}
	s.params = []storage.VolumeParams{{
		Tag:        vTag,
		Size:       1024,
		Provider:   "gce",
		Attachment: s.attachmentParams,
	}}
}

func (s *volumeSourceSuite) TestCreateVolumesNoInstance(c *tc.C) {
	res, err := s.source.CreateVolumes(c.Context(), s.params)
	c.Check(err, tc.ErrorIsNil)
	c.Check(res, tc.HasLen, 1)
	expectedErr := "cannot obtain \"spam\" from instance cache: cannot attach to non-running instance spam"
	c.Assert(res[0].Error, tc.ErrorMatches, expectedErr)

}

func (s *volumeSourceSuite) TestCreateVolumesNoDiskCreated(c *tc.C) {
	s.FakeConn.Insts = []google.Instance{*s.BaseInstance}
	res, err := s.source.CreateVolumes(c.Context(), s.params)
	c.Check(err, tc.ErrorIsNil)
	c.Check(res, tc.HasLen, 1)
	c.Assert(res[0].Error, tc.ErrorMatches, "unexpected number of disks created: 0")

}

func (s *volumeSourceSuite) TestCreateVolumesInvalidCredentialError(c *tc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)
	_, err := s.source.CreateVolumes(c.Context(), s.params)
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}

func (s *volumeSourceSuite) TestCreateVolumes(c *tc.C) {
	s.FakeConn.Insts = []google.Instance{*s.BaseInstance}
	s.FakeConn.GoogleDisks = []*google.Disk{s.BaseDisk}
	s.FakeConn.GoogleDisk = s.BaseDisk
	s.FakeConn.AttachedDisk = &google.AttachedDisk{
		VolumeName: s.BaseDisk.Name,
		DeviceName: "home-zone-1234567",
		Mode:       "READ_WRITE",
	}
	res, err := s.source.CreateVolumes(c.Context(), s.params)
	c.Check(err, tc.ErrorIsNil)
	c.Check(res, tc.HasLen, 1)
	// Volume was created
	c.Assert(res[0].Error, tc.ErrorIsNil)
	c.Assert(res[0].Volume.VolumeId, tc.Equals, s.BaseDisk.Name)
	c.Assert(res[0].Volume.HardwareId, tc.Equals, "")

	// Volume was also attached as indicated by Attachment in params.
	c.Assert(res[0].VolumeAttachment.DeviceName, tc.Equals, "")
	c.Assert(res[0].VolumeAttachment.DeviceLink, tc.Equals, "/dev/disk/by-id/google-home-zone-1234567")
	c.Assert(res[0].VolumeAttachment.Machine.String(), tc.Equals, "machine-0")
	c.Assert(res[0].VolumeAttachment.ReadOnly, tc.IsFalse)
	c.Assert(res[0].VolumeAttachment.Volume.String(), tc.Equals, "volume-0")

	// Internals where properly called
	// Disk Creation
	createCalled, call := s.FakeConn.WasCalled("CreateDisks")
	c.Check(call, tc.HasLen, 1)
	c.Assert(createCalled, tc.IsTrue)
	c.Assert(call[0].ZoneName, tc.Equals, "home-zone")
	c.Assert(call[0].Disks[0].Name, tc.HasPrefix, "home-zone--")

	// Instance existence Checking
	instanceDisksCalled, call := s.FakeConn.WasCalled("InstanceDisks")
	c.Check(call, tc.HasLen, 1)
	c.Assert(instanceDisksCalled, tc.IsTrue)
	c.Assert(call[0].ZoneName, tc.Equals, "home-zone")
	c.Assert(call[0].InstanceId, tc.Equals, string(s.instId))

	// Disk Was attached
	attachCalled, call := s.FakeConn.WasCalled("AttachDisk")
	c.Check(call, tc.HasLen, 1)
	c.Assert(attachCalled, tc.IsTrue)
	c.Assert(call[0].ZoneName, tc.Equals, "home-zone")
	c.Assert(call[0].VolumeName, tc.HasPrefix, "home-zone--")
	c.Assert(call[0].InstanceId, tc.Equals, string(s.instId))
}

func (s *volumeSourceSuite) TestDestroyVolumesInvalidCredentialError(c *tc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)
	_, err := s.source.DestroyVolumes(c.Context(), []string{"a--volume-name"})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}

func (s *volumeSourceSuite) TestDestroyVolumes(c *tc.C) {
	errs, err := s.source.DestroyVolumes(c.Context(), []string{"a--volume-name"})
	c.Check(err, tc.ErrorIsNil)
	c.Check(errs, tc.HasLen, 1)
	c.Assert(errs[0], tc.ErrorIsNil)

	destroyCalled, call := s.FakeConn.WasCalled("RemoveDisk")
	c.Check(call, tc.HasLen, 1)
	c.Assert(destroyCalled, tc.IsTrue)
	c.Assert(call[0].ZoneName, tc.Equals, "a")
	c.Assert(call[0].ID, tc.Equals, "a--volume-name")
}

func (s *volumeSourceSuite) TestReleaseVolumesInvalidCredentialError(c *tc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)
	_, err := s.source.ReleaseVolumes(c.Context(), []string{s.BaseDisk.Name})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}

func (s *volumeSourceSuite) TestReleaseVolumes(c *tc.C) {
	s.FakeConn.GoogleDisk = s.BaseDisk

	errs, err := s.source.ReleaseVolumes(c.Context(), []string{s.BaseDisk.Name})
	c.Check(err, tc.ErrorIsNil)
	c.Check(errs, tc.HasLen, 1)
	c.Assert(errs[0], tc.ErrorIsNil)

	called, calls := s.FakeConn.WasCalled("SetDiskLabels")
	c.Check(called, tc.IsTrue)
	c.Assert(calls, tc.HasLen, 1)
	c.Assert(calls[0].ZoneName, tc.Equals, "home-zone")
	c.Assert(calls[0].ID, tc.Equals, s.BaseDisk.Name)
	c.Assert(calls[0].Labels, tc.DeepEquals, map[string]string{
		"yodel": "eh",
		// Note, no controller/model labels
	})
}

func (s *volumeSourceSuite) TestImportVolumesInvalidCredentialError(c *tc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)
	_, err := s.source.(storage.VolumeImporter).ImportVolume(
		c.Context(),
		s.BaseDisk.Name, map[string]string{
			"juju-model-uuid":      "foo",
			"juju-controller-uuid": "bar",
		},
	)
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}

func (s *volumeSourceSuite) TestImportVolume(c *tc.C) {
	s.FakeConn.GoogleDisk = s.BaseDisk

	c.Assert(s.source, tc.Implements, new(storage.VolumeImporter))
	volumeInfo, err := s.source.(storage.VolumeImporter).ImportVolume(
		c.Context(),
		s.BaseDisk.Name, map[string]string{
			"juju-model-uuid":      "foo",
			"juju-controller-uuid": "bar",
		},
	)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(volumeInfo, tc.DeepEquals, storage.VolumeInfo{
		VolumeId:   s.BaseDisk.Name,
		Size:       1024,
		Persistent: true,
	})

	called, calls := s.FakeConn.WasCalled("SetDiskLabels")
	c.Check(called, tc.IsTrue)
	c.Assert(calls, tc.HasLen, 1)
	c.Assert(calls[0].ZoneName, tc.Equals, "home-zone")
	c.Assert(calls[0].ID, tc.Equals, s.BaseDisk.Name)
	c.Assert(calls[0].Labels, tc.DeepEquals, map[string]string{
		"juju-model-uuid":      "foo",
		"juju-controller-uuid": "bar",
		"yodel":                "eh", // other existing tags left alone
	})
}

func (s *volumeSourceSuite) TestImportVolumeNotReady(c *tc.C) {
	s.FakeConn.GoogleDisk = s.BaseDisk
	s.FakeConn.GoogleDisk.Status = "floop"

	_, err := s.source.(storage.VolumeImporter).ImportVolume(
		c.Context(),
		s.BaseDisk.Name, map[string]string{},
	)
	c.Check(err, tc.ErrorMatches, `cannot import volume "`+s.BaseDisk.Name+`" with status "floop"`)

	called, _ := s.FakeConn.WasCalled("SetDiskLabels")
	c.Check(called, tc.IsFalse)
}

func (s *volumeSourceSuite) TestListVolumesInvalidCredentialError(c *tc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)
	_, err := s.source.ListVolumes(c.Context())
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}

func (s *volumeSourceSuite) TestListVolumes(c *tc.C) {
	s.FakeConn.GoogleDisks = []*google.Disk{s.BaseDisk}
	vols, err := s.source.ListVolumes(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Assert(vols, tc.HasLen, 1)

	disksCalled, call := s.FakeConn.WasCalled("Disks")
	c.Check(call, tc.HasLen, 1)
	c.Assert(disksCalled, tc.IsTrue)
}

func (s *volumeSourceSuite) TestListVolumesOnlyListsCurrentModelUUID(c *tc.C) {
	otherDisk := &google.Disk{
		Id:          1234568,
		Name:        "home-zone--566fe7b2-c026-4a86-a2cc-84cb7f9a4868",
		Zone:        "home-zone",
		Status:      google.StatusReady,
		Size:        1024,
		Description: "a-different-model-uuid",
		Labels: map[string]string{
			"juju-model-uuid": "foo",
		},
	}
	s.FakeConn.GoogleDisks = []*google.Disk{s.BaseDisk, otherDisk}
	vols, err := s.source.ListVolumes(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Assert(vols, tc.HasLen, 1)
}

func (s *volumeSourceSuite) TestListVolumesIgnoresNamesFormatteDifferently(c *tc.C) {
	otherDisk := &google.Disk{
		Id:          1234568,
		Name:        "juju-566fe7b2-c026-4a86-a2cc-84cb7f9a4868",
		Zone:        "home-zone",
		Status:      google.StatusReady,
		Size:        1024,
		Description: "",
	}
	s.FakeConn.GoogleDisks = []*google.Disk{s.BaseDisk, otherDisk}
	vols, err := s.source.ListVolumes(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Assert(vols, tc.HasLen, 1)
}

func (s *volumeSourceSuite) TestDescribeVolumesInvalidCredentialError(c *tc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)
	volName := "home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4"
	_, err := s.source.DescribeVolumes(c.Context(), []string{volName})
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}

func (s *volumeSourceSuite) TestDescribeVolumes(c *tc.C) {
	s.FakeConn.GoogleDisk = s.BaseDisk
	volName := "home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4"
	res, err := s.source.DescribeVolumes(c.Context(), []string{volName})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(res, tc.HasLen, 1)
	c.Assert(res[0].VolumeInfo.Size, tc.Equals, uint64(1024))
	c.Assert(res[0].VolumeInfo.VolumeId, tc.Equals, volName)

	diskCalled, call := s.FakeConn.WasCalled("Disk")
	c.Check(call, tc.HasLen, 1)
	c.Assert(diskCalled, tc.IsTrue)
	c.Assert(call[0].ZoneName, tc.Equals, "home-zone")
	c.Assert(call[0].ID, tc.Equals, volName)
}

func (s *volumeSourceSuite) TestAttachVolumes(c *tc.C) {
	volName := "home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4"
	attachments := []storage.VolumeAttachmentParams{*s.attachmentParams}
	s.FakeConn.AttachedDisk = &google.AttachedDisk{
		VolumeName: s.BaseDisk.Name,
		DeviceName: "home-zone-1234567",
		Mode:       "READ_WRITE",
	}
	res, err := s.source.AttachVolumes(c.Context(), attachments)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(res, tc.HasLen, 1)
	c.Assert(res[0].VolumeAttachment.Volume.String(), tc.Equals, "volume-0")
	c.Assert(res[0].VolumeAttachment.Machine.String(), tc.Equals, "machine-0")
	c.Assert(res[0].VolumeAttachment.VolumeAttachmentInfo.DeviceName, tc.Equals, "")
	c.Assert(res[0].VolumeAttachment.VolumeAttachmentInfo.DeviceLink, tc.Equals, "/dev/disk/by-id/google-home-zone-1234567")

	// Disk Was attached
	attachCalled, call := s.FakeConn.WasCalled("AttachDisk")
	c.Check(call, tc.HasLen, 1)
	c.Assert(attachCalled, tc.IsTrue)
	c.Assert(call[0].ZoneName, tc.Equals, "home-zone")
	c.Assert(call[0].VolumeName, tc.Equals, volName)
	c.Assert(call[0].InstanceId, tc.Equals, string(s.instId))

}

func (s *volumeSourceSuite) TestAttachVolumesInvalidCredentialError(c *tc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)
	_, err := s.source.AttachVolumes(c.Context(), []storage.VolumeAttachmentParams{*s.attachmentParams})
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}

func (s *volumeSourceSuite) TestDetachVolumes(c *tc.C) {
	volName := "home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4"
	attachments := []storage.VolumeAttachmentParams{*s.attachmentParams}
	errs, err := s.source.DetachVolumes(c.Context(), attachments)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(errs, tc.HasLen, 1)
	c.Assert(errs[0], tc.ErrorIsNil)

	// Disk Was detached
	attachCalled, call := s.FakeConn.WasCalled("DetachDisk")
	c.Check(call, tc.HasLen, 1)
	c.Assert(attachCalled, tc.IsTrue)
	c.Assert(call[0].ZoneName, tc.Equals, "home-zone")
	c.Assert(call[0].InstanceId, tc.Equals, string(s.instId))
	c.Assert(call[0].VolumeName, tc.Equals, volName)
}

func (s *volumeSourceSuite) TestDetachVolumesInvalidCredentialError(c *tc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)
	_, err := s.source.DetachVolumes(c.Context(), []storage.VolumeAttachmentParams{*s.attachmentParams})
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}
