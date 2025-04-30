// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/storageprovisioner"
	"github.com/juju/juju/rpc/params"
)

type storageProvisionerSuite struct {
	coretesting.BaseSuite
	provider                *dummyProvider
	registry                storage.ProviderRegistry
	managedFilesystemSource *mockManagedFilesystemSource
}

var _ = gc.Suite(&storageProvisionerSuite{})

func (s *storageProvisionerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.provider = &dummyProvider{dynamic: true}
	s.registry = storage.StaticProviderRegistry{
		Providers: map[storage.ProviderType]storage.Provider{
			"dummy": s.provider,
		},
	}

	s.managedFilesystemSource = nil
	s.PatchValue(
		storageprovisioner.NewManagedFilesystemSource,
		func(
			blockDevices map[names.VolumeTag]blockdevice.BlockDevice,
			filesystems map[names.FilesystemTag]storage.Filesystem,
		) storage.FilesystemSource {
			s.managedFilesystemSource = &mockManagedFilesystemSource{
				blockDevices: blockDevices,
				filesystems:  filesystems,
			}
			return s.managedFilesystemSource
		},
	)
	s.PatchValue(storageprovisioner.DefaultDependentChangesTimeout, 10*time.Millisecond)
}

func (s *storageProvisionerSuite) TestStartStop(c *gc.C) {
	worker, err := storageprovisioner.NewStorageProvisioner(storageprovisioner.Config{
		Scope:       coretesting.ModelTag,
		Volumes:     newMockVolumeAccessor(),
		Filesystems: newMockFilesystemAccessor(),
		Life:        &mockLifecycleManager{},
		Registry:    s.registry,
		Machines:    newMockMachineAccessor(c),
		Status:      &mockStatusSetter{},
		Clock:       &mockClock{},
		Logger:      loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, jc.ErrorIsNil)

	worker.Kill()
	c.Assert(worker.Wait(), gc.IsNil)
}

func (s *storageProvisionerSuite) TestInvalidConfig(c *gc.C) {
	_, err := storageprovisioner.NewStorageProvisioner(almostValidConfig())
	c.Check(err, jc.ErrorIs, errors.NotValid)
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
	volumeAccessor.provisionedMachines["machine-1"] = "already-provisioned-1"
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
	volumeAttachmentPlansCreate := make(chan interface{})
	volumeAccessor.createVolumeAttachmentPlans = func(volumeAttachmentPlans []params.VolumeAttachmentPlan) ([]params.ErrorResult, error) {
		defer close(volumeAttachmentPlansCreate)
		return make([]params.ErrorResult, len(volumeAttachmentPlans)), nil
	}

	args := &workerArgs{volumes: volumeAccessor, registry: s.registry}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	volumeAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "machine-1", AttachmentTag: "volume-1",
	}, {
		MachineTag: "machine-1", AttachmentTag: "volume-2",
	}}
	assertNoEvent(c, volumeAttachmentPlansCreate, "volume attachment plans set")
	assertNoEvent(c, volumeAttachmentInfoSet, "volume attachment set")
	// The worker should create volumes according to ids "1" and "2".
	volumeAccessor.volumesWatcher.changes <- []string{"1", "2"}
	waitChannel(c, volumeInfoSet, "waiting for volume info to be set")
	waitChannel(c, volumeAttachmentPlansCreate, "waiting for volume attachment plans to be set")
	waitChannel(c, volumeAttachmentInfoSet, "waiting for volume attachments to be set")
}

func (s *storageProvisionerSuite) TestCreateVolumeCreatesAttachment(c *gc.C) {
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.provisionedMachines["machine-1"] = "already-provisioned-1"

	volumeAttachmentInfoSet := make(chan interface{})
	volumeAccessor.setVolumeAttachmentInfo = func(volumeAttachments []params.VolumeAttachment) ([]params.ErrorResult, error) {
		defer close(volumeAttachmentInfoSet)
		return make([]params.ErrorResult, len(volumeAttachments)), nil
	}
	volumeAttachmentPlansCreate := make(chan interface{})
	volumeAccessor.createVolumeAttachmentPlans = func(volumeAttachmentPlans []params.VolumeAttachmentPlan) ([]params.ErrorResult, error) {
		defer close(volumeAttachmentPlansCreate)
		return make([]params.ErrorResult, len(volumeAttachmentPlans)), nil
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

	args := &workerArgs{volumes: volumeAccessor, registry: s.registry}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	volumeAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "machine-1", AttachmentTag: "volume-1",
	}}
	assertNoEvent(c, volumeAttachmentInfoSet, "volume attachment set")

	// The worker should create volumes according to ids "1".
	volumeAccessor.volumesWatcher.changes <- []string{"1"}
	waitChannel(c, volumeAttachmentInfoSet, "waiting for volume attachments to be set")
	assertNoEvent(c, attachVolumesCalled, "AttachVolumes called")
}

func (s *storageProvisionerSuite) TestCreateVolumeRetry(c *gc.C) {
	volumeInfoSet := make(chan interface{})
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.provisionedMachines["machine-1"] = "already-provisioned-1"
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

	args := &workerArgs{volumes: volumeAccessor, clock: clock, registry: s.registry}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	volumeAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "machine-1", AttachmentTag: "volume-1",
	}}
	volumeAccessor.volumesWatcher.changes <- []string{"1"}
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

func (s *storageProvisionerSuite) TestCreateFilesystemRetry(c *gc.C) {
	filesystemInfoSet := make(chan interface{})
	filesystemAccessor := newMockFilesystemAccessor()
	filesystemAccessor.provisionedMachines["machine-1"] = "already-provisioned-1"
	filesystemAccessor.setFilesystemInfo = func(filesystems []params.Filesystem) ([]params.ErrorResult, error) {
		defer close(filesystemInfoSet)
		return make([]params.ErrorResult, len(filesystems)), nil
	}

	// mockFunc's After will progress the current time by the specified
	// duration and signal the channel immediately.
	clock := &mockClock{}
	var createFilesystemTimes []time.Time

	s.provider.createFilesystemsFunc = func(args []storage.FilesystemParams) ([]storage.CreateFilesystemsResult, error) {
		createFilesystemTimes = append(createFilesystemTimes, clock.Now())
		if len(createFilesystemTimes) < 10 {
			return []storage.CreateFilesystemsResult{{Error: errors.New("badness")}}, nil
		}
		return []storage.CreateFilesystemsResult{{
			Filesystem: &storage.Filesystem{Tag: args[0].Tag},
		}}, nil
	}

	args := &workerArgs{filesystems: filesystemAccessor, clock: clock, registry: s.registry}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	filesystemAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "machine-1", AttachmentTag: "filesystem-1",
	}}
	filesystemAccessor.filesystemsWatcher.changes <- []string{"1"}
	waitChannel(c, filesystemInfoSet, "waiting for filesystem info to be set")
	c.Assert(createFilesystemTimes, gc.HasLen, 10)

	// The first attempt should have been immediate: T0.
	c.Assert(createFilesystemTimes[0], gc.Equals, time.Time{})

	delays := make([]time.Duration, len(createFilesystemTimes)-1)
	for i := range createFilesystemTimes[1:] {
		delays[i] = createFilesystemTimes[i+1].Sub(createFilesystemTimes[i])
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
		{Tag: "filesystem-1", Status: "pending", Info: "badness"},
		{Tag: "filesystem-1", Status: "pending", Info: "badness"},
		{Tag: "filesystem-1", Status: "pending", Info: "badness"},
		{Tag: "filesystem-1", Status: "pending", Info: "badness"},
		{Tag: "filesystem-1", Status: "pending", Info: "badness"},
		{Tag: "filesystem-1", Status: "pending", Info: "badness"},
		{Tag: "filesystem-1", Status: "pending", Info: "badness"},
		{Tag: "filesystem-1", Status: "pending", Info: "badness"},
		{Tag: "filesystem-1", Status: "pending", Info: "badness"},
		{Tag: "filesystem-1", Status: "attaching", Info: ""},
	})
}

func (s *storageProvisionerSuite) TestFilesystemChannelReceivedOrder(c *gc.C) {
	alreadyAttached := params.MachineStorageId{
		MachineTag:    "machine-1",
		AttachmentTag: "filesystem-1",
	}
	fileSystem := params.Filesystem{
		FilesystemTag: "filesystem-1",
		Info: params.FilesystemInfo{
			FilesystemId: "1/1",
		},
	}

	filesystemAttachInfoSet := make(chan interface{})
	filesystemAccessor := newMockFilesystemAccessor()
	filesystemAccessor.provisionedMachines["machine-1"] = "already-provisioned-1"
	filesystemAccessor.provisionedFilesystems["filesystem-1"] = fileSystem
	filesystemAccessor.provisionedMachinesFilesystems["filesystem-1"] = fileSystem
	filesystemAccessor.provisionedAttachments[alreadyAttached] = params.FilesystemAttachment{
		MachineTag:    "machine-1",
		FilesystemTag: "filesystem-1",
		Info:          params.FilesystemAttachmentInfo{MountPoint: "/dev/sda1"},
	}
	filesystemAccessor.setFilesystemAttachmentInfo = func(attachments []params.FilesystemAttachment) ([]params.ErrorResult, error) {
		defer close(filesystemAttachInfoSet)
		return make([]params.ErrorResult, len(attachments)), nil
	}

	// mockFunc's After will progress the current time by the specified
	// duration and signal the channel immediately.
	clock := &mockClock{}
	s.provider.createFilesystemsFunc = func(args []storage.FilesystemParams) ([]storage.CreateFilesystemsResult, error) {
		return []storage.CreateFilesystemsResult{{
			Filesystem: &storage.Filesystem{Tag: args[0].Tag},
		}}, nil
	}

	life := func(tags []names.Tag) ([]params.LifeResult, error) {
		results := make([]params.LifeResult, len(tags))
		for i := range results {
			results[i].Life = life.Alive
		}
		return results, nil
	}

	args := &workerArgs{
		filesystems: filesystemAccessor,
		life: &mockLifecycleManager{
			life: life,
		},
		clock:    clock,
		registry: s.registry,
	}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	filesystemAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "machine-1", AttachmentTag: "filesystem-1",
	}}
	filesystemAccessor.filesystemsWatcher.changes <- []string{"1"}
	waitChannel(c, filesystemAttachInfoSet, "waiting for filesystem attach info to be set")

	c.Assert(args.statusSetter.args, jc.DeepEquals, []params.EntityStatusArgs{
		{Tag: "filesystem-1", Status: "attached", Info: ""},
	})
}

func (s *storageProvisionerSuite) TestAttachVolumeRetry(c *gc.C) {
	volumeInfoSet := make(chan interface{})
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.provisionedMachines["machine-1"] = "already-provisioned-1"
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
				Volume:  args[0].Volume,
				Machine: args[0].Machine,
				VolumeAttachmentInfo: storage.VolumeAttachmentInfo{
					DeviceName: "/dev/sda1",
				},
			},
		}}, nil
	}

	args := &workerArgs{volumes: volumeAccessor, clock: clock, registry: s.registry}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	volumeAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "machine-1", AttachmentTag: "volume-1",
	}}
	volumeAccessor.volumesWatcher.changes <- []string{"1"}
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

func (s *storageProvisionerSuite) TestAttachFilesystemRetry(c *gc.C) {
	filesystemInfoSet := make(chan interface{})
	filesystemAccessor := newMockFilesystemAccessor()
	filesystemAccessor.provisionedMachines["machine-1"] = "already-provisioned-1"
	filesystemAccessor.setFilesystemInfo = func(filesystems []params.Filesystem) ([]params.ErrorResult, error) {
		defer close(filesystemInfoSet)
		return make([]params.ErrorResult, len(filesystems)), nil
	}
	filesystemAttachmentInfoSet := make(chan interface{})
	filesystemAccessor.setFilesystemAttachmentInfo = func(filesystemAttachments []params.FilesystemAttachment) ([]params.ErrorResult, error) {
		defer close(filesystemAttachmentInfoSet)
		return make([]params.ErrorResult, len(filesystemAttachments)), nil
	}

	// mockFunc's After will progress the current time by the specified
	// duration and signal the channel immediately.
	clock := &mockClock{}
	var attachFilesystemTimes []time.Time

	s.provider.attachFilesystemsFunc = func(args []storage.FilesystemAttachmentParams) ([]storage.AttachFilesystemsResult, error) {
		attachFilesystemTimes = append(attachFilesystemTimes, clock.Now())
		if len(attachFilesystemTimes) < 10 {
			return []storage.AttachFilesystemsResult{{Error: errors.New("badness")}}, nil
		}
		return []storage.AttachFilesystemsResult{{
			FilesystemAttachment: &storage.FilesystemAttachment{
				Filesystem: args[0].Filesystem,
				Machine:    args[0].Machine,
				FilesystemAttachmentInfo: storage.FilesystemAttachmentInfo{
					Path: "/oh/over/there",
				},
			},
		}}, nil
	}

	args := &workerArgs{filesystems: filesystemAccessor, clock: clock, registry: s.registry}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	filesystemAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "machine-1", AttachmentTag: "filesystem-1",
	}}
	filesystemAccessor.filesystemsWatcher.changes <- []string{"1"}
	waitChannel(c, filesystemInfoSet, "waiting for filesystem info to be set")
	waitChannel(c, filesystemAttachmentInfoSet, "waiting for filesystem attachments to be set")
	c.Assert(attachFilesystemTimes, gc.HasLen, 10)

	// The first attempt should have been immediate: T0.
	c.Assert(attachFilesystemTimes[0], gc.Equals, time.Time{})

	delays := make([]time.Duration, len(attachFilesystemTimes)-1)
	for i := range attachFilesystemTimes[1:] {
		delays[i] = attachFilesystemTimes[i+1].Sub(attachFilesystemTimes[i])
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
		{Tag: "filesystem-1", Status: "attaching", Info: ""},        // CreateFilesystems
		{Tag: "filesystem-1", Status: "attaching", Info: "badness"}, // AttachFilesystems
		{Tag: "filesystem-1", Status: "attaching", Info: "badness"},
		{Tag: "filesystem-1", Status: "attaching", Info: "badness"},
		{Tag: "filesystem-1", Status: "attaching", Info: "badness"},
		{Tag: "filesystem-1", Status: "attaching", Info: "badness"},
		{Tag: "filesystem-1", Status: "attaching", Info: "badness"},
		{Tag: "filesystem-1", Status: "attaching", Info: "badness"},
		{Tag: "filesystem-1", Status: "attaching", Info: "badness"},
		{Tag: "filesystem-1", Status: "attaching", Info: "badness"},
		{Tag: "filesystem-1", Status: "attached", Info: ""},
	})
}

func (s *storageProvisionerSuite) TestValidateVolumeParams(c *gc.C) {
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.provisionedMachines["machine-1"] = "already-provisioned-1"
	volumeAccessor.provisionedVolumes["volume-3"] = params.Volume{
		VolumeTag: "volume-3",
		Info:      params.VolumeInfo{VolumeId: "vol-ume"},
	}

	var validateCalls int
	validated := make(chan interface{}, 1)
	s.provider.validateVolumeParamsFunc = func(p storage.VolumeParams) error {
		validateCalls++
		validated <- p
		switch p.Tag.String() {
		case "volume-1", "volume-3":
			return errors.New("something is wrong")
		}
		return nil
	}

	life := func(tags []names.Tag) ([]params.LifeResult, error) {
		results := make([]params.LifeResult, len(tags))
		for i := range results {
			switch tags[i].String() {
			case "volume-3":
				results[i].Life = life.Dead
			default:
				results[i].Life = life.Alive
			}
		}
		return results, nil
	}

	createdVolumes := make(chan interface{}, 1)
	s.provider.createVolumesFunc = func(args []storage.VolumeParams) ([]storage.CreateVolumesResult, error) {
		createdVolumes <- args
		if len(args) != 1 {
			return nil, errors.New("expected one argument")
		}
		return []storage.CreateVolumesResult{{
			Volume: &storage.Volume{Tag: args[0].Tag},
		}}, nil
	}

	destroyedVolumes := make(chan interface{}, 1)
	s.provider.destroyVolumesFunc = func(volumeIds []string) ([]error, error) {
		destroyedVolumes <- volumeIds
		return make([]error, len(volumeIds)), nil
	}

	args := &workerArgs{
		volumes: volumeAccessor,
		life: &mockLifecycleManager{
			life: life,
		},
		registry: s.registry,
	}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	volumeAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "machine-1", AttachmentTag: "volume-1",
	}, {
		MachineTag: "machine-1", AttachmentTag: "volume-2",
	}}
	volumeAccessor.volumesWatcher.changes <- []string{"1"}
	waitChannel(c, validated, "waiting for volume parameter validation")
	assertNoEvent(c, createdVolumes, "volume created")
	c.Assert(validateCalls, gc.Equals, 1)

	// Failure to create volume-1 should not block creation volume-2.
	volumeAccessor.volumesWatcher.changes <- []string{"2"}
	waitChannel(c, validated, "waiting for volume parameter validation")
	createVolumeParams := waitChannel(c, createdVolumes, "volume created").([]storage.VolumeParams)
	c.Assert(createVolumeParams, gc.HasLen, 1)
	c.Assert(createVolumeParams[0].Tag.String(), gc.Equals, "volume-2")
	c.Assert(validateCalls, gc.Equals, 2)

	// destroying filesystems does not validate parameters
	volumeAccessor.volumesWatcher.changes <- []string{"3"}
	assertNoEvent(c, validated, "volume destruction params validated")
	destroyVolumeParams := waitChannel(c, destroyedVolumes, "volume destroyed").([]string)
	c.Assert(destroyVolumeParams, jc.DeepEquals, []string{"vol-ume"})
	c.Assert(validateCalls, gc.Equals, 2) // no change

	c.Assert(args.statusSetter.args, jc.DeepEquals, []params.EntityStatusArgs{
		{Tag: "volume-1", Status: "error", Info: "something is wrong"},
		{Tag: "volume-2", Status: "attaching"},
		// destroyed volumes are removed immediately,
		// so there is no status update.
	})
}

func (s *storageProvisionerSuite) TestValidateFilesystemParams(c *gc.C) {
	filesystemAccessor := newMockFilesystemAccessor()
	filesystemAccessor.provisionedMachines["machine-1"] = "already-provisioned-1"
	filesystemAccessor.provisionedFilesystems["filesystem-3"] = params.Filesystem{
		FilesystemTag: "filesystem-3",
		Info:          params.FilesystemInfo{FilesystemId: "fs-id"},
	}

	var validateCalls int
	validated := make(chan interface{}, 1)
	s.provider.validateFilesystemParamsFunc = func(p storage.FilesystemParams) error {
		validateCalls++
		validated <- p
		switch p.Tag.String() {
		case "filesystem-1", "filesystem-3":
			return errors.New("something is wrong")
		}
		return nil
	}

	life := func(tags []names.Tag) ([]params.LifeResult, error) {
		results := make([]params.LifeResult, len(tags))
		for i := range results {
			switch tags[i].String() {
			case "filesystem-3":
				results[i].Life = life.Dead
			default:
				results[i].Life = life.Alive
			}
		}
		return results, nil
	}

	createdFilesystems := make(chan interface{}, 1)
	s.provider.createFilesystemsFunc = func(args []storage.FilesystemParams) ([]storage.CreateFilesystemsResult, error) {
		createdFilesystems <- args
		if len(args) != 1 {
			return nil, errors.New("expected one argument")
		}
		return []storage.CreateFilesystemsResult{{
			Filesystem: &storage.Filesystem{Tag: args[0].Tag},
		}}, nil
	}

	destroyedFilesystems := make(chan interface{}, 1)
	s.provider.destroyFilesystemsFunc = func(filesystemIds []string) ([]error, error) {
		destroyedFilesystems <- filesystemIds
		return make([]error, len(filesystemIds)), nil
	}

	args := &workerArgs{
		filesystems: filesystemAccessor,
		life: &mockLifecycleManager{
			life: life,
		},
		registry: s.registry,
	}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	filesystemAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "machine-1", AttachmentTag: "filesystem-1",
	}, {
		MachineTag: "machine-1", AttachmentTag: "filesystem-2",
	}}
	filesystemAccessor.filesystemsWatcher.changes <- []string{"1"}
	waitChannel(c, validated, "waiting for filesystem parameter validation")
	assertNoEvent(c, createdFilesystems, "filesystem created")
	c.Assert(validateCalls, gc.Equals, 1)

	// Failure to create filesystem-1 should not block creation filesystem-2.
	filesystemAccessor.filesystemsWatcher.changes <- []string{"2"}
	waitChannel(c, validated, "waiting for filesystem parameter validation")
	createFilesystemParams := waitChannel(c, createdFilesystems, "filesystem created").([]storage.FilesystemParams)
	c.Assert(createFilesystemParams, gc.HasLen, 1)
	c.Assert(createFilesystemParams[0].Tag.String(), gc.Equals, "filesystem-2")
	c.Assert(validateCalls, gc.Equals, 2)

	// destroying filesystems does not validate parameters
	filesystemAccessor.filesystemsWatcher.changes <- []string{"3"}
	assertNoEvent(c, validated, "filesystem destruction params validated")
	destroyFilesystemParams := waitChannel(c, destroyedFilesystems, "filesystem destroyed").([]string)
	c.Assert(destroyFilesystemParams, jc.DeepEquals, []string{"fs-id"})
	c.Assert(validateCalls, gc.Equals, 2) // no change

	c.Assert(args.statusSetter.args, jc.DeepEquals, []params.EntityStatusArgs{
		{Tag: "filesystem-1", Status: "error", Info: "something is wrong"},
		{Tag: "filesystem-2", Status: "attaching"},
		// destroyed filesystems are removed immediately,
		// so there is no status update.
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

	args := &workerArgs{filesystems: filesystemAccessor, registry: s.registry}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	// The worker should create filesystems according to ids "1" and "2".
	filesystemAccessor.filesystemsWatcher.changes <- []string{"1", "2"}
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

	args := &workerArgs{volumes: volumeAccessor, registry: s.registry}
	worker := newStorageProvisioner(c, args)
	defer worker.Wait()
	defer worker.Kill()

	volumeAccessor.volumesWatcher.changes <- []string{needsInstanceVolumeId}
	assertNoEvent(c, volumeInfoSet, "volume info set")
	args.machines.instanceIds[names.NewMachineTag("1")] = "inst-id"
	args.machines.watcher.changes <- struct{}{}
	waitChannel(c, volumeInfoSet, "waiting for volume info to be set")
}

// TestVolumeIncoherent tests that we do not panic when observing
// a pending volume that has no attachments. We send a volume
// update for a volume that is alive and unprovisioned, but has
// no machine attachment. Such volumes are ignored by the storage
// provisioner.
//
// See: https://bugs.launchpad.net/juju/+bug/1732616
func (s *storageProvisionerSuite) TestVolumeIncoherent(c *gc.C) {
	volumeAccessor := newMockVolumeAccessor()
	args := &workerArgs{volumes: volumeAccessor, registry: s.registry}
	worker := newStorageProvisioner(c, args)
	defer workertest.CleanKill(c, worker)

	// Send 3 times, because the channel has a buffer size of 1.
	// The third send guarantees we've sent at least the 2nd one
	// through, which means at least the 1st has been processed
	// (and ignored).
	for i := 0; i < 3; i++ {
		volumeAccessor.volumesWatcher.changes <- []string{noAttachmentVolumeId}
	}
}

func (s *storageProvisionerSuite) TestVolumeNonDynamic(c *gc.C) {
	volumeInfoSet := make(chan interface{})
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.setVolumeInfo = func([]params.Volume) ([]params.ErrorResult, error) {
		defer close(volumeInfoSet)
		return nil, nil
	}

	args := &workerArgs{volumes: volumeAccessor, registry: s.registry}
	worker := newStorageProvisioner(c, args)
	defer worker.Wait()
	defer worker.Kill()

	// Volumes for non-dynamic providers should not be created.
	s.provider.dynamic = false
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
	volumeAccessor.provisionedMachines["machine-0"] = "already-provisioned-0"
	volumeAccessor.provisionedMachines["machine-1"] = "already-provisioned-1"

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

	args := &workerArgs{volumes: volumeAccessor, registry: s.registry}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	volumeAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
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
	waitChannel(c, volumeAttachmentInfoSet, "waiting for volume attachments to be set")
	c.Assert(allVolumeAttachments, jc.SameContents, expectedVolumeAttachments)

	// Reattachment should only happen once per session.
	volumeAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag:    "machine-0",
		AttachmentTag: "volume-1",
	}}
	assertNoEvent(c, volumeAttachmentInfoSet, "volume attachment info set")
}

func (s *storageProvisionerSuite) TestVolumeAttachmentNoStaticReattachment(c *gc.C) {
	// Static storage should never be reattached.
	s.provider.dynamic = false

	volumeAttachmentInfoSet := make(chan interface{})
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.setVolumeAttachmentInfo = func(volumeAttachments []params.VolumeAttachment) ([]params.ErrorResult, error) {
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
	volumeAccessor.provisionedMachines["machine-0"] = "already-provisioned-0"
	volumeAccessor.provisionedMachines["machine-1"] = "already-provisioned-1"

	alreadyAttached := params.MachineStorageId{
		MachineTag:    "machine-0",
		AttachmentTag: "volume-1",
	}
	volumeAccessor.provisionedAttachments[alreadyAttached] = params.VolumeAttachment{
		MachineTag: "machine-0",
		VolumeTag:  "volume-1",
	}

	args := &workerArgs{volumes: volumeAccessor, registry: s.registry}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	volumeAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "machine-0", AttachmentTag: "volume-1",
	}}
	volumeAccessor.volumesWatcher.changes <- []string{"1"}
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
	filesystemAccessor.provisionedMachines["machine-0"] = "already-provisioned-0"
	filesystemAccessor.provisionedMachines["machine-1"] = "already-provisioned-1"

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

	args := &workerArgs{filesystems: filesystemAccessor, registry: s.registry}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	filesystemAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "machine-1", AttachmentTag: "filesystem-1",
	}, {
		MachineTag: "machine-1", AttachmentTag: "filesystem-2",
	}, {
		MachineTag: "machine-2", AttachmentTag: "filesystem-1",
	}, {
		MachineTag: "machine-0", AttachmentTag: "filesystem-1",
	}}
	assertNoEvent(c, filesystemAttachmentInfoSet, "filesystem attachment info set")
	filesystemAccessor.filesystemsWatcher.changes <- []string{"1"}
	waitChannel(c, filesystemAttachmentInfoSet, "waiting for filesystem attachments to be set")
	c.Assert(allFilesystemAttachments, jc.SameContents, expectedFilesystemAttachments)

	// Reattachment should only happen once per session.
	filesystemAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag:    "machine-0",
		AttachmentTag: "filesystem-1",
	}}
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
		registry:    s.registry,
	}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	args.volumes.blockDevices[params.MachineStorageId{
		MachineTag:    "machine-0",
		AttachmentTag: "volume-0-0",
	}] = params.BlockDevice{
		DeviceName: "xvdf1",
		Size:       123,
	}
	filesystemAccessor.filesystemsWatcher.changes <- []string{"0/0", "0/1"}

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
	}] = params.BlockDevice{
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
		return make([]params.ErrorResult, len(attachments)), nil
	}

	args := &workerArgs{
		scope:       names.NewMachineTag("0"),
		filesystems: filesystemAccessor,
		registry:    s.registry,
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
	filesystemAccessor.provisionedMachines["machine-0"] = "already-provisioned-0"

	args.volumes.blockDevices[params.MachineStorageId{
		MachineTag:    "machine-0",
		AttachmentTag: "volume-0-0",
	}] = params.BlockDevice{
		DeviceName: "xvdf1",
		Size:       123,
	}
	filesystemAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag:    "machine-0",
		AttachmentTag: "filesystem-0-0",
	}}
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

	// Update the UUID of the block device and check attachment update.
	args.volumes.blockDevices[params.MachineStorageId{
		MachineTag:    "machine-0",
		AttachmentTag: "volume-0-0",
	}] = params.BlockDevice{
		DeviceName: "xvdf1",
		Size:       123,
		UUID:       "deadbeaf",
	}
	s.managedFilesystemSource.attachedFilesystems = make(chan interface{}, 1)
	args.volumes.blockDevicesWatcher.changes <- struct{}{}
	attachInfo := waitChannel(
		c, s.managedFilesystemSource.attachedFilesystems,
		"waiting for filesystem attachements",
	).([]storage.AttachFilesystemsResult)
	c.Assert(attachInfo, jc.DeepEquals, []storage.AttachFilesystemsResult{{
		FilesystemAttachment: &storage.FilesystemAttachment{
			Filesystem: names.NewFilesystemTag("0/0"),
			FilesystemAttachmentInfo: storage.FilesystemAttachmentInfo{
				Path:     "/mnt/xvdf1",
				ReadOnly: true,
			},
		},
	}})

}

func (s *storageProvisionerSuite) TestResourceTags(c *gc.C) {
	volumeInfoSet := make(chan interface{})
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.provisionedMachines["machine-1"] = "already-provisioned-1"
	volumeAccessor.setVolumeInfo = func(volumes []params.Volume) ([]params.ErrorResult, error) {
		defer close(volumeInfoSet)
		return nil, nil
	}

	filesystemInfoSet := make(chan interface{})
	filesystemAccessor := newMockFilesystemAccessor()
	filesystemAccessor.provisionedMachines["machine-1"] = "already-provisioned-1"
	filesystemAccessor.setFilesystemInfo = func(filesystems []params.Filesystem) ([]params.ErrorResult, error) {
		defer close(filesystemInfoSet)
		return nil, nil
	}

	var volumeSource dummyVolumeSource
	s.provider.volumeSourceFunc = func(sourceConfig *storage.Config) (storage.VolumeSource, error) {
		return &volumeSource, nil
	}

	var filesystemSource dummyFilesystemSource
	s.provider.filesystemSourceFunc = func(sourceConfig *storage.Config) (storage.FilesystemSource, error) {
		return &filesystemSource, nil
	}

	args := &workerArgs{
		volumes:     volumeAccessor,
		filesystems: filesystemAccessor,
		registry:    s.registry,
	}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	volumeAccessor.volumesWatcher.changes <- []string{"1"}
	filesystemAccessor.filesystemsWatcher.changes <- []string{"1"}
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
	volumeAccessor.provisionedMachines["machine-1"] = "already-provisioned-1"
	volumeAccessor.setVolumeInfo = func(volumes []params.Volume) ([]params.ErrorResult, error) {
		return nil, errors.New("belly up")
	}

	args := &workerArgs{volumes: volumeAccessor, registry: s.registry}
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
	waitChannel(c, done, "waiting for worker to exit")
}

func (s *storageProvisionerSuite) TestSetVolumeInfoErrorResultDoesNotStopWorker(c *gc.C) {
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.provisionedMachines["machine-1"] = "already-provisioned-1"
	volumeAccessor.setVolumeInfo = func(volumes []params.Volume) ([]params.ErrorResult, error) {
		return []params.ErrorResult{{Error: &params.Error{Message: "message", Code: "code"}}}, nil
	}

	args := &workerArgs{volumes: volumeAccessor, registry: s.registry}
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
		life:     &mockLifecycleManager{removeAttachments: removeAttachments},
		registry: s.registry,
	}
	worker := newStorageProvisioner(c, args)
	defer worker.Wait()
	defer worker.Kill()

	args.volumes.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "machine-0", AttachmentTag: "volume-0",
	}}
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
		value := life.Alive
		if attached {
			value = life.Dying
		}
		return []params.LifeResult{{Life: value}}, nil
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
	volumeAccessor.provisionedMachines["machine-1"] = "already-provisioned-1"

	args := &workerArgs{
		volumes: volumeAccessor,
		life: &mockLifecycleManager{
			attachmentLife:    attachmentLife,
			removeAttachments: removeAttachments,
		},
		registry: s.registry,
	}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	volumeAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "machine-1", AttachmentTag: "volume-1",
	}}
	volumeAccessor.volumesWatcher.changes <- []string{"1"}
	waitChannel(c, volumeAttachmentInfoSet, "waiting for volume attachments to be set")
	volumeAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "machine-1", AttachmentTag: "volume-1",
	}}
	waitChannel(c, detached, "waiting for volume to be detached")
	waitChannel(c, removed, "waiting for attachment to be removed")
}

func (s *storageProvisionerSuite) TestDetachVolumesRetry(c *gc.C) {
	machine := names.NewMachineTag("1")
	volume := names.NewVolumeTag("1")
	attachmentId := params.MachineStorageId{
		MachineTag:    machine.String(),
		AttachmentTag: volume.String(),
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
	volumeAccessor.provisionedMachines[machine.String()] = "already-provisioned-1"

	attachmentLife := func(ids []params.MachineStorageId) ([]params.LifeResult, error) {
		return []params.LifeResult{{Life: life.Dying}}, nil
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
		registry: s.registry,
	}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	volumeAccessor.volumesWatcher.changes <- []string{volume.Id()}
	volumeAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag:    machine.String(),
		AttachmentTag: volume.String(),
	}}
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

func (s *storageProvisionerSuite) TestDetachVolumesNotFound(c *gc.C) {
	// This test just checks that there are no unexpected api calls
	// if a volume attachment is deleted from state.
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
		value := life.Alive
		var lifeErr *params.Error
		if attached {
			lifeErr = &params.Error{Code: params.CodeNotFound}
		}
		return []params.LifeResult{{Life: value, Error: lifeErr}}, nil
	}

	s.provider.detachVolumesFunc = func(args []storage.VolumeAttachmentParams) ([]error, error) {
		c.Fatalf("unexpected call to detachVolumes")
		return nil, nil
	}
	s.provider.destroyVolumesFunc = func(ids []string) ([]error, error) {
		c.Fatalf("unexpected call to destroyVolumes")
		return nil, nil
	}

	removeAttachments := func(ids []params.MachineStorageId) ([]params.ErrorResult, error) {
		c.Fatalf("unexpected call to removeAttachments")
		return nil, nil
	}

	// volume-1 and machine-1 are provisioned.
	volumeAccessor.provisionedVolumes["volume-1"] = params.Volume{
		VolumeTag: "volume-1",
		Info: params.VolumeInfo{
			VolumeId: "vol-123",
		},
	}
	volumeAccessor.provisionedMachines["machine-1"] = "already-provisioned-1"

	args := &workerArgs{
		volumes: volumeAccessor,
		life: &mockLifecycleManager{
			attachmentLife:    attachmentLife,
			removeAttachments: removeAttachments,
		},
		registry: s.registry,
	}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()

	volumeAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "machine-1", AttachmentTag: "volume-1",
	}}
	volumeAccessor.volumesWatcher.changes <- []string{"1"}
	waitChannel(c, volumeAttachmentInfoSet, "waiting for volume attachments to be set")

	// This results in a not found attachment.
	volumeAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "machine-1", AttachmentTag: "volume-1",
	}}
	workertest.CleanKill(c, worker)
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
		life:     &mockLifecycleManager{removeAttachments: removeAttachments},
		registry: s.registry,
	}
	worker := newStorageProvisioner(c, args)
	defer worker.Wait()
	defer worker.Kill()

	args.filesystems.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "machine-0", AttachmentTag: "filesystem-0",
	}}
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
		value := life.Alive
		if attached {
			value = life.Dying
		}
		return []params.LifeResult{{Life: value}}, nil
	}

	detached := make(chan interface{})
	s.provider.detachFilesystemsFunc = func(args []storage.FilesystemAttachmentParams) ([]error, error) {
		c.Assert(args, gc.HasLen, 1)
		c.Assert(args[0].Machine.String(), gc.Equals, expectedAttachmentIds[0].MachineTag)
		c.Assert(args[0].Filesystem.String(), gc.Equals, expectedAttachmentIds[0].AttachmentTag)
		defer close(detached)
		return make([]error, len(args)), nil
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
	filesystemAccessor.provisionedMachines["machine-1"] = "already-provisioned-1"

	args := &workerArgs{
		filesystems: filesystemAccessor,
		life: &mockLifecycleManager{
			attachmentLife:    attachmentLife,
			removeAttachments: removeAttachments,
		},
		registry: s.registry,
	}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	filesystemAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "machine-1", AttachmentTag: "filesystem-1",
	}}
	filesystemAccessor.filesystemsWatcher.changes <- []string{"1"}
	waitChannel(c, filesystemAttachmentInfoSet, "waiting for filesystem attachments to be set")
	filesystemAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "machine-1", AttachmentTag: "filesystem-1",
	}}
	waitChannel(c, detached, "waiting for filesystem to be detached")
	waitChannel(c, removed, "waiting for attachment to be removed")
}

func (s *storageProvisionerSuite) TestDetachFilesystemsNotFound(c *gc.C) {
	// This test just checks that there are no unexpected api calls
	// if a volume attachment is deleted from state.
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
		value := life.Alive
		var lifeErr *params.Error
		if attached {
			lifeErr = &params.Error{Code: params.CodeNotFound}
		}
		return []params.LifeResult{{Life: value, Error: lifeErr}}, nil
	}

	s.provider.detachFilesystemsFunc = func(args []storage.FilesystemAttachmentParams) ([]error, error) {
		c.Fatalf("unexpected call to detachFilesystems")
		return nil, nil
	}
	s.provider.destroyFilesystemsFunc = func(ids []string) ([]error, error) {
		c.Fatalf("unexpected call to destroyFilesystems")
		return nil, nil
	}

	removeAttachments := func(ids []params.MachineStorageId) ([]params.ErrorResult, error) {
		c.Fatalf("unexpected call to removeAttachments")
		return nil, nil
	}

	// filesystem-1 and machine-1 are provisioned.
	filesystemAccessor.provisionedFilesystems["filesystem-1"] = params.Filesystem{
		FilesystemTag: "filesystem-1",
		Info: params.FilesystemInfo{
			FilesystemId: "fs-id",
		},
	}
	filesystemAccessor.provisionedMachines["machine-1"] = "already-provisioned-1"

	args := &workerArgs{
		filesystems: filesystemAccessor,
		life: &mockLifecycleManager{
			attachmentLife:    attachmentLife,
			removeAttachments: removeAttachments,
		},
		registry: s.registry,
	}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()

	filesystemAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "machine-1", AttachmentTag: "filesystem-1",
	}}
	filesystemAccessor.filesystemsWatcher.changes <- []string{"1"}
	waitChannel(c, filesystemAttachmentInfoSet, "waiting for filesystem attachments to be set")

	// This results in a not found attachment.
	filesystemAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "machine-1", AttachmentTag: "filesystem-1",
	}}
	workertest.CleanKill(c, worker)
}

func (s *storageProvisionerSuite) TestDestroyVolumes(c *gc.C) {
	unprovisionedVolume := names.NewVolumeTag("0")
	provisionedDestroyVolume := names.NewVolumeTag("1")
	provisionedReleaseVolume := names.NewVolumeTag("2")

	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.provisionVolume(provisionedDestroyVolume)
	volumeAccessor.provisionVolume(provisionedReleaseVolume)

	life := func(tags []names.Tag) ([]params.LifeResult, error) {
		results := make([]params.LifeResult, len(tags))
		for i := range results {
			results[i].Life = life.Dead
		}
		return results, nil
	}

	destroyedChan := make(chan interface{}, 1)
	s.provider.destroyVolumesFunc = func(volumeIds []string) ([]error, error) {
		destroyedChan <- volumeIds
		return make([]error, len(volumeIds)), nil
	}

	releasedChan := make(chan interface{}, 1)
	s.provider.releaseVolumesFunc = func(volumeIds []string) ([]error, error) {
		releasedChan <- volumeIds
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
		registry: s.registry,
	}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	volumeAccessor.volumesWatcher.changes <- []string{
		unprovisionedVolume.Id(),
		provisionedDestroyVolume.Id(),
		provisionedReleaseVolume.Id(),
	}

	// All volumes should be removed; the provisioned ones
	// should be destroyed/released first.

	destroyed := waitChannel(c, destroyedChan, "waiting for volume to be destroyed")
	assertNoEvent(c, destroyedChan, "volumes destroyed")
	c.Assert(destroyed, jc.DeepEquals, []string{"vol-1"})

	released := waitChannel(c, releasedChan, "waiting for volume to be released")
	assertNoEvent(c, releasedChan, "volumes released")
	c.Assert(released, jc.DeepEquals, []string{"vol-2"})

	var removed []names.Tag
	for len(removed) < 3 {
		tags := waitChannel(c, removedChan, "waiting for volumes to be removed").([]names.Tag)
		removed = append(removed, tags...)
	}
	c.Assert(removed, jc.SameContents, []names.Tag{
		unprovisionedVolume,
		provisionedDestroyVolume,
		provisionedReleaseVolume,
	})
	assertNoEvent(c, removedChan, "volumes removed")
}

func (s *storageProvisionerSuite) TestDestroyVolumesRetry(c *gc.C) {
	volume := names.NewVolumeTag("1")
	volumeAccessor := newMockVolumeAccessor()
	volumeAccessor.provisionVolume(volume)

	life := func(tags []names.Tag) ([]params.LifeResult, error) {
		return []params.LifeResult{{Life: life.Dead}}, nil
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
		registry: s.registry,
	}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	volumeAccessor.volumesWatcher.changes <- []string{volume.Id()}
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
		{Tag: "volume-1", Status: "error", Info: "destroying volume: badness"},
		{Tag: "volume-1", Status: "error", Info: "destroying volume: badness"},
		{Tag: "volume-1", Status: "error", Info: "destroying volume: badness"},
		{Tag: "volume-1", Status: "error", Info: "destroying volume: badness"},
		{Tag: "volume-1", Status: "error", Info: "destroying volume: badness"},
		{Tag: "volume-1", Status: "error", Info: "destroying volume: badness"},
		{Tag: "volume-1", Status: "error", Info: "destroying volume: badness"},
		{Tag: "volume-1", Status: "error", Info: "destroying volume: badness"},
		{Tag: "volume-1", Status: "error", Info: "destroying volume: badness"},
	})
}

func (s *storageProvisionerSuite) TestDestroyFilesystems(c *gc.C) {
	unprovisionedFilesystem := names.NewFilesystemTag("0")
	provisionedDestroyFilesystem := names.NewFilesystemTag("1")
	provisionedReleaseFilesystem := names.NewFilesystemTag("2")

	filesystemAccessor := newMockFilesystemAccessor()
	filesystemAccessor.provisionFilesystem(provisionedDestroyFilesystem)
	filesystemAccessor.provisionFilesystem(provisionedReleaseFilesystem)

	life := func(tags []names.Tag) ([]params.LifeResult, error) {
		results := make([]params.LifeResult, len(tags))
		for i := range results {
			results[i].Life = life.Dead
		}
		return results, nil
	}

	destroyedChan := make(chan interface{}, 1)
	s.provider.destroyFilesystemsFunc = func(filesystemIds []string) ([]error, error) {
		destroyedChan <- filesystemIds
		return make([]error, len(filesystemIds)), nil
	}

	releasedChan := make(chan interface{}, 1)
	s.provider.releaseFilesystemsFunc = func(filesystemIds []string) ([]error, error) {
		releasedChan <- filesystemIds
		return make([]error, len(filesystemIds)), nil
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
		registry: s.registry,
	}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	filesystemAccessor.filesystemsWatcher.changes <- []string{
		unprovisionedFilesystem.Id(),
		provisionedDestroyFilesystem.Id(),
		provisionedReleaseFilesystem.Id(),
	}

	// Both filesystems should be removed; the provisioned ones
	// should be destroyed/released first.

	destroyed := waitChannel(c, destroyedChan, "waiting for filesystem to be destroyed")
	assertNoEvent(c, destroyedChan, "filesystems destroyed")
	c.Assert(destroyed, jc.DeepEquals, []string{"fs-1"})

	released := waitChannel(c, releasedChan, "waiting for filesystem to be released")
	assertNoEvent(c, releasedChan, "filesystems released")
	c.Assert(released, jc.DeepEquals, []string{"fs-2"})

	var removed []names.Tag
	for len(removed) < 3 {
		tags := waitChannel(c, removedChan, "waiting for filesystems to be removed").([]names.Tag)
		removed = append(removed, tags...)
	}
	c.Assert(removed, jc.SameContents, []names.Tag{
		unprovisionedFilesystem,
		provisionedDestroyFilesystem,
		provisionedReleaseFilesystem,
	})
	assertNoEvent(c, removedChan, "filesystems removed")
}

func (s *storageProvisionerSuite) TestDestroyFilesystemsRetry(c *gc.C) {
	provisionedDestroyFilesystem := names.NewFilesystemTag("0")

	filesystemAccessor := newMockFilesystemAccessor()
	filesystemAccessor.provisionFilesystem(provisionedDestroyFilesystem)

	life := func(tags []names.Tag) ([]params.LifeResult, error) {
		return []params.LifeResult{{Life: life.Dead}}, nil
	}

	// mockFunc's After will progress the current time by the specified
	// duration and signal the channel immediately.
	clock := &mockClock{}
	var destroyFilesystemTimes []time.Time
	s.provider.destroyFilesystemsFunc = func(filesystemIds []string) ([]error, error) {
		destroyFilesystemTimes = append(destroyFilesystemTimes, clock.Now())
		if len(destroyFilesystemTimes) < 10 {
			return []error{errors.New("destroyFilesystems failed, please retry later")}, nil
		}
		return []error{nil}, nil
	}

	removedChan := make(chan interface{}, 1)
	remove := func(tags []names.Tag) ([]params.ErrorResult, error) {
		removedChan <- tags
		return make([]params.ErrorResult, len(tags)), nil
	}

	args := &workerArgs{
		filesystems: filesystemAccessor,
		clock:       clock,
		life: &mockLifecycleManager{
			life:   life,
			remove: remove,
		},
		registry: s.registry,
	}
	worker := newStorageProvisioner(c, args)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	filesystemAccessor.filesystemsWatcher.changes <- []string{
		provisionedDestroyFilesystem.Id(),
	}

	waitChannel(c, removedChan, "waiting for filesystem to be removed")
	c.Assert(destroyFilesystemTimes, gc.HasLen, 10)

	// The first attempt should have been immediate: T0.
	c.Assert(destroyFilesystemTimes[0], gc.Equals, time.Time{})

	delays := make([]time.Duration, len(destroyFilesystemTimes)-1)
	for i := range destroyFilesystemTimes[1:] {
		delays[i] = destroyFilesystemTimes[i+1].Sub(destroyFilesystemTimes[i])
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
		{Tag: "filesystem-0", Status: "error", Info: "removing filesystem: destroyFilesystems failed, please retry later"},
		{Tag: "filesystem-0", Status: "error", Info: "removing filesystem: destroyFilesystems failed, please retry later"},
		{Tag: "filesystem-0", Status: "error", Info: "removing filesystem: destroyFilesystems failed, please retry later"},
		{Tag: "filesystem-0", Status: "error", Info: "removing filesystem: destroyFilesystems failed, please retry later"},
		{Tag: "filesystem-0", Status: "error", Info: "removing filesystem: destroyFilesystems failed, please retry later"},
		{Tag: "filesystem-0", Status: "error", Info: "removing filesystem: destroyFilesystems failed, please retry later"},
		{Tag: "filesystem-0", Status: "error", Info: "removing filesystem: destroyFilesystems failed, please retry later"},
		{Tag: "filesystem-0", Status: "error", Info: "removing filesystem: destroyFilesystems failed, please retry later"},
		{Tag: "filesystem-0", Status: "error", Info: "removing filesystem: destroyFilesystems failed, please retry later"},
	})
}

type caasStorageProvisionerSuite struct {
	coretesting.BaseSuite
	provider *dummyProvider
	registry storage.ProviderRegistry
}

var _ = gc.Suite(&caasStorageProvisionerSuite{})

func (s *caasStorageProvisionerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.provider = &dummyProvider{dynamic: true}
	s.registry = storage.StaticProviderRegistry{
		map[storage.ProviderType]storage.Provider{
			"dummy": s.provider,
		},
	}
	s.PatchValue(storageprovisioner.DefaultDependentChangesTimeout, 10*time.Millisecond)
}

func (s *caasStorageProvisionerSuite) TestDetachVolumesUnattached(c *gc.C) {
	removed := make(chan interface{})
	removeAttachments := func(ids []params.MachineStorageId) ([]params.ErrorResult, error) {
		defer close(removed)
		c.Assert(ids, gc.DeepEquals, []params.MachineStorageId{{
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "volume-0",
		}})
		return make([]params.ErrorResult, len(ids)), nil
	}

	args := &workerArgs{
		life:     &mockLifecycleManager{removeAttachments: removeAttachments},
		registry: s.registry,
	}
	w := newStorageProvisioner(c, args)
	defer w.Wait()
	defer w.Kill()

	args.volumes.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "unit-mariadb-0", AttachmentTag: "volume-0",
	}}
	waitChannel(c, removed, "waiting for attachment to be removed")
}

func (s *caasStorageProvisionerSuite) TestDetachVolumes(c *gc.C) {
	volumeAccessor := newMockVolumeAccessor()

	expectedAttachmentIds := []params.MachineStorageId{{
		MachineTag: "unit-mariadb-1", AttachmentTag: "volume-1",
	}}

	attachmentLife := func(ids []params.MachineStorageId) ([]params.LifeResult, error) {
		return []params.LifeResult{{Life: life.Dying}}, nil
	}

	detached := make(chan interface{})
	s.provider.detachVolumesFunc = func(args []storage.VolumeAttachmentParams) ([]error, error) {
		c.Assert(args, gc.HasLen, 1)
		c.Assert(args[0].Machine.String(), gc.Equals, expectedAttachmentIds[0].MachineTag)
		c.Assert(args[0].Volume.String(), gc.Equals, expectedAttachmentIds[0].AttachmentTag)
		defer close(detached)
		return make([]error, len(args)), nil
	}

	args := &workerArgs{
		volumes: volumeAccessor,
		life: &mockLifecycleManager{
			attachmentLife: attachmentLife,
		},
		registry: s.registry,
	}
	w := newStorageProvisioner(c, args)
	defer func() { c.Assert(w.Wait(), gc.IsNil) }()
	defer w.Kill()

	volumeAccessor.provisionedAttachments[expectedAttachmentIds[0]] = params.VolumeAttachment{
		MachineTag: "unit-mariadb-1",
		VolumeTag:  "volume-1",
	}
	volumeAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "unit-mariadb-1", AttachmentTag: "volume-1",
	}}
	waitChannel(c, detached, "waiting for volume to be detached")
}

func (s *caasStorageProvisionerSuite) TestRemoveVolumes(c *gc.C) {
	volumeAccessor := newMockVolumeAccessor()

	expectedAttachmentIds := []params.MachineStorageId{{
		MachineTag: "unit-mariadb-1", AttachmentTag: "volume-1",
	}}

	attachmentLife := func(ids []params.MachineStorageId) ([]params.LifeResult, error) {
		return []params.LifeResult{{Life: life.Dying}}, nil
	}

	removed := make(chan interface{})
	removeAttachments := func(ids []params.MachineStorageId) ([]params.ErrorResult, error) {
		c.Assert(ids, gc.DeepEquals, expectedAttachmentIds)
		close(removed)
		return make([]params.ErrorResult, len(ids)), nil
	}

	args := &workerArgs{
		volumes: volumeAccessor,
		life: &mockLifecycleManager{
			attachmentLife:    attachmentLife,
			removeAttachments: removeAttachments,
		},
		registry: s.registry,
	}
	w := newStorageProvisioner(c, args)
	defer func() { c.Assert(w.Wait(), gc.IsNil) }()
	defer w.Kill()

	volumeAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "unit-mariadb-1", AttachmentTag: "volume-1",
	}}
	waitChannel(c, removed, "waiting for attachment to be removed")
}

func (s *caasStorageProvisionerSuite) TestDetachFilesystems(c *gc.C) {
	removed := make(chan interface{})
	removeAttachments := func(ids []params.MachineStorageId) ([]params.ErrorResult, error) {
		defer close(removed)
		c.Assert(ids, gc.DeepEquals, []params.MachineStorageId{{
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "filesystem-0",
		}})
		return make([]params.ErrorResult, len(ids)), nil
	}

	args := &workerArgs{
		life:     &mockLifecycleManager{removeAttachments: removeAttachments},
		registry: s.registry,
	}
	w := newStorageProvisioner(c, args)
	defer w.Wait()
	defer w.Kill()

	args.filesystems.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "unit-mariadb-0", AttachmentTag: "filesystem-0",
	}}
	waitChannel(c, removed, "waiting for attachment to be removed")
}

func (s *caasStorageProvisionerSuite) TestRemoveFilesystems(c *gc.C) {
	filesystemAccessor := newMockFilesystemAccessor()

	expectedAttachmentIds := []params.MachineStorageId{{
		MachineTag: "unit-mariadb-1", AttachmentTag: "filesystem-1",
	}}

	attachmentLife := func(ids []params.MachineStorageId) ([]params.LifeResult, error) {
		c.Assert(ids, gc.DeepEquals, expectedAttachmentIds)
		return []params.LifeResult{{Life: life.Dying}}, nil
	}

	removed := make(chan interface{})
	removeAttachments := func(ids []params.MachineStorageId) ([]params.ErrorResult, error) {
		c.Assert(ids, gc.DeepEquals, expectedAttachmentIds)
		close(removed)
		return make([]params.ErrorResult, len(ids)), nil
	}

	args := &workerArgs{
		filesystems: filesystemAccessor,
		life: &mockLifecycleManager{
			attachmentLife:    attachmentLife,
			removeAttachments: removeAttachments,
		},
		registry: s.registry,
	}
	w := newStorageProvisioner(c, args)
	defer func() { c.Assert(w.Wait(), gc.IsNil) }()
	defer w.Kill()

	filesystemAccessor.attachmentsWatcher.changes <- []watcher.MachineStorageID{{
		MachineTag: "unit-mariadb-1", AttachmentTag: "filesystem-1",
	}}
	waitChannel(c, removed, "waiting for filesystem to be removed")
}

func newStorageProvisioner(c *gc.C, args *workerArgs) worker.Worker {
	if args == nil {
		args = &workerArgs{}
	}
	var storageDir string
	switch args.scope.(type) {
	case names.MachineTag:
		storageDir = "storage-dir"
	case names.ModelTag:
	case nil:
		args.scope = coretesting.ModelTag
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
	if args.machines == nil {
		args.machines = newMockMachineAccessor(c)
	}
	if args.clock == nil {
		args.clock = &mockClock{}
	}
	if args.statusSetter == nil {
		args.statusSetter = &mockStatusSetter{}
	}
	worker, err := storageprovisioner.NewStorageProvisioner(storageprovisioner.Config{
		Scope:       args.scope,
		StorageDir:  storageDir,
		Volumes:     args.volumes,
		Filesystems: args.filesystems,
		Life:        args.life,
		Registry:    args.registry,
		Machines:    args.machines,
		Status:      args.statusSetter,
		Clock:       args.clock,
		Logger:      loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, jc.ErrorIsNil)
	return worker
}

type workerArgs struct {
	scope        names.Tag
	volumes      *mockVolumeAccessor
	filesystems  *mockFilesystemAccessor
	life         *mockLifecycleManager
	registry     storage.ProviderRegistry
	machines     *mockMachineAccessor
	clock        clock.Clock
	statusSetter *mockStatusSetter
}

func waitChannel(c *gc.C, ch <-chan interface{}, activity string) interface{} {
	select {
	case v := <-ch:
		return v
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out %s", activity)
		panic("unreachable")
	}
}

func assertNoEvent(c *gc.C, ch <-chan interface{}, event string) {
	select {
	case <-ch:
		c.Fatalf("unexpected %s", event)
	case <-time.After(coretesting.ShortWait):
	}
}
