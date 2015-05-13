// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"errors"
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider/registry"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/storageprovisioner"
)

type storageProvisionerSuite struct {
	coretesting.BaseSuite
	provider                *dummyProvider
	managedFilesystemSource *mockManagedFilesystemSource
}

var _ = gc.Suite(&storageProvisionerSuite{})

func (s *storageProvisionerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.provider = &dummyProvider{dynamic: true}
	registry.RegisterProvider("dummy", s.provider)
	s.AddCleanup(func(*gc.C) {
		registry.RegisterProvider("dummy", nil)
	})

	s.managedFilesystemSource = nil
	s.PatchValue(
		storageprovisioner.NewManagedFilesystemSource,
		func(
			blockDevices map[names.VolumeTag]storage.BlockDevice,
			filesystems map[names.FilesystemTag]storage.Filesystem,
		) storage.FilesystemSource {
			s.managedFilesystemSource = &mockManagedFilesystemSource{
				blockDevices: blockDevices,
				filesystems:  filesystems,
			}
			return s.managedFilesystemSource
		},
	)
}

func (s *storageProvisionerSuite) TestStartStop(c *gc.C) {
	worker := storageprovisioner.NewStorageProvisioner(
		coretesting.EnvironmentTag,
		"dir",
		newMockVolumeAccessor(),
		newMockFilesystemAccessor(),
		&mockLifecycleManager{},
		newMockEnvironAccessor(c),
	)
	worker.Kill()
	c.Assert(worker.Wait(), gc.IsNil)
}

func (s *storageProvisionerSuite) TestVolumeAdded(c *gc.C) {
	expectedVolumes := []params.Volume{{
		VolumeTag:  "volume-1",
		VolumeId:   "id-1",
		HardwareId: "serial-1",
		Size:       1024,
		Persistent: true,
	}, {
		VolumeTag:  "volume-2",
		VolumeId:   "id-2",
		HardwareId: "serial-2",
		Size:       1024,
	}}
	expectedVolumeAttachments := []params.VolumeAttachment{{
		VolumeTag:  "volume-1",
		MachineTag: "machine-1",
		DeviceName: "/dev/sda1",
	}}

	volumeInfoSet := make(chan interface{})
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.setVolumeInfo = func(volumes []params.Volume) ([]params.ErrorResult, error) {
		defer close(volumeInfoSet)
		c.Assert(volumes, gc.DeepEquals, expectedVolumes)
		return nil, nil
	}

	volumeAttachmentInfoSet := make(chan interface{})
	volumeAccessor.setVolumeAttachmentInfo = func(volumeAttachments []params.VolumeAttachment) ([]params.ErrorResult, error) {
		defer close(volumeAttachmentInfoSet)
		c.Assert(volumeAttachments, gc.DeepEquals, expectedVolumeAttachments)
		return nil, nil
	}
	lifecycleManager := &mockLifecycleManager{}

	filesystemAccessor := newMockFilesystemAccessor()
	environAccessor := newMockEnvironAccessor(c)

	worker := storageprovisioner.NewStorageProvisioner(
		coretesting.EnvironmentTag,
		"storage-dir",
		volumeAccessor,
		filesystemAccessor,
		lifecycleManager,
		environAccessor,
	)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	// The worker should create volumes according to ids "1" and "2".
	volumeAccessor.volumesWatcher.changes <- []string{"1", "2"}
	// ... but not until the environment config is available.
	assertNoEvent(c, volumeInfoSet, "volume info set")
	assertNoEvent(c, volumeAttachmentInfoSet, "volume attachment info set")
	environAccessor.watcher.changes <- struct{}{}
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

	filesystemInfoSet := make(chan interface{})
	filesystemAccessor := newMockFilesystemAccessor()
	filesystemAccessor.setFilesystemInfo = func(filesystems []params.Filesystem) ([]params.ErrorResult, error) {
		defer close(filesystemInfoSet)
		c.Assert(filesystems, jc.SameContents, expectedFilesystems)
		return nil, nil
	}

	lifecycleManager := &mockLifecycleManager{}
	volumeAccessor := newMockVolumeAccessor()
	environAccessor := newMockEnvironAccessor(c)

	worker := storageprovisioner.NewStorageProvisioner(
		coretesting.EnvironmentTag,
		"storage-dir",
		volumeAccessor,
		filesystemAccessor,
		lifecycleManager,
		environAccessor,
	)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	// The worker should create filesystems according to ids "1" and "2".
	filesystemAccessor.filesystemsWatcher.changes <- []string{"1", "2"}
	// ... but not until the environment config is available.
	assertNoEvent(c, filesystemInfoSet, "filesystem info set")
	environAccessor.watcher.changes <- struct{}{}
	waitChannel(c, filesystemInfoSet, "waiting for filesystem info to be set")
}

func (s *storageProvisionerSuite) TestVolumeNeedsInstance(c *gc.C) {
	volumeAccessor := newMockVolumeAccessor()
	lifecycleManager := &mockLifecycleManager{}
	filesystemAccessor := newMockFilesystemAccessor()
	environAccessor := newMockEnvironAccessor(c)
	worker := storageprovisioner.NewStorageProvisioner(
		coretesting.EnvironmentTag,
		"storage-dir",
		volumeAccessor,
		filesystemAccessor,
		lifecycleManager,
		environAccessor,
	)
	defer worker.Wait()
	defer worker.Kill()

	// Note: we're testing the *current* behaviour. Later, the provisioner
	// should not rely on bouncing to wait for the instance, but should
	// implement a state machine that watches instances.
	volumeAccessor.volumesWatcher.changes <- []string{needsInstanceVolumeId}
	environAccessor.watcher.changes <- struct{}{}
	err := worker.Wait()
	c.Assert(err, gc.ErrorMatches, `provisioning volumes: creating volumes: need running instance to provision volume`)
}

func (s *storageProvisionerSuite) TestVolumeNonDynamic(c *gc.C) {
	volumeInfoSet := make(chan interface{})
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.setVolumeInfo = func([]params.Volume) ([]params.ErrorResult, error) {
		defer close(volumeInfoSet)
		return nil, nil
	}

	lifecycleManager := &mockLifecycleManager{}
	filesystemAccessor := newMockFilesystemAccessor()
	environAccessor := newMockEnvironAccessor(c)
	worker := storageprovisioner.NewStorageProvisioner(
		coretesting.EnvironmentTag,
		"storage-dir",
		volumeAccessor,
		filesystemAccessor,
		lifecycleManager,
		environAccessor,
	)
	defer worker.Wait()
	defer worker.Kill()

	// Volumes for non-dynamic providers should not be created.
	s.provider.dynamic = false
	environAccessor.watcher.changes <- struct{}{}
	volumeAccessor.volumesWatcher.changes <- []string{"1"}
	assertNoEvent(c, volumeInfoSet, "volume info set")
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

	volumeAttachmentInfoSet := make(chan interface{})
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.setVolumeAttachmentInfo = func(volumeAttachments []params.VolumeAttachment) ([]params.ErrorResult, error) {
		defer close(volumeAttachmentInfoSet)
		c.Assert(volumeAttachments, gc.DeepEquals, expectedVolumeAttachments)
		return nil, nil
	}
	lifecycleManager := &mockLifecycleManager{}

	// volume-1, machine-0, and machine-1 are provisioned.
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
	environAccessor := newMockEnvironAccessor(c)

	worker := storageprovisioner.NewStorageProvisioner(
		coretesting.EnvironmentTag,
		"storage-dir",
		volumeAccessor,
		filesystemAccessor,
		lifecycleManager,
		environAccessor,
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
	assertNoEvent(c, volumeAttachmentInfoSet, "volume attachment info set")
	volumeAccessor.volumesWatcher.changes <- []string{"1"}
	environAccessor.watcher.changes <- struct{}{}
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

	filesystemAttachmentInfoSet := make(chan interface{})
	filesystemAccessor := newMockFilesystemAccessor()
	filesystemAccessor.setFilesystemAttachmentInfo = func(filesystemAttachments []params.FilesystemAttachment) ([]params.ErrorResult, error) {
		defer close(filesystemAttachmentInfoSet)
		c.Assert(filesystemAttachments, gc.DeepEquals, expectedFilesystemAttachments)
		return nil, nil
	}

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

	lifecycleManager := &mockLifecycleManager{}
	volumeAccessor := newMockVolumeAccessor()
	environAccessor := newMockEnvironAccessor(c)

	worker := storageprovisioner.NewStorageProvisioner(
		coretesting.EnvironmentTag,
		"storage-dir",
		volumeAccessor,
		filesystemAccessor,
		lifecycleManager,
		environAccessor,
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
	// ... but not until the environment config is available.
	assertNoEvent(c, filesystemAttachmentInfoSet, "filesystem attachment info set")
	filesystemAccessor.filesystemsWatcher.changes <- []string{"1"}
	environAccessor.watcher.changes <- struct{}{}
	waitChannel(c, filesystemAttachmentInfoSet, "waiting for filesystem attachments to be set")
}

func (s *storageProvisionerSuite) TestCreateVolumeBackedFilesystem(c *gc.C) {
	filesystemInfoSet := make(chan interface{})
	filesystemAccessor := newMockFilesystemAccessor()
	filesystemAccessor.setFilesystemInfo = func(filesystems []params.Filesystem) ([]params.ErrorResult, error) {
		filesystemInfoSet <- filesystems
		return nil, nil
	}

	lifecycleManager := &mockLifecycleManager{}
	volumeAccessor := newMockVolumeAccessor()
	environAccessor := newMockEnvironAccessor(c)

	worker := storageprovisioner.NewStorageProvisioner(
		names.NewMachineTag("0"),
		"storage-dir",
		volumeAccessor,
		filesystemAccessor,
		lifecycleManager,
		environAccessor,
	)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	volumeAccessor.blockDevices[params.MachineStorageId{
		MachineTag:    "machine-0",
		AttachmentTag: "volume-0-0",
	}] = storage.BlockDevice{
		DeviceName: "xvdf1",
		Size:       123,
	}
	filesystemAccessor.filesystemsWatcher.changes <- []string{"0/0", "0/1"}
	assertNoEvent(c, filesystemInfoSet, "filesystem info set")
	environAccessor.watcher.changes <- struct{}{}

	// Only the block device for volume 0/0 is attached at the moment,
	// so only the corresponding filesystem will be created.
	filesystemInfo := waitChannel(
		c, filesystemInfoSet,
		"waiting for filesystem info to be set",
	).([]params.Filesystem)
	c.Assert(filesystemInfo, jc.DeepEquals, []params.Filesystem{{
		FilesystemTag: "filesystem-0-0",
		FilesystemId:  "xvdf1",
		Size:          123,
	}})

	// If we now attach the block device for volume 0/1 and trigger the
	// notification, then the storage provisioner will wake up and create
	// the filesystem.
	volumeAccessor.blockDevices[params.MachineStorageId{
		MachineTag:    "machine-0",
		AttachmentTag: "volume-0-1",
	}] = storage.BlockDevice{
		DeviceName: "xvdf2",
		Size:       246,
	}
	volumeAccessor.blockDevicesWatcher.changes <- struct{}{}
	filesystemInfo = waitChannel(
		c, filesystemInfoSet,
		"waiting for filesystem info to be set",
	).([]params.Filesystem)
	c.Assert(filesystemInfo, jc.DeepEquals, []params.Filesystem{{
		FilesystemTag: "filesystem-0-1",
		FilesystemId:  "xvdf2",
		Size:          246,
	}})
}

func (s *storageProvisionerSuite) TestAttachVolumeBackedFilesystem(c *gc.C) {
	infoSet := make(chan interface{})
	filesystemAccessor := newMockFilesystemAccessor()
	filesystemAccessor.setFilesystemAttachmentInfo = func(attachments []params.FilesystemAttachment) ([]params.ErrorResult, error) {
		infoSet <- attachments
		return nil, nil
	}

	lifecycleManager := &mockLifecycleManager{}
	volumeAccessor := newMockVolumeAccessor()
	environAccessor := newMockEnvironAccessor(c)

	worker := storageprovisioner.NewStorageProvisioner(
		names.NewMachineTag("0"),
		"storage-dir",
		volumeAccessor,
		filesystemAccessor,
		lifecycleManager,
		environAccessor,
	)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	filesystemAccessor.provisionedFilesystems["filesystem-0-0"] = params.Filesystem{
		FilesystemTag: "filesystem-0-0",
		VolumeTag:     "volume-0-0",
		FilesystemId:  "whatever",
		Size:          123,
	}
	filesystemAccessor.provisionedMachines["machine-0"] = instance.Id("already-provisioned-0")

	volumeAccessor.blockDevices[params.MachineStorageId{
		MachineTag:    "machine-0",
		AttachmentTag: "volume-0-0",
	}] = storage.BlockDevice{
		DeviceName: "xvdf1",
		Size:       123,
	}
	filesystemAccessor.attachmentsWatcher.changes <- []params.MachineStorageId{{
		MachineTag:    "machine-0",
		AttachmentTag: "filesystem-0-0",
	}}
	assertNoEvent(c, infoSet, "filesystem attachment info set")
	environAccessor.watcher.changes <- struct{}{}
	filesystemAccessor.filesystemsWatcher.changes <- []string{"0/0"}

	info := waitChannel(
		c, infoSet, "waiting for filesystem attachment info to be set",
	).([]params.FilesystemAttachment)
	c.Assert(info, jc.DeepEquals, []params.FilesystemAttachment{{
		FilesystemTag: "filesystem-0-0",
		MachineTag:    "machine-0",
		MountPoint:    "/mnt/xvdf1",
	}})
}

func (s *storageProvisionerSuite) TestUpdateEnvironConfig(c *gc.C) {
	volumeAccessor := newMockVolumeAccessor()
	lifecycleManager := &mockLifecycleManager{}
	filesystemAccessor := newMockFilesystemAccessor()
	environAccessor := newMockEnvironAccessor(c)

	s.provider.volumeSourceFunc = func(envConfig *config.Config, sourceConfig *storage.Config) (storage.VolumeSource, error) {
		c.Assert(envConfig, gc.NotNil)
		c.Assert(sourceConfig, gc.NotNil)
		c.Assert(envConfig.AllAttrs()["foo"], gc.Equals, "bar")
		return nil, errors.New("zinga")
	}

	worker := storageprovisioner.NewStorageProvisioner(
		coretesting.EnvironmentTag,
		"storage-dir",
		volumeAccessor,
		filesystemAccessor,
		lifecycleManager,
		environAccessor,
	)
	defer worker.Wait()
	defer worker.Kill()

	newConfig, err := environAccessor.cfg.Apply(map[string]interface{}{"foo": "bar"})
	c.Assert(err, jc.ErrorIsNil)

	environAccessor.watcher.changes <- struct{}{}
	environAccessor.setConfig(newConfig)
	environAccessor.watcher.changes <- struct{}{}
	volumeAccessor.volumesWatcher.changes <- []string{"1", "2"}

	err = worker.Wait()
	c.Assert(err, gc.ErrorMatches, `provisioning volumes: creating volumes: getting volume source: getting storage source "dummy": zinga`)
}

func waitChannel(c *gc.C, ch <-chan interface{}, activity string) interface{} {
	select {
	case v := <-ch:
		return v
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out " + activity)
		panic("unreachable")
	}
}

func assertNoEvent(c *gc.C, ch <-chan interface{}, event string) {
	select {
	case <-ch:
		c.Fatalf("unexpected " + event)
	case <-time.After(coretesting.ShortWait):
	}
}

// TODO(wallyworld) - test destroying volumes when done
