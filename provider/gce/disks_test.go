// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"context"
	"strings"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"go.uber.org/mock/gomock"
	"google.golang.org/api/compute/v1"
	gc "gopkg.in/check.v1"

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
	env := s.SetupEnv(c, nil)
	var err error
	s.provider, err = env.StorageProvider("gce")
	c.Assert(err, gc.IsNil)
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
	sConfig := &storage.Config{}
	_, err := s.provider.FilesystemSource(sConfig)
	c.Check(err, gc.ErrorMatches, "filesystems not supported")
}

func (s *storageProviderSuite) TestVolumeSource(c *gc.C) {
	storageCfg := &storage.Config{}
	_, err := s.provider.VolumeSource(storageCfg)
	c.Check(err, jc.ErrorIsNil)
}

type volumeSourceSuite struct {
	gce.BaseSuite
	params           []storage.VolumeParams
	attachmentParams *storage.VolumeAttachmentParams
}

var _ = gc.Suite(&volumeSourceSuite{})

func (s *volumeSourceSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.attachmentParams = &storage.VolumeAttachmentParams{
		AttachmentParams: storage.AttachmentParams{
			Provider:   "gce",
			Machine:    names.NewMachineTag("0"),
			InstanceId: "inst-0",
		},
		VolumeId: "zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
		Volume:   names.NewVolumeTag("0"),
	}
	s.params = []storage.VolumeParams{{
		Tag:        names.NewVolumeTag("0"),
		Size:       1024,
		Provider:   "gce",
		Attachment: s.attachmentParams,
	}}
}

func (s *volumeSourceSuite) setUpSource(c *gc.C) storage.VolumeSource {
	env := s.SetupEnv(c, s.MockService)
	provider, err := env.StorageProvider("gce")
	c.Assert(err, jc.ErrorIsNil)
	source, err := provider.VolumeSource(&storage.Config{})
	c.Check(err, jc.ErrorIsNil)
	return source
}

func (s *volumeSourceSuite) TestCreateVolumesNoInstance(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().Instances(gomock.Any(), "", google.StatusRunning).Return(nil, nil)

	source := s.setUpSource(c)
	res, err := source.CreateVolumes(s.CallCtx, s.params)
	c.Check(err, jc.ErrorIsNil)
	c.Check(res, gc.HasLen, 1)
	expectedErr := "cannot obtain \"inst-0\" from instance cache: cannot attach to non-running instance inst-0"
	c.Assert(res[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *volumeSourceSuite) TestCreateVolumesInvalidCredentialError(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().Instances(gomock.Any(), "", google.StatusRunning).Return([]*compute.Instance{{
		Name: "inst-0",
		Zone: "path/to/zone",
	}}, nil)

	c.Assert(s.InvalidatedCredentials, jc.IsFalse)
	expected := &compute.Disk{
		Name:   "zone",
		SizeGb: 1024,
		Type:   "pd-standard",
		Labels: map[string]string{},
	}
	s.MockService.EXPECT().CreateDisks(gomock.Any(), "zone", gomock.Any()).
		DoAndReturn(func(ctx context.Context, zone string, disks []*compute.Disk) error {
			c.Assert(disks, gc.HasLen, 1)
			if !strings.HasPrefix(disks[0].Name, "zone--") {
				c.Fail()
			}
			expected.Name = disks[0].Name
			c.Assert(disks[0], jc.DeepEquals, expected)
			return gce.InvalidCredentialError
		})

	s.MockService.EXPECT().RemoveDisk(gomock.Any(), "zone", gomock.Any()).
		DoAndReturn(func(ctx context.Context, zone, volName string) error {
			if !strings.HasPrefix(volName, zone+"--") {
				c.Fail()
			}
			return nil
		})

	source := s.setUpSource(c)
	_, err := source.CreateVolumes(s.CallCtx, s.params)
	c.Check(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *volumeSourceSuite) TestCreateVolumes(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().Instances(gomock.Any(), "", google.StatusRunning).Return([]*compute.Instance{{
		Name: "inst-0",
		Zone: "path/to/zone",
	}}, nil)

	expected := &compute.Disk{
		Name:   "zone",
		SizeGb: 1024,
		Type:   "pd-standard",
		Labels: map[string]string{},
	}
	s.MockService.EXPECT().CreateDisks(gomock.Any(), "zone", gomock.Any()).
		DoAndReturn(func(ctx context.Context, zone string, disks []*compute.Disk) error {
			c.Assert(disks, gc.HasLen, 1)
			if !strings.HasPrefix(disks[0].Name, "zone--") {
				c.Fail()
			}
			expected.Name = disks[0].Name
			c.Assert(disks[0], jc.DeepEquals, expected)
			return nil
		})
	s.MockService.EXPECT().InstanceDisks(gomock.Any(), "zone", "inst-0").Return([]*compute.AttachedDisk{{
		Source: "not-already-attached",
	}}, nil)
	var attachedVol string
	s.MockService.EXPECT().AttachDisk(gomock.Any(), "zone", gomock.Any(), "inst-0", google.ModeRW).
		DoAndReturn(func(ctx context.Context, zone, volName, instanceId string, mode google.DiskMode) (*compute.AttachedDisk, error) {
			if !strings.HasPrefix(volName, zone+"--") {
				c.Fail()
			}
			attachedVol = volName
			return &compute.AttachedDisk{
				DeviceName: "zone-1234567",
			}, nil
		})

	source := s.setUpSource(c)
	res, err := source.CreateVolumes(s.CallCtx, s.params)
	c.Check(err, jc.ErrorIsNil)
	c.Check(res, gc.HasLen, 1)
	// Volume was created
	c.Assert(res[0].Error, jc.ErrorIsNil)
	c.Assert(res[0].Volume.VolumeId, gc.Equals, attachedVol)
	c.Assert(res[0].Volume.HardwareId, gc.Equals, "")

	// Volume was also attached as indicated by Attachment in params.
	c.Assert(res[0].VolumeAttachment.DeviceName, gc.Equals, "")
	c.Assert(res[0].VolumeAttachment.DeviceLink, gc.Equals, "/dev/disk/by-id/google-zone-1234567")
	c.Assert(res[0].VolumeAttachment.Machine.String(), gc.Equals, "machine-0")
	c.Assert(res[0].VolumeAttachment.ReadOnly, jc.IsFalse)
	c.Assert(res[0].VolumeAttachment.Volume.String(), gc.Equals, "volume-0")
}

func (s *volumeSourceSuite) TestDestroyVolumesInvalidCredentialError(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().RemoveDisk(gomock.Any(), "zone", "zone--volume-name").Return(gce.InvalidCredentialError)

	source := s.setUpSource(c)
	_, err := source.DestroyVolumes(s.CallCtx, []string{"zone--volume-name"})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *volumeSourceSuite) TestDestroyVolumes(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().RemoveDisk(gomock.Any(), "zone", "zone--volume-name")

	source := s.setUpSource(c)
	errs, err := source.DestroyVolumes(s.CallCtx, []string{"zone--volume-name"})
	c.Check(err, jc.ErrorIsNil)
	c.Check(errs, gc.HasLen, 1)
	c.Assert(errs[0], jc.ErrorIsNil)
}

func (s *volumeSourceSuite) TestReleaseVolumesInvalidCredentialError(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().Disk(gomock.Any(), "zone", "zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4").
		Return(nil, gce.InvalidCredentialError)
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)

	source := s.setUpSource(c)
	_, err := source.ReleaseVolumes(s.CallCtx, []string{"zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4"})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *volumeSourceSuite) TestReleaseVolumes(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().Disk(gomock.Any(), "zone", "zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4").Return(&compute.Disk{
		Status:           "READY",
		Users:            []string(nil),
		LabelFingerprint: "fingerprint",
		Labels:           map[string]string{"foo": "bar"},
	}, nil)
	s.MockService.EXPECT().SetDiskLabels(
		gomock.Any(), "zone", "zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4", "fingerprint",
		map[string]string{"foo": "bar"})

	source := s.setUpSource(c)
	errs, err := source.ReleaseVolumes(s.CallCtx, []string{"zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4"})
	c.Check(err, jc.ErrorIsNil)
	c.Check(errs, gc.HasLen, 1)
	c.Assert(errs[0], jc.ErrorIsNil)
}

func (s *volumeSourceSuite) TestImportVolumesInvalidCredentialError(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().Disk(gomock.Any(), "zone", "zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4").
		Return(nil, gce.InvalidCredentialError)
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)

	source := s.setUpSource(c)
	_, err := source.(storage.VolumeImporter).ImportVolume(
		s.CallCtx,
		"zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4", map[string]string{
			"juju-model-uuid":      "foo",
			"juju-controller-uuid": "bar",
		},
	)
	c.Check(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *volumeSourceSuite) TestImportVolume(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().Disk(gomock.Any(), "zone", "zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4").
		Return(&compute.Disk{
			Name:             "zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
			Status:           "READY",
			SizeGb:           1,
			Users:            []string(nil),
			LabelFingerprint: "fingerprint",
			Labels:           map[string]string{"foo": "bar"},
		}, nil)
	s.MockService.EXPECT().SetDiskLabels(
		gomock.Any(),
		"zone", "zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
		"fingerprint",
		map[string]string{
			"foo":                  "bar",
			"juju-controller-uuid": "bar",
			"juju-model-uuid":      "foo",
		})

	source := s.setUpSource(c)
	c.Assert(source, gc.Implements, new(storage.VolumeImporter))
	volumeInfo, err := source.(storage.VolumeImporter).ImportVolume(
		s.CallCtx,
		"zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4", map[string]string{
			"juju-model-uuid":      "foo",
			"juju-controller-uuid": "bar",
		},
	)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(volumeInfo, jc.DeepEquals, storage.VolumeInfo{
		VolumeId:   "zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
		Size:       1024,
		Persistent: true,
	})
}

func (s *volumeSourceSuite) TestImportVolumeNotReady(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().Disk(gomock.Any(), "zone", "zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4").
		Return(&compute.Disk{
			Status:           "FAILED",
			Users:            []string(nil),
			LabelFingerprint: "fingerprint",
			Labels:           map[string]string{"foo": "bar"},
		}, nil)

	source := s.setUpSource(c)
	_, err := source.(storage.VolumeImporter).ImportVolume(
		s.CallCtx,
		"zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4", map[string]string{},
	)
	c.Check(err, gc.ErrorMatches, `cannot import volume "zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4" with status "FAILED"`)
}

func (s *volumeSourceSuite) TestListVolumesInvalidCredentialError(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().Disks(gomock.Any()).Return(nil, gce.InvalidCredentialError)
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)

	source := s.setUpSource(c)
	_, err := source.ListVolumes(s.CallCtx)
	c.Check(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *volumeSourceSuite) TestListVolumes(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().Disks(gomock.Any()).Return([]*compute.Disk{{
		Name:   "zone--566fe7b2-c026-4a86-a2cc-84cb7f9a4868",
		Status: "READY",
		Labels: map[string]string{
			"juju-model-uuid": s.ModelUUID,
		},
	}}, nil)

	source := s.setUpSource(c)
	vols, err := source.ListVolumes(s.CallCtx)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(vols, jc.DeepEquals, []string{"zone--566fe7b2-c026-4a86-a2cc-84cb7f9a4868"})
}

func (s *volumeSourceSuite) TestListVolumesOnlyListsCurrentModelUUID(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().Disks(gomock.Any()).Return([]*compute.Disk{{
		Name:   "zone--566fe7b2-c026-4a86-a2cc-84cb7f9a4868",
		Status: "READY",
		Labels: map[string]string{
			"juju-model-uuid": s.ModelUUID,
		},
	}, {
		Name:   "zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
		Status: "READY",
		Labels: map[string]string{
			"juju-model-uuid": utils.MustNewUUID().String(),
		},
	}}, nil)

	source := s.setUpSource(c)
	vols, err := source.ListVolumes(s.CallCtx)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(vols, jc.DeepEquals, []string{"zone--566fe7b2-c026-4a86-a2cc-84cb7f9a4868"})
}

func (s *volumeSourceSuite) TestListVolumesIgnoresNamesFormattedDifferently(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().Disks(gomock.Any()).Return([]*compute.Disk{{
		Name:   "zone--566fe7b2-c026-4a86-a2cc-84cb7f9a4868",
		Status: "READY",
		Labels: map[string]string{
			"juju-model-uuid": s.ModelUUID,
		},
	}, {
		Name:   "c930380d-8337-4bf5-b07a-9dbb5ae771e4",
		Status: "READY",
		Labels: map[string]string{
			"juju-model-uuid": s.ModelUUID,
		},
	}}, nil)

	source := s.setUpSource(c)
	vols, err := source.ListVolumes(s.CallCtx)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(vols, jc.DeepEquals, []string{"zone--566fe7b2-c026-4a86-a2cc-84cb7f9a4868"})
}

func (s *volumeSourceSuite) TestDescribeVolumesInvalidCredentialError(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().Disk(gomock.Any(), "zone", "zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4").
		Return(nil, gce.InvalidCredentialError)
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)

	source := s.setUpSource(c)
	volName := "zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4"
	_, err := source.DescribeVolumes(s.CallCtx, []string{volName})
	c.Check(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *volumeSourceSuite) TestDescribeVolumes(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().Disk(gomock.Any(), "zone", "zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4").
		Return(&compute.Disk{
			Name:   "zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
			SizeGb: 1,
		}, nil)

	source := s.setUpSource(c)
	volName := "zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4"
	res, err := source.DescribeVolumes(s.CallCtx, []string{volName})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res[0].Error, jc.ErrorIsNil)
	c.Assert(res[0].VolumeInfo.Size, gc.Equals, uint64(1024))
	c.Assert(res[0].VolumeInfo.VolumeId, gc.Equals, volName)
}

func (s *volumeSourceSuite) TestAttachVolumes(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().InstanceDisks(gomock.Any(), "zone", "inst-0").
		Return([]*compute.AttachedDisk{{
			Source: "not-already-attached",
		}}, nil)
	s.MockService.EXPECT().AttachDisk(gomock.Any(), "zone", gomock.Any(), "inst-0", google.ModeRW).
		DoAndReturn(func(ctx context.Context, zone, volName, instanceId string, mode google.DiskMode) (*compute.AttachedDisk, error) {
			if !strings.HasPrefix(volName, zone+"--") {
				c.Fail()
			}
			return &compute.AttachedDisk{
				DeviceName: "zone-1234567",
			}, nil
		})

	source := s.setUpSource(c)
	attachments := []storage.VolumeAttachmentParams{*s.attachmentParams}
	res, err := source.AttachVolumes(s.CallCtx, attachments)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(res, gc.HasLen, 1)
	c.Assert(res[0].Error, jc.ErrorIsNil)
	c.Assert(res[0].VolumeAttachment.Volume.String(), gc.Equals, "volume-0")
	c.Assert(res[0].VolumeAttachment.Machine.String(), gc.Equals, "machine-0")
	c.Assert(res[0].VolumeAttachment.VolumeAttachmentInfo.DeviceName, gc.Equals, "")
	c.Assert(res[0].VolumeAttachment.VolumeAttachmentInfo.DeviceLink, gc.Equals, "/dev/disk/by-id/google-zone-1234567")
}

func (s *volumeSourceSuite) TestAttachVolumesInvalidCredentialError(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().InstanceDisks(gomock.Any(), "zone", "inst-0").
		Return(nil, gce.InvalidCredentialError)
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)

	source := s.setUpSource(c)
	_, err := source.AttachVolumes(s.CallCtx, []storage.VolumeAttachmentParams{*s.attachmentParams})
	c.Check(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *volumeSourceSuite) TestDetachVolumes(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().DetachDisk(gomock.Any(), "zone", "inst-0", "zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4")

	source := s.setUpSource(c)
	attachments := []storage.VolumeAttachmentParams{*s.attachmentParams}
	errs, err := source.DetachVolumes(s.CallCtx, attachments)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(errs, gc.HasLen, 1)
	c.Assert(errs[0], jc.ErrorIsNil)
}

func (s *volumeSourceSuite) TestDetachVolumesInvalidCredentialError(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	s.MockService.EXPECT().DetachDisk(gomock.Any(), "zone", "inst-0", "zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4").
		Return(gce.InvalidCredentialError)
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)

	source := s.setUpSource(c)
	_, err := source.DetachVolumes(s.CallCtx, []storage.VolumeAttachmentParams{*s.attachmentParams})
	c.Check(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}
