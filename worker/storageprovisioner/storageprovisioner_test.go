// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"errors"
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider/registry"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
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
		newMockMachineAccessor(c),
		&mockStatusSetter{},
		&mockClock{},
	)
	worker.Kill()
	c.Assert(worker.Wait(), gc.IsNil)
}

func (s *storageProvisionerSuite) TestVolumeAdded(c *gc.C) {
	expectedVolumes := []params.Volume{{
		VolumeTag: "volume-1",
		Info: params.VolumeInfo{
			VolumeId:   "id-1",
			HardwareId: "serial-1",
			Size:       1024,
			Persistent: true,
		},
	}, {
		VolumeTag: "volume-2",
		Info: params.VolumeInfo{
			VolumeId:   "id-2",
			HardwareId: "serial-2",
			Size:       1024,
		},
	}}
	expectedVolumeAttachments := []params.VolumeAttachment{{
		VolumeTag:  "volume-1",
		MachineTag: "machine-1",
		Info: params.VolumeAttachmentInfo{
			DeviceName: "/dev/sda1",
			ReadOnly:   true,
		},
	}, {
		VolumeTag:  "volume-2",
		MachineTag: "machine-1",
		Info: params.VolumeAttachmentInfo{
			DeviceName: "/dev/sda2",
		},
	}}

	volumeInfoSet := make(chan interface{})
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.provisionedMachines["machine-1"] = instance.Id("already-provisioned-1")
	volumeAccessor.setVolumeInfo = func(volumes []params.Volume) ([]params.ErrorResult, error) {
		defer close(volumeInfoSet)
		c.Assert(volumes, jc.SameContents, expectedVolumes)
		return nil, nil
	}

	volumeAttachmentInfoSet := make(chan interface{})
	volumeAccessor.setVolumeAttachmentInfo = func(volumeAttachments []params.VolumeAttachment) ([]params.ErrorResult, error) {
		defer close(volumeAttachmentInfoSet)
		c.Assert(volumeAttachments, jc.SameContents, expectedVolumeAttachments)
		return nil, nil
	}

	args := &workerArgs{volumes: volumeAccessor}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	volumeAccessor.attachmentsWatcher.changes <- []params.MachineStorageId{{
		MachineTag: "machine-1", AttachmentTag: "volume-1",
	}, {
		MachineTag: "machine-1", AttachmentTag: "volume-2",
	}}
	assertNoEvent(c, volumeAttachmentInfoSet, "volume attachment set")

	// The worker should create volumes according to ids "1" and "2".
	volumeAccessor.volumesWatcher.changes <- []string{"1", "2"}
	// ... but not until the environment config is available.
	assertNoEvent(c, volumeInfoSet, "volume info set")
	assertNoEvent(c, volumeAttachmentInfoSet, "volume attachment info set")
	args.environ.watcher.changes <- struct{}{}
	waitChannel(c, volumeInfoSet, "waiting for volume info to be set")
	waitChannel(c, volumeAttachmentInfoSet, "waiting for volume attachments to be set")
}

func (s *storageProvisionerSuite) TestCreateVolumeCreatesAttachment(c *gc.C) {
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.provisionedMachines["machine-1"] = instance.Id("already-provisioned-1")

	volumeAttachmentInfoSet := make(chan interface{})
	volumeAccessor.setVolumeAttachmentInfo = func(volumeAttachments []params.VolumeAttachment) ([]params.ErrorResult, error) {
		defer close(volumeAttachmentInfoSet)
		return make([]params.ErrorResult, len(volumeAttachments)), nil
	}

	s.provider.createVolumesFunc = func(args []storage.VolumeParams) ([]storage.CreateVolumesResult, error) {
		volumeAccessor.provisionedAttachments[params.MachineStorageId{
			MachineTag:    args[0].Attachment.Machine.String(),
			AttachmentTag: args[0].Attachment.Volume.String(),
		}] = params.VolumeAttachment{
			VolumeTag:  args[0].Attachment.Volume.String(),
			MachineTag: args[0].Attachment.Machine.String(),
		}
		return []storage.CreateVolumesResult{{
			Volume: &storage.Volume{
				Tag: args[0].Tag,
				VolumeInfo: storage.VolumeInfo{
					VolumeId: "vol-ume",
				},
			},
			VolumeAttachment: &storage.VolumeAttachment{
				Volume:  args[0].Attachment.Volume,
				Machine: args[0].Attachment.Machine,
			},
		}}, nil
	}

	attachVolumesCalled := make(chan interface{})
	s.provider.attachVolumesFunc = func(args []storage.VolumeAttachmentParams) ([]storage.AttachVolumesResult, error) {
		defer close(attachVolumesCalled)
		return nil, errors.New("should not be called")
	}

	args := &workerArgs{volumes: volumeAccessor}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	volumeAccessor.attachmentsWatcher.changes <- []params.MachineStorageId{{
		MachineTag: "machine-1", AttachmentTag: "volume-1",
	}}
	assertNoEvent(c, volumeAttachmentInfoSet, "volume attachment set")

	// The worker should create volumes according to ids "1".
	volumeAccessor.volumesWatcher.changes <- []string{"1"}
	args.environ.watcher.changes <- struct{}{}
	waitChannel(c, volumeAttachmentInfoSet, "waiting for volume attachments to be set")
	assertNoEvent(c, attachVolumesCalled, "AttachVolumes called")
}

func (s *storageProvisionerSuite) TestCreateVolumeRetry(c *gc.C) {
	volumeInfoSet := make(chan interface{})
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.provisionedMachines["machine-1"] = instance.Id("already-provisioned-1")
	volumeAccessor.setVolumeInfo = func(volumes []params.Volume) ([]params.ErrorResult, error) {
		defer close(volumeInfoSet)
		return make([]params.ErrorResult, len(volumes)), nil
	}

	// mockFunc's After will progress the current time by the specified
	// duration and signal the channel immediately.
	clock := &mockClock{}
	var createVolumeTimes []time.Time

	s.provider.createVolumesFunc = func(args []storage.VolumeParams) ([]storage.CreateVolumesResult, error) {
		createVolumeTimes = append(createVolumeTimes, clock.Now())
		if len(createVolumeTimes) < 10 {
			return []storage.CreateVolumesResult{{Error: errors.New("badness")}}, nil
		}
		return []storage.CreateVolumesResult{{
			Volume: &storage.Volume{Tag: args[0].Tag},
		}}, nil
	}

	args := &workerArgs{volumes: volumeAccessor, clock: clock}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	volumeAccessor.attachmentsWatcher.changes <- []params.MachineStorageId{{
		MachineTag: "machine-1", AttachmentTag: "volume-1",
	}}
	volumeAccessor.volumesWatcher.changes <- []string{"1"}
	args.environ.watcher.changes <- struct{}{}
	waitChannel(c, volumeInfoSet, "waiting for volume info to be set")
	c.Assert(createVolumeTimes, gc.HasLen, 10)

	// The first attempt should have been immediate: T0.
	c.Assert(createVolumeTimes[0], gc.Equals, time.Time{})

	delays := make([]time.Duration, len(createVolumeTimes)-1)
	for i := range createVolumeTimes[1:] {
		delays[i] = createVolumeTimes[i+1].Sub(createVolumeTimes[i])
	}
	c.Assert(delays, jc.DeepEquals, []time.Duration{
		30 * time.Second,
		1 * time.Minute,
		2 * time.Minute,
		4 * time.Minute,
		8 * time.Minute,
		16 * time.Minute,
		30 * time.Minute, // ceiling reached
		30 * time.Minute,
		30 * time.Minute,
	})

	c.Assert(args.statusSetter.args, jc.DeepEquals, []params.EntityStatusArgs{
		{Tag: "volume-1", Status: "pending", Info: "badness"},
		{Tag: "volume-1", Status: "pending", Info: "badness"},
		{Tag: "volume-1", Status: "pending", Info: "badness"},
		{Tag: "volume-1", Status: "pending", Info: "badness"},
		{Tag: "volume-1", Status: "pending", Info: "badness"},
		{Tag: "volume-1", Status: "pending", Info: "badness"},
		{Tag: "volume-1", Status: "pending", Info: "badness"},
		{Tag: "volume-1", Status: "pending", Info: "badness"},
		{Tag: "volume-1", Status: "pending", Info: "badness"},
		{Tag: "volume-1", Status: "attaching", Info: ""},
	})
}

func (s *storageProvisionerSuite) TestAttachVolumeRetry(c *gc.C) {
	volumeInfoSet := make(chan interface{})
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.provisionedMachines["machine-1"] = instance.Id("already-provisioned-1")
	volumeAccessor.setVolumeInfo = func(volumes []params.Volume) ([]params.ErrorResult, error) {
		defer close(volumeInfoSet)
		return make([]params.ErrorResult, len(volumes)), nil
	}
	volumeAttachmentInfoSet := make(chan interface{})
	volumeAccessor.setVolumeAttachmentInfo = func(volumeAttachments []params.VolumeAttachment) ([]params.ErrorResult, error) {
		defer close(volumeAttachmentInfoSet)
		return make([]params.ErrorResult, len(volumeAttachments)), nil
	}

	// mockFunc's After will progress the current time by the specified
	// duration and signal the channel immediately.
	clock := &mockClock{}
	var attachVolumeTimes []time.Time

	s.provider.attachVolumesFunc = func(args []storage.VolumeAttachmentParams) ([]storage.AttachVolumesResult, error) {
		attachVolumeTimes = append(attachVolumeTimes, clock.Now())
		if len(attachVolumeTimes) < 10 {
			return []storage.AttachVolumesResult{{Error: errors.New("badness")}}, nil
		}
		return []storage.AttachVolumesResult{{
			VolumeAttachment: &storage.VolumeAttachment{
				args[0].Volume,
				args[0].Machine,
				storage.VolumeAttachmentInfo{
					DeviceName: "/dev/sda1",
				},
			},
		}}, nil
	}

	args := &workerArgs{volumes: volumeAccessor, clock: clock}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	volumeAccessor.attachmentsWatcher.changes <- []params.MachineStorageId{{
		MachineTag: "machine-1", AttachmentTag: "volume-1",
	}}
	volumeAccessor.volumesWatcher.changes <- []string{"1"}
	args.environ.watcher.changes <- struct{}{}
	waitChannel(c, volumeInfoSet, "waiting for volume info to be set")
	waitChannel(c, volumeAttachmentInfoSet, "waiting for volume attachments to be set")
	c.Assert(attachVolumeTimes, gc.HasLen, 10)

	// The first attempt should have been immediate: T0.
	c.Assert(attachVolumeTimes[0], gc.Equals, time.Time{})

	delays := make([]time.Duration, len(attachVolumeTimes)-1)
	for i := range attachVolumeTimes[1:] {
		delays[i] = attachVolumeTimes[i+1].Sub(attachVolumeTimes[i])
	}
	c.Assert(delays, jc.DeepEquals, []time.Duration{
		30 * time.Second,
		1 * time.Minute,
		2 * time.Minute,
		4 * time.Minute,
		8 * time.Minute,
		16 * time.Minute,
		30 * time.Minute, // ceiling reached
		30 * time.Minute,
		30 * time.Minute,
	})

	c.Assert(args.statusSetter.args, jc.DeepEquals, []params.EntityStatusArgs{
		{Tag: "volume-1", Status: "attaching", Info: ""},        // CreateVolumes
		{Tag: "volume-1", Status: "attaching", Info: "badness"}, // AttachVolumes
		{Tag: "volume-1", Status: "attaching", Info: "badness"},
		{Tag: "volume-1", Status: "attaching", Info: "badness"},
		{Tag: "volume-1", Status: "attaching", Info: "badness"},
		{Tag: "volume-1", Status: "attaching", Info: "badness"},
		{Tag: "volume-1", Status: "attaching", Info: "badness"},
		{Tag: "volume-1", Status: "attaching", Info: "badness"},
		{Tag: "volume-1", Status: "attaching", Info: "badness"},
		{Tag: "volume-1", Status: "attaching", Info: "badness"},
		{Tag: "volume-1", Status: "attached", Info: ""},
	})
}

func (s *storageProvisionerSuite) TestFilesystemAdded(c *gc.C) {
	expectedFilesystems := []params.Filesystem{{
		FilesystemTag: "filesystem-1",
		Info: params.FilesystemInfo{
			FilesystemId: "id-1",
			Size:         1024,
		},
	}, {
		FilesystemTag: "filesystem-2",
		Info: params.FilesystemInfo{
			FilesystemId: "id-2",
			Size:         1024,
		},
	}}

	filesystemInfoSet := make(chan interface{})
	filesystemAccessor := newMockFilesystemAccessor()
	filesystemAccessor.setFilesystemInfo = func(filesystems []params.Filesystem) ([]params.ErrorResult, error) {
		defer close(filesystemInfoSet)
		c.Assert(filesystems, jc.SameContents, expectedFilesystems)
		return nil, nil
	}

	args := &workerArgs{filesystems: filesystemAccessor}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	// The worker should create filesystems according to ids "1" and "2".
	filesystemAccessor.filesystemsWatcher.changes <- []string{"1", "2"}
	// ... but not until the environment config is available.
	assertNoEvent(c, filesystemInfoSet, "filesystem info set")
	args.environ.watcher.changes <- struct{}{}
	waitChannel(c, filesystemInfoSet, "waiting for filesystem info to be set")
}

func (s *storageProvisionerSuite) TestVolumeNeedsInstance(c *gc.C) {
	volumeInfoSet := make(chan interface{})
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.setVolumeInfo = func([]params.Volume) ([]params.ErrorResult, error) {
		defer close(volumeInfoSet)
		return nil, nil
	}
	volumeAccessor.setVolumeAttachmentInfo = func([]params.VolumeAttachment) ([]params.ErrorResult, error) {
		return nil, nil
	}

	args := &workerArgs{volumes: volumeAccessor}
	worker := newStorageProvisioner(c, args)
	defer worker.Wait()
	defer worker.Kill()

	volumeAccessor.volumesWatcher.changes <- []string{needsInstanceVolumeId}
	args.environ.watcher.changes <- struct{}{}
	assertNoEvent(c, volumeInfoSet, "volume info set")
	args.machines.instanceIds[names.NewMachineTag("1")] = "inst-id"
	args.machines.watcher.changes <- struct{}{}
	waitChannel(c, volumeInfoSet, "waiting for volume info to be set")
}

func (s *storageProvisionerSuite) TestVolumeNonDynamic(c *gc.C) {
	volumeInfoSet := make(chan interface{})
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.setVolumeInfo = func([]params.Volume) ([]params.ErrorResult, error) {
		defer close(volumeInfoSet)
		return nil, nil
	}

	args := &workerArgs{volumes: volumeAccessor}
	worker := newStorageProvisioner(c, args)
	defer worker.Wait()
	defer worker.Kill()

	// Volumes for non-dynamic providers should not be created.
	s.provider.dynamic = false
	args.environ.watcher.changes <- struct{}{}
	volumeAccessor.volumesWatcher.changes <- []string{"1"}
	assertNoEvent(c, volumeInfoSet, "volume info set")
}

func (s *storageProvisionerSuite) TestVolumeAttachmentAdded(c *gc.C) {
	// We should get two volume attachments:
	//   - volume-1 to machine-1, because the volume and
	//     machine are provisioned, but the attachment is not.
	//   - volume-1 to machine-0, because the volume,
	//     machine, and attachment are provisioned, but
	//     in a previous session, so a reattachment is
	//     requested.
	expectedVolumeAttachments := []params.VolumeAttachment{{
		VolumeTag:  "volume-1",
		MachineTag: "machine-1",
		Info: params.VolumeAttachmentInfo{
			DeviceName: "/dev/sda1",
			ReadOnly:   true,
		},
	}, {
		VolumeTag:  "volume-1",
		MachineTag: "machine-0",
		Info: params.VolumeAttachmentInfo{
			DeviceName: "/dev/sda1",
			ReadOnly:   true,
		},
	}}

	var allVolumeAttachments []params.VolumeAttachment
	volumeAttachmentInfoSet := make(chan interface{})
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.setVolumeAttachmentInfo = func(volumeAttachments []params.VolumeAttachment) ([]params.ErrorResult, error) {
		allVolumeAttachments = append(allVolumeAttachments, volumeAttachments...)
		volumeAttachmentInfoSet <- nil
		return make([]params.ErrorResult, len(volumeAttachments)), nil
	}

	// volume-1, machine-0, and machine-1 are provisioned.
	volumeAccessor.provisionedVolumes["volume-1"] = params.Volume{
		VolumeTag: "volume-1",
		Info: params.VolumeInfo{
			VolumeId: "vol-123",
		},
	}
	volumeAccessor.provisionedMachines["machine-0"] = instance.Id("already-provisioned-0")
	volumeAccessor.provisionedMachines["machine-1"] = instance.Id("already-provisioned-1")

	// machine-0/volume-1 attachment is already created.
	// We should see a reattachment.
	alreadyAttached := params.MachineStorageId{
		MachineTag:    "machine-0",
		AttachmentTag: "volume-1",
	}
	volumeAccessor.provisionedAttachments[alreadyAttached] = params.VolumeAttachment{
		MachineTag: "machine-0",
		VolumeTag:  "volume-1",
	}

	args := &workerArgs{volumes: volumeAccessor}
	worker := newStorageProvisioner(c, args)
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
	args.environ.watcher.changes <- struct{}{}
	waitChannel(c, volumeAttachmentInfoSet, "waiting for volume attachments to be set")
	c.Assert(allVolumeAttachments, jc.SameContents, expectedVolumeAttachments)

	// Reattachment should only happen once per session.
	volumeAccessor.attachmentsWatcher.changes <- []params.MachineStorageId{alreadyAttached}
	assertNoEvent(c, volumeAttachmentInfoSet, "volume attachment info set")
}

func (s *storageProvisionerSuite) TestFilesystemAttachmentAdded(c *gc.C) {
	// We should only get a single filesystem attachment, because it is the
	// only combination where both machine and filesystem are already
	// provisioned, and the attachmenti s not.
	// We should get two filesystem attachments:
	//   - filesystem-1 to machine-1, because the filesystem and
	//     machine are provisioned, but the attachment is not.
	//   - filesystem-1 to machine-0, because the filesystem,
	//     machine, and attachment are provisioned, but in a
	//     previous session, so a reattachment is requested.
	expectedFilesystemAttachments := []params.FilesystemAttachment{{
		FilesystemTag: "filesystem-1",
		MachineTag:    "machine-1",
		Info: params.FilesystemAttachmentInfo{
			MountPoint: "/srv/fs-123",
		},
	}, {
		FilesystemTag: "filesystem-1",
		MachineTag:    "machine-0",
		Info: params.FilesystemAttachmentInfo{
			MountPoint: "/srv/fs-123",
		},
	}}

	var allFilesystemAttachments []params.FilesystemAttachment
	filesystemAttachmentInfoSet := make(chan interface{})
	filesystemAccessor := newMockFilesystemAccessor()
	filesystemAccessor.setFilesystemAttachmentInfo = func(filesystemAttachments []params.FilesystemAttachment) ([]params.ErrorResult, error) {
		allFilesystemAttachments = append(allFilesystemAttachments, filesystemAttachments...)
		filesystemAttachmentInfoSet <- nil
		return make([]params.ErrorResult, len(filesystemAttachments)), nil
	}

	// filesystem-1 and machine-1 are provisioned.
	filesystemAccessor.provisionedFilesystems["filesystem-1"] = params.Filesystem{
		FilesystemTag: "filesystem-1",
		Info: params.FilesystemInfo{
			FilesystemId: "fs-123",
		},
	}
	filesystemAccessor.provisionedMachines["machine-0"] = instance.Id("already-provisioned-0")
	filesystemAccessor.provisionedMachines["machine-1"] = instance.Id("already-provisioned-1")

	// machine-0/filesystem-1 attachment is already created.
	// We should see a reattachment.
	alreadyAttached := params.MachineStorageId{
		MachineTag:    "machine-0",
		AttachmentTag: "filesystem-1",
	}
	filesystemAccessor.provisionedAttachments[alreadyAttached] = params.FilesystemAttachment{
		MachineTag:    "machine-0",
		FilesystemTag: "filesystem-1",
	}

	args := &workerArgs{filesystems: filesystemAccessor}
	worker := newStorageProvisioner(c, args)
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
	args.environ.watcher.changes <- struct{}{}
	waitChannel(c, filesystemAttachmentInfoSet, "waiting for filesystem attachments to be set")
	c.Assert(allFilesystemAttachments, jc.SameContents, expectedFilesystemAttachments)

	// Reattachment should only happen once per session.
	filesystemAccessor.attachmentsWatcher.changes <- []params.MachineStorageId{alreadyAttached}
	assertNoEvent(c, filesystemAttachmentInfoSet, "filesystem attachment info set")
}

func (s *storageProvisionerSuite) TestCreateVolumeBackedFilesystem(c *gc.C) {
	filesystemInfoSet := make(chan interface{})
	filesystemAccessor := newMockFilesystemAccessor()
	filesystemAccessor.setFilesystemInfo = func(filesystems []params.Filesystem) ([]params.ErrorResult, error) {
		filesystemInfoSet <- filesystems
		return nil, nil
	}

	args := &workerArgs{
		scope:       names.NewMachineTag("0"),
		filesystems: filesystemAccessor,
	}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	args.volumes.blockDevices[params.MachineStorageId{
		MachineTag:    "machine-0",
		AttachmentTag: "volume-0-0",
	}] = storage.BlockDevice{
		DeviceName: "xvdf1",
		Size:       123,
	}
	filesystemAccessor.filesystemsWatcher.changes <- []string{"0/0", "0/1"}
	assertNoEvent(c, filesystemInfoSet, "filesystem info set")
	args.environ.watcher.changes <- struct{}{}

	// Only the block device for volume 0/0 is attached at the moment,
	// so only the corresponding filesystem will be created.
	filesystemInfo := waitChannel(
		c, filesystemInfoSet,
		"waiting for filesystem info to be set",
	).([]params.Filesystem)
	c.Assert(filesystemInfo, jc.DeepEquals, []params.Filesystem{{
		FilesystemTag: "filesystem-0-0",
		Info: params.FilesystemInfo{
			FilesystemId: "xvdf1",
			Size:         123,
		},
	}})

	// If we now attach the block device for volume 0/1 and trigger the
	// notification, then the storage provisioner will wake up and create
	// the filesystem.
	args.volumes.blockDevices[params.MachineStorageId{
		MachineTag:    "machine-0",
		AttachmentTag: "volume-0-1",
	}] = storage.BlockDevice{
		DeviceName: "xvdf2",
		Size:       246,
	}
	args.volumes.blockDevicesWatcher.changes <- struct{}{}
	filesystemInfo = waitChannel(
		c, filesystemInfoSet,
		"waiting for filesystem info to be set",
	).([]params.Filesystem)
	c.Assert(filesystemInfo, jc.DeepEquals, []params.Filesystem{{
		FilesystemTag: "filesystem-0-1",
		Info: params.FilesystemInfo{
			FilesystemId: "xvdf2",
			Size:         246,
		},
	}})
}

func (s *storageProvisionerSuite) TestAttachVolumeBackedFilesystem(c *gc.C) {
	infoSet := make(chan interface{})
	filesystemAccessor := newMockFilesystemAccessor()
	filesystemAccessor.setFilesystemAttachmentInfo = func(attachments []params.FilesystemAttachment) ([]params.ErrorResult, error) {
		infoSet <- attachments
		return nil, nil
	}

	args := &workerArgs{
		scope:       names.NewMachineTag("0"),
		filesystems: filesystemAccessor,
	}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	filesystemAccessor.provisionedFilesystems["filesystem-0-0"] = params.Filesystem{
		FilesystemTag: "filesystem-0-0",
		VolumeTag:     "volume-0-0",
		Info: params.FilesystemInfo{
			FilesystemId: "whatever",
			Size:         123,
		},
	}
	filesystemAccessor.provisionedMachines["machine-0"] = instance.Id("already-provisioned-0")

	args.volumes.blockDevices[params.MachineStorageId{
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
	args.environ.watcher.changes <- struct{}{}
	filesystemAccessor.filesystemsWatcher.changes <- []string{"0/0"}

	info := waitChannel(
		c, infoSet, "waiting for filesystem attachment info to be set",
	).([]params.FilesystemAttachment)
	c.Assert(info, jc.DeepEquals, []params.FilesystemAttachment{{
		FilesystemTag: "filesystem-0-0",
		MachineTag:    "machine-0",
		Info: params.FilesystemAttachmentInfo{
			MountPoint: "/mnt/xvdf1",
			ReadOnly:   true,
		},
	}})
}

func (s *storageProvisionerSuite) TestUpdateEnvironConfig(c *gc.C) {
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.provisionedMachines["machine-1"] = instance.Id("already-provisioned-1")
	s.provider.volumeSourceFunc = func(envConfig *config.Config, sourceConfig *storage.Config) (storage.VolumeSource, error) {
		c.Assert(envConfig, gc.NotNil)
		c.Assert(sourceConfig, gc.NotNil)
		c.Assert(envConfig.AllAttrs()["foo"], gc.Equals, "bar")
		return nil, errors.New("zinga")
	}

	args := &workerArgs{volumes: volumeAccessor}
	worker := newStorageProvisioner(c, args)
	defer worker.Wait()
	defer worker.Kill()

	newConfig, err := args.environ.cfg.Apply(map[string]interface{}{"foo": "bar"})
	c.Assert(err, jc.ErrorIsNil)

	args.environ.watcher.changes <- struct{}{}
	args.environ.setConfig(newConfig)
	args.environ.watcher.changes <- struct{}{}
	args.volumes.volumesWatcher.changes <- []string{"1", "2"}

	err = worker.Wait()
	c.Assert(err, gc.ErrorMatches, `creating volumes: getting volume source: getting storage source "dummy": zinga`)
}

func (s *storageProvisionerSuite) TestResourceTags(c *gc.C) {
	volumeInfoSet := make(chan interface{})
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.provisionedMachines["machine-1"] = instance.Id("already-provisioned-1")
	volumeAccessor.setVolumeInfo = func(volumes []params.Volume) ([]params.ErrorResult, error) {
		defer close(volumeInfoSet)
		return nil, nil
	}

	filesystemInfoSet := make(chan interface{})
	filesystemAccessor := newMockFilesystemAccessor()
	filesystemAccessor.provisionedMachines["machine-1"] = instance.Id("already-provisioned-1")
	filesystemAccessor.setFilesystemInfo = func(filesystems []params.Filesystem) ([]params.ErrorResult, error) {
		defer close(filesystemInfoSet)
		return nil, nil
	}

	var volumeSource dummyVolumeSource
	s.provider.volumeSourceFunc = func(envConfig *config.Config, sourceConfig *storage.Config) (storage.VolumeSource, error) {
		return &volumeSource, nil
	}

	var filesystemSource dummyFilesystemSource
	s.provider.filesystemSourceFunc = func(envConfig *config.Config, sourceConfig *storage.Config) (storage.FilesystemSource, error) {
		return &filesystemSource, nil
	}

	args := &workerArgs{
		volumes:     volumeAccessor,
		filesystems: filesystemAccessor,
	}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	volumeAccessor.volumesWatcher.changes <- []string{"1"}
	filesystemAccessor.filesystemsWatcher.changes <- []string{"1"}
	args.environ.watcher.changes <- struct{}{}
	waitChannel(c, volumeInfoSet, "waiting for volume info to be set")
	waitChannel(c, filesystemInfoSet, "waiting for filesystem info to be set")
	c.Assert(volumeSource.createVolumesArgs, jc.DeepEquals, [][]storage.VolumeParams{{{
		Tag:          names.NewVolumeTag("1"),
		Size:         1024,
		Provider:     "dummy",
		Attributes:   map[string]interface{}{"persistent": true},
		ResourceTags: map[string]string{"very": "fancy"},
		Attachment: &storage.VolumeAttachmentParams{
			Volume: names.NewVolumeTag("1"),
			AttachmentParams: storage.AttachmentParams{
				Machine:    names.NewMachineTag("1"),
				Provider:   "dummy",
				InstanceId: "already-provisioned-1",
				ReadOnly:   true,
			},
		},
	}}})
	c.Assert(filesystemSource.createFilesystemsArgs, jc.DeepEquals, [][]storage.FilesystemParams{{{
		Tag:          names.NewFilesystemTag("1"),
		Size:         1024,
		Provider:     "dummy",
		ResourceTags: map[string]string{"very": "fancy"},
	}}})
}

func (s *storageProvisionerSuite) TestSetVolumeInfoErrorStopsWorker(c *gc.C) {
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.provisionedMachines["machine-1"] = instance.Id("already-provisioned-1")
	volumeAccessor.setVolumeInfo = func(volumes []params.Volume) ([]params.ErrorResult, error) {
		return nil, errors.New("belly up")
	}

	args := &workerArgs{volumes: volumeAccessor}
	worker := newStorageProvisioner(c, args)
	defer worker.Wait()
	defer worker.Kill()

	done := make(chan interface{})
	go func() {
		defer close(done)
		err := worker.Wait()
		c.Assert(err, gc.ErrorMatches, "creating volumes: publishing volumes to state: belly up")
	}()

	args.volumes.volumesWatcher.changes <- []string{"1"}
	args.environ.watcher.changes <- struct{}{}
	waitChannel(c, done, "waiting for worker to exit")
}

func (s *storageProvisionerSuite) TestSetVolumeInfoErrorResultDoesNotStopWorker(c *gc.C) {
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.provisionedMachines["machine-1"] = instance.Id("already-provisioned-1")
	volumeAccessor.setVolumeInfo = func(volumes []params.Volume) ([]params.ErrorResult, error) {
		return []params.ErrorResult{{Error: &params.Error{Message: "message", Code: "code"}}}, nil
	}

	args := &workerArgs{volumes: volumeAccessor}
	worker := newStorageProvisioner(c, args)
	defer func() {
		err := worker.Wait()
		c.Assert(err, jc.ErrorIsNil)
	}()
	defer worker.Kill()

	done := make(chan interface{})
	go func() {
		defer close(done)
		worker.Wait()
	}()

	args.volumes.volumesWatcher.changes <- []string{"1"}
	args.environ.watcher.changes <- struct{}{}
	assertNoEvent(c, done, "worker exited")
}

func (s *storageProvisionerSuite) TestDetachVolumesUnattached(c *gc.C) {
	removed := make(chan interface{})
	removeAttachments := func(ids []params.MachineStorageId) ([]params.ErrorResult, error) {
		defer close(removed)
		c.Assert(ids, gc.DeepEquals, []params.MachineStorageId{{
			MachineTag:    "machine-0",
			AttachmentTag: "volume-0",
		}})
		return make([]params.ErrorResult, len(ids)), nil
	}

	args := &workerArgs{
		life: &mockLifecycleManager{removeAttachments: removeAttachments},
	}
	worker := newStorageProvisioner(c, args)
	defer worker.Wait()
	defer worker.Kill()

	args.volumes.attachmentsWatcher.changes <- []params.MachineStorageId{{
		MachineTag: "machine-0", AttachmentTag: "volume-0",
	}}
	args.environ.watcher.changes <- struct{}{}
	waitChannel(c, removed, "waiting for attachment to be removed")
}

func (s *storageProvisionerSuite) TestDetachVolumes(c *gc.C) {
	var attached bool
	volumeAttachmentInfoSet := make(chan interface{})
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.setVolumeAttachmentInfo = func(volumeAttachments []params.VolumeAttachment) ([]params.ErrorResult, error) {
		close(volumeAttachmentInfoSet)
		attached = true
		for _, a := range volumeAttachments {
			id := params.MachineStorageId{
				MachineTag:    a.MachineTag,
				AttachmentTag: a.VolumeTag,
			}
			volumeAccessor.provisionedAttachments[id] = a
		}
		return make([]params.ErrorResult, len(volumeAttachments)), nil
	}

	expectedAttachmentIds := []params.MachineStorageId{{
		MachineTag: "machine-1", AttachmentTag: "volume-1",
	}}

	attachmentLife := func(ids []params.MachineStorageId) ([]params.LifeResult, error) {
		c.Assert(ids, gc.DeepEquals, expectedAttachmentIds)
		life := params.Alive
		if attached {
			life = params.Dying
		}
		return []params.LifeResult{{Life: life}}, nil
	}

	detached := make(chan interface{})
	s.provider.detachVolumesFunc = func(args []storage.VolumeAttachmentParams) ([]error, error) {
		c.Assert(args, gc.HasLen, 1)
		c.Assert(args[0].Machine.String(), gc.Equals, expectedAttachmentIds[0].MachineTag)
		c.Assert(args[0].Volume.String(), gc.Equals, expectedAttachmentIds[0].AttachmentTag)
		defer close(detached)
		return make([]error, len(args)), nil
	}

	removed := make(chan interface{})
	removeAttachments := func(ids []params.MachineStorageId) ([]params.ErrorResult, error) {
		c.Assert(ids, gc.DeepEquals, expectedAttachmentIds)
		close(removed)
		return make([]params.ErrorResult, len(ids)), nil
	}

	// volume-1 and machine-1 are provisioned.
	volumeAccessor.provisionedVolumes["volume-1"] = params.Volume{
		VolumeTag: "volume-1",
		Info: params.VolumeInfo{
			VolumeId: "vol-123",
		},
	}
	volumeAccessor.provisionedMachines["machine-1"] = instance.Id("already-provisioned-1")

	args := &workerArgs{
		volumes: volumeAccessor,
		life: &mockLifecycleManager{
			attachmentLife:    attachmentLife,
			removeAttachments: removeAttachments,
		},
	}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	volumeAccessor.attachmentsWatcher.changes <- []params.MachineStorageId{{
		MachineTag: "machine-1", AttachmentTag: "volume-1",
	}}
	volumeAccessor.volumesWatcher.changes <- []string{"1"}
	args.environ.watcher.changes <- struct{}{}
	waitChannel(c, volumeAttachmentInfoSet, "waiting for volume attachments to be set")
	volumeAccessor.attachmentsWatcher.changes <- []params.MachineStorageId{{
		MachineTag: "machine-1", AttachmentTag: "volume-1",
	}}
	waitChannel(c, detached, "waiting for volume to be detached")
	waitChannel(c, removed, "waiting for attachment to be removed")
}

func (s *storageProvisionerSuite) TestDetachVolumesRetry(c *gc.C) {
	machine := names.NewMachineTag("1")
	volume := names.NewVolumeTag("1")
	attachmentId := params.MachineStorageId{
		MachineTag: machine.String(), AttachmentTag: volume.String(),
	}
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.provisionedAttachments[attachmentId] = params.VolumeAttachment{
		MachineTag: machine.String(),
		VolumeTag:  volume.String(),
	}
	volumeAccessor.provisionedVolumes[volume.String()] = params.Volume{
		VolumeTag: volume.String(),
		Info: params.VolumeInfo{
			VolumeId: "vol-123",
		},
	}
	volumeAccessor.provisionedMachines[machine.String()] = instance.Id("already-provisioned-1")

	attachmentLife := func(ids []params.MachineStorageId) ([]params.LifeResult, error) {
		return []params.LifeResult{{Life: params.Dying}}, nil
	}

	// mockFunc's After will progress the current time by the specified
	// duration and signal the channel immediately.
	clock := &mockClock{}
	var detachVolumeTimes []time.Time

	s.provider.detachVolumesFunc = func(args []storage.VolumeAttachmentParams) ([]error, error) {
		detachVolumeTimes = append(detachVolumeTimes, clock.Now())
		if len(detachVolumeTimes) < 10 {
			return []error{errors.New("badness")}, nil
		}
		return []error{nil}, nil
	}

	removed := make(chan interface{})
	removeAttachments := func(ids []params.MachineStorageId) ([]params.ErrorResult, error) {
		close(removed)
		return make([]params.ErrorResult, len(ids)), nil
	}

	args := &workerArgs{
		volumes: volumeAccessor,
		clock:   clock,
		life: &mockLifecycleManager{
			attachmentLife:    attachmentLife,
			removeAttachments: removeAttachments,
		},
	}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	volumeAccessor.volumesWatcher.changes <- []string{volume.Id()}
	args.environ.watcher.changes <- struct{}{}
	volumeAccessor.attachmentsWatcher.changes <- []params.MachineStorageId{attachmentId}
	waitChannel(c, removed, "waiting for attachment to be removed")
	c.Assert(detachVolumeTimes, gc.HasLen, 10)

	// The first attempt should have been immediate: T0.
	c.Assert(detachVolumeTimes[0], gc.Equals, time.Time{})

	delays := make([]time.Duration, len(detachVolumeTimes)-1)
	for i := range detachVolumeTimes[1:] {
		delays[i] = detachVolumeTimes[i+1].Sub(detachVolumeTimes[i])
	}
	c.Assert(delays, jc.DeepEquals, []time.Duration{
		30 * time.Second,
		1 * time.Minute,
		2 * time.Minute,
		4 * time.Minute,
		8 * time.Minute,
		16 * time.Minute,
		30 * time.Minute, // ceiling reached
		30 * time.Minute,
		30 * time.Minute,
	})

	c.Assert(args.statusSetter.args, jc.DeepEquals, []params.EntityStatusArgs{
		{Tag: "volume-1", Status: "detaching", Info: "badness"}, // DetachVolumes
		{Tag: "volume-1", Status: "detaching", Info: "badness"},
		{Tag: "volume-1", Status: "detaching", Info: "badness"},
		{Tag: "volume-1", Status: "detaching", Info: "badness"},
		{Tag: "volume-1", Status: "detaching", Info: "badness"},
		{Tag: "volume-1", Status: "detaching", Info: "badness"},
		{Tag: "volume-1", Status: "detaching", Info: "badness"},
		{Tag: "volume-1", Status: "detaching", Info: "badness"},
		{Tag: "volume-1", Status: "detaching", Info: "badness"},
		{Tag: "volume-1", Status: "detached", Info: ""},
	})
}

func (s *storageProvisionerSuite) TestDetachFilesystemsUnattached(c *gc.C) {
	removed := make(chan interface{})
	removeAttachments := func(ids []params.MachineStorageId) ([]params.ErrorResult, error) {
		defer close(removed)
		c.Assert(ids, gc.DeepEquals, []params.MachineStorageId{{
			MachineTag:    "machine-0",
			AttachmentTag: "filesystem-0",
		}})
		return make([]params.ErrorResult, len(ids)), nil
	}

	args := &workerArgs{
		life: &mockLifecycleManager{removeAttachments: removeAttachments},
	}
	worker := newStorageProvisioner(c, args)
	defer worker.Wait()
	defer worker.Kill()

	args.filesystems.attachmentsWatcher.changes <- []params.MachineStorageId{{
		MachineTag: "machine-0", AttachmentTag: "filesystem-0",
	}}
	args.environ.watcher.changes <- struct{}{}
	waitChannel(c, removed, "waiting for attachment to be removed")
}

func (s *storageProvisionerSuite) TestDetachFilesystems(c *gc.C) {
	var attached bool
	filesystemAttachmentInfoSet := make(chan interface{})
	filesystemAccessor := newMockFilesystemAccessor()
	filesystemAccessor.setFilesystemAttachmentInfo = func(filesystemAttachments []params.FilesystemAttachment) ([]params.ErrorResult, error) {
		close(filesystemAttachmentInfoSet)
		attached = true
		for _, a := range filesystemAttachments {
			id := params.MachineStorageId{
				MachineTag:    a.MachineTag,
				AttachmentTag: a.FilesystemTag,
			}
			filesystemAccessor.provisionedAttachments[id] = a
		}
		return make([]params.ErrorResult, len(filesystemAttachments)), nil
	}

	expectedAttachmentIds := []params.MachineStorageId{{
		MachineTag: "machine-1", AttachmentTag: "filesystem-1",
	}}

	attachmentLife := func(ids []params.MachineStorageId) ([]params.LifeResult, error) {
		c.Assert(ids, gc.DeepEquals, expectedAttachmentIds)
		life := params.Alive
		if attached {
			life = params.Dying
		}
		return []params.LifeResult{{Life: life}}, nil
	}

	detached := make(chan interface{})
	s.provider.detachFilesystemsFunc = func(args []storage.FilesystemAttachmentParams) error {
		c.Assert(args, gc.HasLen, 1)
		c.Assert(args[0].Machine.String(), gc.Equals, expectedAttachmentIds[0].MachineTag)
		c.Assert(args[0].Filesystem.String(), gc.Equals, expectedAttachmentIds[0].AttachmentTag)
		defer close(detached)
		return nil
	}

	removed := make(chan interface{})
	removeAttachments := func(ids []params.MachineStorageId) ([]params.ErrorResult, error) {
		c.Assert(ids, gc.DeepEquals, expectedAttachmentIds)
		close(removed)
		return make([]params.ErrorResult, len(ids)), nil
	}

	// filesystem-1 and machine-1 are provisioned.
	filesystemAccessor.provisionedFilesystems["filesystem-1"] = params.Filesystem{
		FilesystemTag: "filesystem-1",
		Info: params.FilesystemInfo{
			FilesystemId: "fs-id",
		},
	}
	filesystemAccessor.provisionedMachines["machine-1"] = instance.Id("already-provisioned-1")

	args := &workerArgs{
		filesystems: filesystemAccessor,
		life: &mockLifecycleManager{
			attachmentLife:    attachmentLife,
			removeAttachments: removeAttachments,
		},
	}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	filesystemAccessor.attachmentsWatcher.changes <- []params.MachineStorageId{{
		MachineTag: "machine-1", AttachmentTag: "filesystem-1",
	}}
	filesystemAccessor.filesystemsWatcher.changes <- []string{"1"}
	args.environ.watcher.changes <- struct{}{}
	waitChannel(c, filesystemAttachmentInfoSet, "waiting for filesystem attachments to be set")
	filesystemAccessor.attachmentsWatcher.changes <- []params.MachineStorageId{{
		MachineTag: "machine-1", AttachmentTag: "filesystem-1",
	}}
	waitChannel(c, detached, "waiting for filesystem to be detached")
	waitChannel(c, removed, "waiting for attachment to be removed")
}

func (s *storageProvisionerSuite) TestDestroyVolumes(c *gc.C) {
	provisionedVolume := names.NewVolumeTag("1")
	unprovisionedVolume := names.NewVolumeTag("2")

	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.provisionVolume(provisionedVolume)

	life := func(tags []names.Tag) ([]params.LifeResult, error) {
		results := make([]params.LifeResult, len(tags))
		for i := range results {
			results[i].Life = params.Dead
		}
		return results, nil
	}

	destroyedChan := make(chan interface{}, 1)
	s.provider.destroyVolumesFunc = func(volumeIds []string) ([]error, error) {
		destroyedChan <- volumeIds
		return make([]error, len(volumeIds)), nil
	}

	removedChan := make(chan interface{}, 1)
	remove := func(tags []names.Tag) ([]params.ErrorResult, error) {
		removedChan <- tags
		return make([]params.ErrorResult, len(tags)), nil
	}

	args := &workerArgs{
		volumes: volumeAccessor,
		life: &mockLifecycleManager{
			life:   life,
			remove: remove,
		},
	}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	volumeAccessor.volumesWatcher.changes <- []string{
		provisionedVolume.Id(),
		unprovisionedVolume.Id(),
	}
	args.environ.watcher.changes <- struct{}{}

	// Both volumes should be removed; the provisioned one
	// should be deprovisioned first.

	destroyed := waitChannel(c, destroyedChan, "waiting for volume to be deprovisioned")
	assertNoEvent(c, destroyedChan, "volumes deprovisioned")
	c.Assert(destroyed, jc.DeepEquals, []string{"vol-1"})

	var removed []names.Tag
	for len(removed) < 2 {
		tags := waitChannel(c, removedChan, "waiting for volumes to be removed").([]names.Tag)
		removed = append(removed, tags...)
	}
	c.Assert(removed, jc.SameContents, []names.Tag{provisionedVolume, unprovisionedVolume})
	assertNoEvent(c, removedChan, "volumes removed")
}

func (s *storageProvisionerSuite) TestDestroyVolumesRetry(c *gc.C) {
	volume := names.NewVolumeTag("1")
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.provisionVolume(volume)

	life := func(tags []names.Tag) ([]params.LifeResult, error) {
		return []params.LifeResult{{Life: params.Dead}}, nil
	}

	// mockFunc's After will progress the current time by the specified
	// duration and signal the channel immediately.
	clock := &mockClock{}
	var destroyVolumeTimes []time.Time

	s.provider.destroyVolumesFunc = func(volumeIds []string) ([]error, error) {
		destroyVolumeTimes = append(destroyVolumeTimes, clock.Now())
		if len(destroyVolumeTimes) < 10 {
			return []error{errors.New("badness")}, nil
		}
		return []error{nil}, nil
	}

	removedChan := make(chan interface{}, 1)
	remove := func(tags []names.Tag) ([]params.ErrorResult, error) {
		removedChan <- tags
		return make([]params.ErrorResult, len(tags)), nil
	}

	args := &workerArgs{
		volumes: volumeAccessor,
		clock:   clock,
		life: &mockLifecycleManager{
			life:   life,
			remove: remove,
		},
	}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	volumeAccessor.volumesWatcher.changes <- []string{volume.Id()}
	args.environ.watcher.changes <- struct{}{}
	waitChannel(c, removedChan, "waiting for volume to be removed")
	c.Assert(destroyVolumeTimes, gc.HasLen, 10)

	// The first attempt should have been immediate: T0.
	c.Assert(destroyVolumeTimes[0], gc.Equals, time.Time{})

	delays := make([]time.Duration, len(destroyVolumeTimes)-1)
	for i := range destroyVolumeTimes[1:] {
		delays[i] = destroyVolumeTimes[i+1].Sub(destroyVolumeTimes[i])
	}
	c.Assert(delays, jc.DeepEquals, []time.Duration{
		30 * time.Second,
		1 * time.Minute,
		2 * time.Minute,
		4 * time.Minute,
		8 * time.Minute,
		16 * time.Minute,
		30 * time.Minute, // ceiling reached
		30 * time.Minute,
		30 * time.Minute,
	})

	c.Assert(args.statusSetter.args, jc.DeepEquals, []params.EntityStatusArgs{
		{Tag: "volume-1", Status: "destroying", Info: "badness"},
		{Tag: "volume-1", Status: "destroying", Info: "badness"},
		{Tag: "volume-1", Status: "destroying", Info: "badness"},
		{Tag: "volume-1", Status: "destroying", Info: "badness"},
		{Tag: "volume-1", Status: "destroying", Info: "badness"},
		{Tag: "volume-1", Status: "destroying", Info: "badness"},
		{Tag: "volume-1", Status: "destroying", Info: "badness"},
		{Tag: "volume-1", Status: "destroying", Info: "badness"},
		{Tag: "volume-1", Status: "destroying", Info: "badness"},
	})
}

func (s *storageProvisionerSuite) TestDestroyFilesystems(c *gc.C) {
	provisionedFilesystem := names.NewFilesystemTag("1")
	unprovisionedFilesystem := names.NewFilesystemTag("2")

	filesystemAccessor := newMockFilesystemAccessor()
	filesystemAccessor.provisionFilesystem(provisionedFilesystem)

	life := func(tags []names.Tag) ([]params.LifeResult, error) {
		results := make([]params.LifeResult, len(tags))
		for i := range results {
			results[i].Life = params.Dead
		}
		return results, nil
	}

	removedChan := make(chan interface{}, 1)
	remove := func(tags []names.Tag) ([]params.ErrorResult, error) {
		removedChan <- tags
		return make([]params.ErrorResult, len(tags)), nil
	}

	args := &workerArgs{
		filesystems: filesystemAccessor,
		life: &mockLifecycleManager{
			life:   life,
			remove: remove,
		},
	}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	filesystemAccessor.filesystemsWatcher.changes <- []string{
		provisionedFilesystem.Id(),
		unprovisionedFilesystem.Id(),
	}
	args.environ.watcher.changes <- struct{}{}

	// Both filesystems should be removed; the provisioned one
	// *should* be deprovisioned first, but we don't currently
	// have the ability to do so via the storage provider API.

	var removed []names.Tag
	for len(removed) < 2 {
		tags := waitChannel(c, removedChan, "waiting for filesystems to be removed").([]names.Tag)
		removed = append(removed, tags...)
	}
	c.Assert(removed, jc.SameContents, []names.Tag{provisionedFilesystem, unprovisionedFilesystem})
	assertNoEvent(c, removedChan, "filesystems removed")
}

func newStorageProvisioner(c *gc.C, args *workerArgs) worker.Worker {
	if args == nil {
		args = &workerArgs{}
	}
	if args.scope == nil {
		args.scope = coretesting.EnvironmentTag
	}
	if args.volumes == nil {
		args.volumes = newMockVolumeAccessor()
	}
	if args.filesystems == nil {
		args.filesystems = newMockFilesystemAccessor()
	}
	if args.life == nil {
		args.life = &mockLifecycleManager{}
	}
	if args.environ == nil {
		args.environ = newMockEnvironAccessor(c)
	}
	if args.machines == nil {
		args.machines = newMockMachineAccessor(c)
	}
	if args.clock == nil {
		args.clock = &mockClock{}
	}
	if args.statusSetter == nil {
		args.statusSetter = &mockStatusSetter{}
	}
	return storageprovisioner.NewStorageProvisioner(
		args.scope,
		"storage-dir",
		args.volumes,
		args.filesystems,
		args.life,
		args.environ,
		args.machines,
		args.statusSetter,
		args.clock,
	)
}

type workerArgs struct {
	scope        names.Tag
	volumes      *mockVolumeAccessor
	filesystems  *mockFilesystemAccessor
	life         *mockLifecycleManager
	environ      *mockEnvironAccessor
	machines     *mockMachineAccessor
	clock        clock.Clock
	statusSetter *mockStatusSetter
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
