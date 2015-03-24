// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"time"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage/provider/registry"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/storageprovisioner"
)

type storageProvisionerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&storageProvisionerSuite{})

func (s *storageProvisionerSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	registry.RegisterProvider("dummy", &dummyProvider{})
	s.AddSuiteCleanup(func(*gc.C) {
		registry.RegisterProvider("dummy", nil)
	})
}

func (s *storageProvisionerSuite) TestStartStop(c *gc.C) {
	worker := storageprovisioner.NewStorageProvisioner(
		"dir",
		newMockVolumeAccessor(),
		newMockFilesystemAccessor(),
		&mockLifecycleManager{},
	)
	worker.Kill()
	c.Assert(worker.Wait(), gc.IsNil)
}

func (s *storageProvisionerSuite) TestVolumeAdded(c *gc.C) {
	expectedVolumes := []params.Volume{{
		VolumeTag:  "volume-1",
		VolumeId:   "id-1",
		Serial:     "serial-1",
		Size:       1024,
		Persistent: true,
	}, {
		VolumeTag: "volume-2",
		VolumeId:  "id-2",
		Serial:    "serial-2",
		Size:      1024,
	}}
	expectedVolumeAttachments := []params.VolumeAttachment{{
		VolumeTag:  "volume-1",
		MachineTag: "machine-1",
		DeviceName: "/dev/sda1",
	}}

	volumeInfoSet := make(chan struct{})
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.setVolumeInfo = func(volumes []params.Volume) ([]params.ErrorResult, error) {
		defer close(volumeInfoSet)
		c.Assert(volumes, gc.DeepEquals, expectedVolumes)
		return nil, nil
	}

	volumeAttachmentInfoSet := make(chan struct{})
	volumeAccessor.setVolumeAttachmentInfo = func(volumeAttachments []params.VolumeAttachment) ([]params.ErrorResult, error) {
		defer close(volumeAttachmentInfoSet)
		c.Assert(volumeAttachments, gc.DeepEquals, expectedVolumeAttachments)
		return nil, nil
	}
	lifecycleManager := &mockLifecycleManager{}

	filesystemAccessor := newMockFilesystemAccessor()

	worker := storageprovisioner.NewStorageProvisioner(
		"storage-dir", volumeAccessor, filesystemAccessor, lifecycleManager,
	)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	// The worker should create volumes according to ids "1" and "2".
	volumeAccessor.volumesWatcher.changes <- []string{"1", "2"}
	waitChannel(c, volumeInfoSet, "waiting for volume info to be set")
	waitChannel(c, volumeAttachmentInfoSet, "waiting for volume attachments to be set")
}

func (s *storageProvisionerSuite) TestFilesystemAdded(c *gc.C) {
	expectedFilesystems := []params.Filesystem{{
		FilesystemTag: "filesystem-1",
		FilesystemId:  "id-1",
		Size:          1024,
	}, {
		FilesystemTag: "filesystem-2",
		FilesystemId:  "id-2",
		Size:          1024,
	}}

	filesystemInfoSet := make(chan struct{})
	filesystemAccessor := newMockFilesystemAccessor()
	filesystemAccessor.setFilesystemInfo = func(filesystems []params.Filesystem) ([]params.ErrorResult, error) {
		defer close(filesystemInfoSet)
		c.Assert(filesystems, gc.DeepEquals, expectedFilesystems)
		return nil, nil
	}

	lifecycleManager := &mockLifecycleManager{}

	volumeAccessor := newMockVolumeAccessor()

	worker := storageprovisioner.NewStorageProvisioner(
		"storage-dir", volumeAccessor, filesystemAccessor, lifecycleManager,
	)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	// The worker should create filesystems according to ids "1" and "2".
	filesystemAccessor.filesystemsWatcher.changes <- []string{"1", "2"}
	waitChannel(c, filesystemInfoSet, "waiting for filesystem info to be set")
}

func (s *storageProvisionerSuite) TestVolumeAttachmentAdded(c *gc.C) {
	// We should only get a single volume attachment, because it is the
	// only combination where both machine and volume are already
	// provisioned, and the attachmenti s not.
	expectedVolumeAttachments := []params.VolumeAttachment{{
		VolumeTag:  "volume-1",
		MachineTag: "machine-1",
		DeviceName: "/dev/sda1",
	}}

	volumeAttachmentInfoSet := make(chan struct{})
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.setVolumeAttachmentInfo = func(volumeAttachments []params.VolumeAttachment) ([]params.ErrorResult, error) {
		defer close(volumeAttachmentInfoSet)
		c.Assert(volumeAttachments, gc.DeepEquals, expectedVolumeAttachments)
		return nil, nil
	}
	lifecycleManager := &mockLifecycleManager{}

	// volume-1 and machine-1 are provisioned.
	volumeAccessor.provisionedVolumes["volume-1"] = params.Volume{
		VolumeTag: "volume-1",
		VolumeId:  "vol-123",
	}
	volumeAccessor.provisionedMachines["machine-0"] = instance.Id("already-provisioned-0")
	volumeAccessor.provisionedMachines["machine-1"] = instance.Id("already-provisioned-1")

	// machine-0/volume-1 attachment is already created.
	//
	// TODO(axw) later we should ensure that a reattachment occurs
	// the first time the attachment is seen by the worker.
	alreadyAttached := params.MachineStorageId{
		MachineTag:    "machine-0",
		AttachmentTag: "volume-1",
	}
	volumeAccessor.provisionedAttachments[alreadyAttached] = params.VolumeAttachment{
		MachineTag: "machine-0",
		VolumeTag:  "volume-1",
	}

	filesystemAccessor := newMockFilesystemAccessor()

	worker := storageprovisioner.NewStorageProvisioner(
		"storage-dir", volumeAccessor, filesystemAccessor, lifecycleManager,
	)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	volumeAccessor.attachmentsWatcher.changes <- []params.MachineStorageId{{
		MachineTag: "machine-1", AttachmentTag: "volume-1",
	}, {
		MachineTag: "machine-1", AttachmentTag: "volume-2",
	}, {
		MachineTag: "machine-2", AttachmentTag: "volume-1",
	}, {
		MachineTag: "machine-0", AttachmentTag: "volume-1",
	}}
	waitChannel(c, volumeAttachmentInfoSet, "waiting for volume attachments to be set")
}

func (s *storageProvisionerSuite) TestFilesystemAttachmentAdded(c *gc.C) {
	// We should only get a single filesystem attachment, because it is the
	// only combination where both machine and filesystem are already
	// provisioned, and the attachmenti s not.
	expectedFilesystemAttachments := []params.FilesystemAttachment{{
		FilesystemTag: "filesystem-1",
		MachineTag:    "machine-1",
		MountPoint:    "/srv/fs-123",
	}}

	filesystemAttachmentInfoSet := make(chan struct{})
	filesystemAccessor := newMockFilesystemAccessor()
	filesystemAccessor.setFilesystemAttachmentInfo = func(filesystemAttachments []params.FilesystemAttachment) ([]params.ErrorResult, error) {
		defer close(filesystemAttachmentInfoSet)
		c.Assert(filesystemAttachments, gc.DeepEquals, expectedFilesystemAttachments)
		return nil, nil
	}
	lifecycleManager := &mockLifecycleManager{}

	// filesystem-1 and machine-1 are provisioned.
	filesystemAccessor.provisionedFilesystems["filesystem-1"] = params.Filesystem{
		FilesystemTag: "filesystem-1",
		FilesystemId:  "fs-123",
	}
	filesystemAccessor.provisionedMachines["machine-0"] = instance.Id("already-provisioned-0")
	filesystemAccessor.provisionedMachines["machine-1"] = instance.Id("already-provisioned-1")

	// machine-0/filesystem-1 attachment is already created.
	//
	// TODO(axw) later we should ensure that a reattachment occurs
	// the first time the attachment is seen by the worker.
	alreadyAttached := params.MachineStorageId{
		MachineTag:    "machine-0",
		AttachmentTag: "filesystem-1",
	}
	filesystemAccessor.provisionedAttachments[alreadyAttached] = params.FilesystemAttachment{
		MachineTag:    "machine-0",
		FilesystemTag: "filesystem-1",
	}

	volumeAccessor := newMockVolumeAccessor()

	worker := storageprovisioner.NewStorageProvisioner(
		"storage-dir", volumeAccessor, filesystemAccessor, lifecycleManager,
	)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	filesystemAccessor.attachmentsWatcher.changes <- []params.MachineStorageId{{
		MachineTag: "machine-1", AttachmentTag: "filesystem-1",
	}, {
		MachineTag: "machine-1", AttachmentTag: "filesystem-2",
	}, {
		MachineTag: "machine-2", AttachmentTag: "filesystem-1",
	}, {
		MachineTag: "machine-0", AttachmentTag: "filesystem-1",
	}}
	waitChannel(c, filesystemAttachmentInfoSet, "waiting for filesystem attachments to be set")
}

func waitChannel(c *gc.C, ch <-chan struct{}, activity string) {
	select {
	case <-ch:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out " + activity)
	}
}

// TODO(wallyworld) - test destroying volumes when done
