// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/storageprovisioner"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider/dummy"
	"github.com/juju/juju/storage/provider/registry"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

var _ = gc.Suite(&provisionerSuite{})

type provisionerSuite struct {
	// TODO(wallyworld) remove JujuConnSuite
	jujutesting.JujuConnSuite

	factory    *factory.Factory
	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	api        *storageprovisioner.StorageProvisionerAPI
}

func (s *provisionerSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)

	registry.RegisterProvider("environscoped", &dummy.StorageProvider{
		StorageScope: storage.ScopeEnviron,
	})
	registry.RegisterProvider("machinescoped", &dummy.StorageProvider{
		StorageScope: storage.ScopeMachine,
	})
	registry.RegisterEnvironStorageProviders(
		"dummy", "environscoped", "machinescoped",
	)
	s.AddSuiteCleanup(func(c *gc.C) {
		registry.RegisterProvider("environscoped", nil)
		registry.RegisterProvider("machinescoped", nil)
	})
}

func (s *provisionerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.factory = factory.NewFactory(s.State)
	s.resources = common.NewResources()
	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	var err error
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:            names.NewMachineTag("0"),
		EnvironManager: true,
	}
	s.api, err = storageprovisioner.NewStorageProvisionerAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *provisionerSuite) TestNewStorageProvisionerAPINonMachine(c *gc.C) {
	tag := names.NewUnitTag("mysql/0")
	authorizer := &apiservertesting.FakeAuthorizer{Tag: tag}
	_, err := storageprovisioner.NewStorageProvisionerAPI(s.State, common.NewResources(), authorizer)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *provisionerSuite) setupVolumes(c *gc.C) {
	s.factory.MakeMachine(c, &factory.MachineParams{
		InstanceId: instance.Id("inst-id"),
		Volumes: []state.MachineVolumeParams{
			{Volume: state.VolumeParams{Pool: "machinescoped", Size: 1024}},
			{Volume: state.VolumeParams{Pool: "environscoped", Size: 2048}},
			{Volume: state.VolumeParams{Pool: "environscoped", Size: 4096}},
			{
				Volume: state.VolumeParams{Pool: "environscoped", Size: 4096},
				Attachment: state.VolumeAttachmentParams{
					ReadOnly: true,
				},
			},
		},
	})
	// Only provision the first and third volumes.
	err := s.State.SetVolumeInfo(names.NewVolumeTag("0/0"), state.VolumeInfo{
		HardwareId: "123",
		VolumeId:   "abc",
		Size:       1024,
		Persistent: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.SetVolumeInfo(names.NewVolumeTag("2"), state.VolumeInfo{
		HardwareId: "456",
		VolumeId:   "def",
		Size:       4096,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Make a machine without storage for tests to use.
	s.factory.MakeMachine(c, nil)

	// Make an unprovisioned machine with storage for tests to use.
	// TODO(axw) extend testing/factory to allow creating unprovisioned
	// machines.
	_, err = s.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
		Volumes: []state.MachineVolumeParams{
			{Volume: state.VolumeParams{Pool: "environscoped", Size: 2048}},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *provisionerSuite) setupFilesystems(c *gc.C) {
	s.factory.MakeMachine(c, &factory.MachineParams{
		InstanceId: instance.Id("inst-id"),
		Filesystems: []state.MachineFilesystemParams{{
			Filesystem: state.FilesystemParams{Pool: "machinescoped", Size: 1024},
			Attachment: state.FilesystemAttachmentParams{
				Location: "/srv",
				ReadOnly: true,
			},
		}, {
			Filesystem: state.FilesystemParams{Pool: "environscoped", Size: 2048},
		}, {
			Filesystem: state.FilesystemParams{Pool: "environscoped", Size: 4096},
		}},
	})

	// Only provision the first and third filesystems.
	err := s.State.SetFilesystemInfo(names.NewFilesystemTag("0/0"), state.FilesystemInfo{
		FilesystemId: "abc",
		Size:         1024,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.SetFilesystemInfo(names.NewFilesystemTag("2"), state.FilesystemInfo{
		FilesystemId: "def",
		Size:         4096,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Make a machine without storage for tests to use.
	s.factory.MakeMachine(c, nil)

	// Make an unprovisioned machine with storage for tests to use.
	// TODO(axw) extend testing/factory to allow creating unprovisioned
	// machines.
	_, err = s.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
		Filesystems: []state.MachineFilesystemParams{{
			Filesystem: state.FilesystemParams{Pool: "environscoped", Size: 2048},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *provisionerSuite) TestVolumesMachine(c *gc.C) {
	s.setupVolumes(c)
	s.authorizer.EnvironManager = false

	results, err := s.api.Volumes(params.Entities{
		Entities: []params.Entity{{"volume-0-0"}, {"volume-1"}, {"volume-42"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.VolumeResults{
		Results: []params.VolumeResult{
			{Result: params.Volume{
				VolumeTag: "volume-0-0",
				Info: params.VolumeInfo{
					VolumeId:   "abc",
					HardwareId: "123",
					Size:       1024,
					Persistent: true,
				},
			}},
			{Error: &params.Error{"permission denied", "unauthorized access"}},
			{Error: &params.Error{"permission denied", "unauthorized access"}},
		},
	})
}

func (s *provisionerSuite) TestVolumesEnviron(c *gc.C) {
	s.setupVolumes(c)
	s.authorizer.Tag = names.NewMachineTag("2") // neither 0 nor 1

	results, err := s.api.Volumes(params.Entities{
		Entities: []params.Entity{
			{"volume-0-0"},
			{"volume-1"},
			{"volume-2"},
			{"volume-42"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.VolumeResults{
		Results: []params.VolumeResult{
			{Error: &params.Error{"permission denied", "unauthorized access"}},
			{Error: common.ServerError(errors.NotProvisionedf(`volume "1"`))},
			{Result: params.Volume{
				VolumeTag: "volume-2",
				Info: params.VolumeInfo{
					VolumeId:   "def",
					HardwareId: "456",
					Size:       4096,
				},
			}},
			{Error: &params.Error{"permission denied", "unauthorized access"}},
		},
	})
}

func (s *provisionerSuite) TestVolumesEmptyArgs(c *gc.C) {
	results, err := s.api.Volumes(params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)
}

func (s *provisionerSuite) TestFilesystems(c *gc.C) {
	s.setupFilesystems(c)
	s.authorizer.Tag = names.NewMachineTag("2") // neither 0 nor 1

	results, err := s.api.Filesystems(params.Entities{
		Entities: []params.Entity{
			{"filesystem-0-0"},
			{"filesystem-1"},
			{"filesystem-2"},
			{"filesystem-42"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.FilesystemResults{
		Results: []params.FilesystemResult{
			{Error: &params.Error{"permission denied", "unauthorized access"}},
			{Error: common.ServerError(errors.NotProvisionedf(`filesystem "1"`))},
			{Result: params.Filesystem{
				FilesystemTag: "filesystem-2",
				Info: params.FilesystemInfo{
					FilesystemId: "def",
					Size:         4096,
				},
			}},
			{Error: &params.Error{"permission denied", "unauthorized access"}},
		},
	})
}

func (s *provisionerSuite) TestVolumeAttachments(c *gc.C) {
	s.setupVolumes(c)
	s.authorizer.EnvironManager = false

	err := s.State.SetVolumeAttachmentInfo(
		names.NewMachineTag("0"),
		names.NewVolumeTag("0/0"),
		state.VolumeAttachmentInfo{DeviceName: "xvdf1"},
	)
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.api.VolumeAttachments(params.MachineStorageIds{
		Ids: []params.MachineStorageId{{
			MachineTag:    "machine-0",
			AttachmentTag: "volume-0-0",
		}, {
			MachineTag:    "machine-0",
			AttachmentTag: "volume-2", // volume attachment not provisioned
		}, {
			MachineTag:    "machine-0",
			AttachmentTag: "volume-42",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.VolumeAttachmentResults{
		Results: []params.VolumeAttachmentResult{
			{Result: params.VolumeAttachment{
				VolumeTag:  "volume-0-0",
				MachineTag: "machine-0",
				Info: params.VolumeAttachmentInfo{
					DeviceName: "xvdf1",
				},
			}},
			{Error: &params.Error{
				Code:    params.CodeNotProvisioned,
				Message: `volume attachment "2" on "0" not provisioned`,
			}},
			{Error: &params.Error{"permission denied", "unauthorized access"}},
		},
	})
}

func (s *provisionerSuite) TestFilesystemAttachments(c *gc.C) {
	s.setupFilesystems(c)
	s.authorizer.EnvironManager = false

	err := s.State.SetFilesystemAttachmentInfo(
		names.NewMachineTag("0"),
		names.NewFilesystemTag("0/0"),
		state.FilesystemAttachmentInfo{
			MountPoint: "/srv",
			ReadOnly:   true,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.api.FilesystemAttachments(params.MachineStorageIds{
		Ids: []params.MachineStorageId{{
			MachineTag:    "machine-0",
			AttachmentTag: "filesystem-0-0",
		}, {
			MachineTag:    "machine-0",
			AttachmentTag: "filesystem-2", // filesystem attachment not provisioned
		}, {
			MachineTag:    "machine-0",
			AttachmentTag: "filesystem-42",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.FilesystemAttachmentResults{
		Results: []params.FilesystemAttachmentResult{
			{Result: params.FilesystemAttachment{
				FilesystemTag: "filesystem-0-0",
				MachineTag:    "machine-0",
				Info: params.FilesystemAttachmentInfo{
					MountPoint: "/srv",
					ReadOnly:   true,
				},
			}},
			{Error: &params.Error{
				Code:    params.CodeNotProvisioned,
				Message: `filesystem attachment "2" on "0" not provisioned`,
			}},
			{Error: &params.Error{"permission denied", "unauthorized access"}},
		},
	})
}

func (s *provisionerSuite) TestVolumeParams(c *gc.C) {
	s.setupVolumes(c)
	results, err := s.api.VolumeParams(params.Entities{
		Entities: []params.Entity{
			{"volume-0-0"},
			{"volume-1"},
			{"volume-3"},
			{"volume-42"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.VolumeParamsResults{
		Results: []params.VolumeParamsResult{
			{Result: params.VolumeParams{
				VolumeTag: "volume-0-0",
				Size:      1024,
				Provider:  "machinescoped",
				Tags: map[string]string{
					tags.JujuEnv: testing.EnvironmentTag.Id(),
				},
				Attachment: &params.VolumeAttachmentParams{
					MachineTag: "machine-0",
					VolumeTag:  "volume-0-0",
					Provider:   "machinescoped",
					InstanceId: "inst-id",
				},
			}},
			{Result: params.VolumeParams{
				VolumeTag: "volume-1",
				Size:      2048,
				Provider:  "environscoped",
				Tags: map[string]string{
					tags.JujuEnv: testing.EnvironmentTag.Id(),
				},
				Attachment: &params.VolumeAttachmentParams{
					MachineTag: "machine-0",
					VolumeTag:  "volume-1",
					Provider:   "environscoped",
					InstanceId: "inst-id",
				},
			}},
			{Result: params.VolumeParams{
				VolumeTag: "volume-3",
				Size:      4096,
				Provider:  "environscoped",
				Tags: map[string]string{
					tags.JujuEnv: testing.EnvironmentTag.Id(),
				},
				Attachment: &params.VolumeAttachmentParams{
					MachineTag: "machine-0",
					VolumeTag:  "volume-3",
					Provider:   "environscoped",
					InstanceId: "inst-id",
					ReadOnly:   true,
				},
			}},
			{Error: &params.Error{"permission denied", "unauthorized access"}},
		},
	})
}

func (s *provisionerSuite) TestVolumeParamsEmptyArgs(c *gc.C) {
	results, err := s.api.VolumeParams(params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)
}

func (s *provisionerSuite) TestFilesystemParams(c *gc.C) {
	s.setupFilesystems(c)
	results, err := s.api.FilesystemParams(params.Entities{
		Entities: []params.Entity{{"filesystem-0-0"}, {"filesystem-1"}, {"filesystem-42"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.FilesystemParamsResults{
		Results: []params.FilesystemParamsResult{
			{Result: params.FilesystemParams{
				FilesystemTag: "filesystem-0-0",
				Size:          1024,
				Provider:      "machinescoped",
				Tags: map[string]string{
					tags.JujuEnv: testing.EnvironmentTag.Id(),
				},
			}},
			{Result: params.FilesystemParams{
				FilesystemTag: "filesystem-1",
				Size:          2048,
				Provider:      "environscoped",
				Tags: map[string]string{
					tags.JujuEnv: testing.EnvironmentTag.Id(),
				},
			}},
			{Error: &params.Error{"permission denied", "unauthorized access"}},
		},
	})
}

func (s *provisionerSuite) TestVolumeAttachmentParams(c *gc.C) {
	s.setupVolumes(c)

	err := s.State.SetVolumeInfo(names.NewVolumeTag("3"), state.VolumeInfo{
		HardwareId: "123",
		VolumeId:   "xyz",
		Size:       1024,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.SetVolumeAttachmentInfo(
		names.NewMachineTag("0"),
		names.NewVolumeTag("3"),
		state.VolumeAttachmentInfo{
			DeviceName: "xvdf1",
			ReadOnly:   true,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.api.VolumeAttachmentParams(params.MachineStorageIds{
		Ids: []params.MachineStorageId{{
			MachineTag:    "machine-0",
			AttachmentTag: "volume-0-0",
		}, {
			MachineTag:    "machine-0",
			AttachmentTag: "volume-1",
		}, {
			MachineTag:    "machine-0",
			AttachmentTag: "volume-3",
		}, {
			MachineTag:    "machine-2",
			AttachmentTag: "volume-4",
		}, {
			MachineTag:    "machine-0",
			AttachmentTag: "volume-42",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.VolumeAttachmentParamsResults{
		Results: []params.VolumeAttachmentParamsResult{
			{Result: params.VolumeAttachmentParams{
				MachineTag: "machine-0",
				VolumeTag:  "volume-0-0",
				InstanceId: "inst-id",
				VolumeId:   "abc",
				Provider:   "machinescoped",
			}},
			{Result: params.VolumeAttachmentParams{
				MachineTag: "machine-0",
				VolumeTag:  "volume-1",
				InstanceId: "inst-id",
				Provider:   "environscoped",
			}},
			{Result: params.VolumeAttachmentParams{
				MachineTag: "machine-0",
				VolumeTag:  "volume-3",
				InstanceId: "inst-id",
				VolumeId:   "xyz",
				Provider:   "environscoped",
				ReadOnly:   true,
			}},
			{Result: params.VolumeAttachmentParams{
				MachineTag: "machine-2",
				VolumeTag:  "volume-4",
				Provider:   "environscoped",
			}},
			{Error: &params.Error{"permission denied", "unauthorized access"}},
		},
	})
}

func (s *provisionerSuite) TestFilesystemAttachmentParams(c *gc.C) {
	s.setupFilesystems(c)

	err := s.State.SetFilesystemInfo(names.NewFilesystemTag("1"), state.FilesystemInfo{
		FilesystemId: "fsid",
		Size:         1024,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.SetFilesystemAttachmentInfo(
		names.NewMachineTag("0"),
		names.NewFilesystemTag("1"),
		state.FilesystemAttachmentInfo{
			MountPoint: "/in/the/place",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.api.FilesystemAttachmentParams(params.MachineStorageIds{
		Ids: []params.MachineStorageId{{
			MachineTag:    "machine-0",
			AttachmentTag: "filesystem-0-0",
		}, {
			MachineTag:    "machine-0",
			AttachmentTag: "filesystem-1",
		}, {
			MachineTag:    "machine-2",
			AttachmentTag: "filesystem-3",
		}, {
			MachineTag:    "machine-0",
			AttachmentTag: "filesystem-42",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.FilesystemAttachmentParamsResults{
		Results: []params.FilesystemAttachmentParamsResult{
			{Result: params.FilesystemAttachmentParams{
				MachineTag:    "machine-0",
				FilesystemTag: "filesystem-0-0",
				InstanceId:    "inst-id",
				FilesystemId:  "abc",
				Provider:      "machinescoped",
				MountPoint:    "/srv",
				ReadOnly:      true,
			}},
			{Result: params.FilesystemAttachmentParams{
				MachineTag:    "machine-0",
				FilesystemTag: "filesystem-1",
				InstanceId:    "inst-id",
				FilesystemId:  "fsid",
				Provider:      "environscoped",
				MountPoint:    "/in/the/place",
			}},
			{Result: params.FilesystemAttachmentParams{
				MachineTag:    "machine-2",
				FilesystemTag: "filesystem-3",
				Provider:      "environscoped",
			}},
			{Error: &params.Error{"permission denied", "unauthorized access"}},
		},
	})
}

func (s *provisionerSuite) TestSetVolumeAttachmentInfo(c *gc.C) {
	s.setupVolumes(c)

	err := s.State.SetVolumeInfo(names.NewVolumeTag("4"), state.VolumeInfo{
		VolumeId: "whatever",
		Size:     1024,
	})
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.api.SetVolumeAttachmentInfo(params.VolumeAttachments{
		VolumeAttachments: []params.VolumeAttachment{{
			MachineTag: "machine-0",
			VolumeTag:  "volume-0-0",
			Info: params.VolumeAttachmentInfo{
				DeviceName: "sda",
				ReadOnly:   true,
			},
		}, {
			MachineTag: "machine-0",
			VolumeTag:  "volume-1",
			Info: params.VolumeAttachmentInfo{
				DeviceName: "sdb",
			},
		}, {
			MachineTag: "machine-2",
			VolumeTag:  "volume-4",
			Info: params.VolumeAttachmentInfo{
				DeviceName: "sdc",
			},
		}, {
			MachineTag: "machine-0",
			VolumeTag:  "volume-42",
			Info: params.VolumeAttachmentInfo{
				DeviceName: "sdd",
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{},
			{Error: &params.Error{`cannot set info for volume attachment 1:0: volume "1" not provisioned`, "not provisioned"}},
			{Error: &params.Error{`cannot set info for volume attachment 4:2: machine 2 not provisioned`, "not provisioned"}},
			{Error: &params.Error{"permission denied", "unauthorized access"}},
		},
	})
}

func (s *provisionerSuite) TestSetFilesystemAttachmentInfo(c *gc.C) {
	s.setupFilesystems(c)

	err := s.State.SetFilesystemInfo(names.NewFilesystemTag("3"), state.FilesystemInfo{
		FilesystemId: "whatever",
		Size:         1024,
	})
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.api.SetFilesystemAttachmentInfo(params.FilesystemAttachments{
		FilesystemAttachments: []params.FilesystemAttachment{{
			MachineTag:    "machine-0",
			FilesystemTag: "filesystem-0-0",
			Info: params.FilesystemAttachmentInfo{
				MountPoint: "/srv/a",
				ReadOnly:   true,
			},
		}, {
			MachineTag:    "machine-0",
			FilesystemTag: "filesystem-1",
			Info: params.FilesystemAttachmentInfo{
				MountPoint: "/srv/b",
			},
		}, {
			MachineTag:    "machine-2",
			FilesystemTag: "filesystem-3",
			Info: params.FilesystemAttachmentInfo{
				MountPoint: "/srv/c",
			},
		}, {
			MachineTag:    "machine-0",
			FilesystemTag: "filesystem-42",
			Info: params.FilesystemAttachmentInfo{
				MountPoint: "/srv/d",
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{},
			{Error: &params.Error{`cannot set info for filesystem attachment 1:0: filesystem "1" not provisioned`, "not provisioned"}},
			{Error: &params.Error{`cannot set info for filesystem attachment 3:2: machine 2 not provisioned`, "not provisioned"}},
			{Error: &params.Error{"permission denied", "unauthorized access"}},
		},
	})
}

func (s *provisionerSuite) TestWatchVolumes(c *gc.C) {
	s.setupVolumes(c)
	s.factory.MakeMachine(c, nil)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{"machine-0"},
		{s.State.EnvironTag().String()},
		{"environ-adb650da-b77b-4ee8-9cbb-d57a9a592847"},
		{"machine-1"},
		{"machine-42"}},
	}
	result, err := s.api.WatchVolumes(args)
	c.Assert(err, jc.ErrorIsNil)
	sort.Strings(result.Results[1].Changes)
	c.Assert(result, jc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{StringsWatcherId: "1", Changes: []string{"0/0"}},
			{StringsWatcherId: "2", Changes: []string{"1", "2", "3", "4"}},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), gc.Equals, 2)
	v0Watcher := s.resources.Get("1")
	defer statetesting.AssertStop(c, v0Watcher)
	v1Watcher := s.resources.Get("2")
	defer statetesting.AssertStop(c, v1Watcher)

	// Check that the Watch has consumed the initial events ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.State, v0Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
	wc = statetesting.NewStringsWatcherC(c, s.State, v1Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *provisionerSuite) TestWatchVolumeAttachments(c *gc.C) {
	s.setupVolumes(c)
	s.factory.MakeMachine(c, nil)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{"machine-0"},
		{s.State.EnvironTag().String()},
		{"environ-adb650da-b77b-4ee8-9cbb-d57a9a592847"},
		{"machine-1"},
		{"machine-42"}},
	}
	result, err := s.api.WatchVolumeAttachments(args)
	c.Assert(err, jc.ErrorIsNil)
	sort.Sort(byMachineAndEntity(result.Results[0].Changes))
	sort.Sort(byMachineAndEntity(result.Results[1].Changes))
	c.Assert(result, jc.DeepEquals, params.MachineStorageIdsWatchResults{
		Results: []params.MachineStorageIdsWatchResult{
			{
				MachineStorageIdsWatcherId: "1",
				Changes: []params.MachineStorageId{{
					MachineTag:    "machine-0",
					AttachmentTag: "volume-0-0",
				}},
			},
			{
				MachineStorageIdsWatcherId: "2",
				Changes: []params.MachineStorageId{{
					MachineTag:    "machine-0",
					AttachmentTag: "volume-1",
				}, {
					MachineTag:    "machine-0",
					AttachmentTag: "volume-2",
				}, {
					MachineTag:    "machine-0",
					AttachmentTag: "volume-3",
				}, {
					MachineTag:    "machine-2",
					AttachmentTag: "volume-4",
				}},
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), gc.Equals, 2)
	v0Watcher := s.resources.Get("1")
	defer statetesting.AssertStop(c, v0Watcher)
	v1Watcher := s.resources.Get("2")
	defer statetesting.AssertStop(c, v1Watcher)

	// Check that the Watch has consumed the initial events ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.State, v0Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
	wc = statetesting.NewStringsWatcherC(c, s.State, v1Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *provisionerSuite) TestWatchFilesystems(c *gc.C) {
	s.setupFilesystems(c)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{"machine-0"},
		{s.State.EnvironTag().String()},
		{"environ-adb650da-b77b-4ee8-9cbb-d57a9a592847"},
		{"machine-1"},
		{"machine-42"}},
	}
	result, err := s.api.WatchFilesystems(args)
	c.Assert(err, jc.ErrorIsNil)
	sort.Strings(result.Results[1].Changes)
	c.Assert(result, jc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{
				StringsWatcherId: "1",
				Changes:          []string{"0/0"},
			},
			{
				StringsWatcherId: "2",
				Changes:          []string{"1", "2", "3"},
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), gc.Equals, 2)
	v0Watcher := s.resources.Get("1")
	defer statetesting.AssertStop(c, v0Watcher)
	v1Watcher := s.resources.Get("2")
	defer statetesting.AssertStop(c, v1Watcher)

	// Check that the Watch has consumed the initial events ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.State, v0Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
	wc = statetesting.NewStringsWatcherC(c, s.State, v1Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *provisionerSuite) TestWatchFilesystemAttachments(c *gc.C) {
	s.setupFilesystems(c)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{"machine-0"},
		{s.State.EnvironTag().String()},
		{"environ-adb650da-b77b-4ee8-9cbb-d57a9a592847"},
		{"machine-1"},
		{"machine-42"}},
	}
	result, err := s.api.WatchFilesystemAttachments(args)
	c.Assert(err, jc.ErrorIsNil)
	sort.Sort(byMachineAndEntity(result.Results[0].Changes))
	sort.Sort(byMachineAndEntity(result.Results[1].Changes))
	c.Assert(result, jc.DeepEquals, params.MachineStorageIdsWatchResults{
		Results: []params.MachineStorageIdsWatchResult{
			{
				MachineStorageIdsWatcherId: "1",
				Changes: []params.MachineStorageId{{
					MachineTag:    "machine-0",
					AttachmentTag: "filesystem-0-0",
				}},
			},
			{
				MachineStorageIdsWatcherId: "2",
				Changes: []params.MachineStorageId{{
					MachineTag:    "machine-0",
					AttachmentTag: "filesystem-1",
				}, {
					MachineTag:    "machine-0",
					AttachmentTag: "filesystem-2",
				}, {
					MachineTag:    "machine-2",
					AttachmentTag: "filesystem-3",
				}},
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), gc.Equals, 2)
	v0Watcher := s.resources.Get("1")
	defer statetesting.AssertStop(c, v0Watcher)
	v1Watcher := s.resources.Get("2")
	defer statetesting.AssertStop(c, v1Watcher)

	// Check that the Watch has consumed the initial events ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.State, v0Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
	wc = statetesting.NewStringsWatcherC(c, s.State, v1Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *provisionerSuite) TestWatchBlockDevices(c *gc.C) {
	s.factory.MakeMachine(c, nil)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{"machine-0"},
		{"service-mysql"},
		{"machine-1"},
		{"machine-42"}},
	}
	results, err := s.api.WatchBlockDevices(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "1"},
			{Error: &params.Error{Message: `"service-mysql" is not a valid machine tag`}},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), gc.Equals, 1)
	watcher := s.resources.Get("1")
	defer statetesting.AssertStop(c, watcher)

	// Check that the Watch has consumed the initial event.
	wc := statetesting.NewNotifyWatcherC(c, s.State, watcher.(state.NotifyWatcher))
	wc.AssertNoChange()

	m, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetMachineBlockDevices(state.BlockDeviceInfo{
		DeviceName: "sda",
		Size:       123,
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *provisionerSuite) TestVolumeBlockDevices(c *gc.C) {
	s.setupVolumes(c)
	s.factory.MakeMachine(c, nil)

	err := s.State.SetVolumeAttachmentInfo(
		names.NewMachineTag("0"),
		names.NewVolumeTag("0/0"),
		state.VolumeAttachmentInfo{},
	)
	c.Assert(err, jc.ErrorIsNil)

	machine0, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	err = machine0.SetMachineBlockDevices(state.BlockDeviceInfo{
		DeviceName: "sda",
		Size:       123,
		HardwareId: "123", // matches volume-0/0
	})
	c.Assert(err, jc.ErrorIsNil)

	args := params.MachineStorageIds{Ids: []params.MachineStorageId{
		{MachineTag: "machine-0", AttachmentTag: "volume-0-0"},
		{MachineTag: "machine-0", AttachmentTag: "volume-0-1"},
		{MachineTag: "machine-0", AttachmentTag: "volume-0-2"},
		{MachineTag: "machine-1", AttachmentTag: "volume-1"},
		{MachineTag: "machine-42", AttachmentTag: "volume-42"},
		{MachineTag: "service-mysql", AttachmentTag: "volume-1"},
	}}
	results, err := s.api.VolumeBlockDevices(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.BlockDeviceResults{
		Results: []params.BlockDeviceResult{
			{Result: storage.BlockDevice{
				DeviceName: "sda",
				Size:       123,
				HardwareId: "123",
			}},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: &params.Error{Message: `"service-mysql" is not a valid machine tag`}},
		},
	})
}

func (s *provisionerSuite) TestLife(c *gc.C) {
	s.setupVolumes(c)
	args := params.Entities{Entities: []params.Entity{{"volume-0-0"}, {"volume-1"}, {"volume-42"}}}
	result, err := s.api.Life(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: params.Alive},
			{Life: params.Alive},
			{Error: common.ServerError(errors.NotFoundf(`volume "42"`))},
		},
	})
}

func (s *provisionerSuite) TestAttachmentLife(c *gc.C) {
	s.setupVolumes(c)

	// TODO(axw) test filesystem attachment life
	// TODO(axw) test Dying

	results, err := s.api.AttachmentLife(params.MachineStorageIds{
		Ids: []params.MachineStorageId{{
			MachineTag:    "machine-0",
			AttachmentTag: "volume-0-0",
		}, {
			MachineTag:    "machine-0",
			AttachmentTag: "volume-1",
		}, {
			MachineTag:    "machine-2",
			AttachmentTag: "volume-4",
		}, {
			MachineTag:    "machine-0",
			AttachmentTag: "volume-42",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: params.Alive},
			{Life: params.Alive},
			{Life: params.Alive},
			{Error: &params.Error{"permission denied", "unauthorized access"}},
		},
	})
}

func (s *provisionerSuite) TestEnsureDead(c *gc.C) {
	s.setupVolumes(c)
	args := params.Entities{Entities: []params.Entity{{"volume-0-0"}, {"volume-1"}, {"volume-42"}}}
	result, err := s.api.EnsureDead(args)
	c.Assert(err, jc.ErrorIsNil)
	// TODO(wallyworld) - this test will be updated when EnsureDead is supported
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: common.ServerError(common.NotSupportedError(names.NewVolumeTag("0/0"), "ensuring death"))},
			{Error: common.ServerError(common.NotSupportedError(names.NewVolumeTag("1"), "ensuring death"))},
			{Error: common.ServerError(errors.NotFoundf(`volume "42"`))},
		},
	})
}

func (s *provisionerSuite) TestWatchForEnvironConfigChanges(c *gc.C) {
	result, err := s.api.WatchForEnvironConfigChanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.NotifyWatcherId, gc.Equals, "1")

	// Verify the resource was registered and stop it when done.
	c.Assert(s.resources.Count(), gc.Equals, 1)
	watcher := s.resources.Get("1")
	defer statetesting.AssertStop(c, watcher)

	// Check that the Watch has consumed the initial events ("returned" in
	// the Watch call)
	wc := statetesting.NewNotifyWatcherC(c, s.State, watcher.(state.NotifyWatcher))
	wc.AssertNoChange()

	// Updating config should trigger the watcher.
	err = s.State.UpdateEnvironConfig(map[string]interface{}{"what": "ever"}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *provisionerSuite) TestEnvironConfig(c *gc.C) {
	stateEnvironConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.api.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Config, jc.DeepEquals, params.EnvironConfig(stateEnvironConfig.AllAttrs()))
}

func (s *provisionerSuite) TestRemoveVolumesEnvironManager(c *gc.C) {
	s.setupVolumes(c)
	args := params.Entities{Entities: []params.Entity{
		{"volume-1-0"}, {"volume-1"}, {"volume-2"}, {"volume-42"},
		{"volume-invalid"}, {"machine-0"},
	}}

	err := s.State.DetachVolume(names.NewMachineTag("0"), names.NewVolumeTag("1"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.RemoveVolumeAttachment(names.NewMachineTag("0"), names.NewVolumeTag("1"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.DestroyVolume(names.NewVolumeTag("1"))
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.api.Remove(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{"permission denied", "unauthorized access"}},
			{Error: nil},
			{Error: &params.Error{Message: "removing volume 2: volume is not dead"}},
			{Error: nil},
			{Error: &params.Error{Message: `"volume-invalid" is not a valid volume tag`}},
			{Error: &params.Error{"permission denied", "unauthorized access"}},
		},
	})
}

func (s *provisionerSuite) TestRemoveFilesystemsEnvironManager(c *gc.C) {
	s.setupFilesystems(c)
	args := params.Entities{Entities: []params.Entity{
		{"filesystem-1-0"}, {"filesystem-1"}, {"filesystem-2"}, {"filesystem-42"},
		{"filesystem-invalid"}, {"machine-0"},
	}}

	err := s.State.DetachFilesystem(names.NewMachineTag("0"), names.NewFilesystemTag("1"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.RemoveFilesystemAttachment(names.NewMachineTag("0"), names.NewFilesystemTag("1"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.DestroyFilesystem(names.NewFilesystemTag("1"))
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.api.Remove(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{"permission denied", "unauthorized access"}},
			{Error: nil},
			{Error: &params.Error{Message: "removing filesystem 2: filesystem is not dead"}},
			{Error: nil},
			{Error: &params.Error{Message: `"filesystem-invalid" is not a valid filesystem tag`}},
			{Error: &params.Error{"permission denied", "unauthorized access"}},
		},
	})
}

func (s *provisionerSuite) TestRemoveVolumesMachineAgent(c *gc.C) {
	s.setupVolumes(c)
	s.authorizer.EnvironManager = false
	args := params.Entities{Entities: []params.Entity{
		{"volume-0-0"}, {"volume-0-42"}, {"volume-42"},
		{"volume-invalid"}, {"machine-0"},
	}}

	err := s.State.DetachVolume(names.NewMachineTag("0"), names.NewVolumeTag("0/0"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.RemoveVolumeAttachment(names.NewMachineTag("0"), names.NewVolumeTag("0/0"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.DestroyVolume(names.NewVolumeTag("0/0"))
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.api.Remove(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: nil},
			{Error: &params.Error{"permission denied", "unauthorized access"}},
			{Error: &params.Error{Message: `"volume-invalid" is not a valid volume tag`}},
			{Error: &params.Error{"permission denied", "unauthorized access"}},
		},
	})
}

func (s *provisionerSuite) TestRemoveFilesystemsMachineAgent(c *gc.C) {
	s.setupFilesystems(c)
	s.authorizer.EnvironManager = false
	args := params.Entities{Entities: []params.Entity{
		{"filesystem-0-0"}, {"filesystem-0-42"}, {"filesystem-42"},
		{"filesystem-invalid"}, {"machine-0"},
	}}

	err := s.State.DetachFilesystem(names.NewMachineTag("0"), names.NewFilesystemTag("0/0"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.RemoveFilesystemAttachment(names.NewMachineTag("0"), names.NewFilesystemTag("0/0"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.DestroyFilesystem(names.NewFilesystemTag("0/0"))
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.api.Remove(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: nil},
			{Error: &params.Error{"permission denied", "unauthorized access"}},
			{Error: &params.Error{Message: `"filesystem-invalid" is not a valid filesystem tag`}},
			{Error: &params.Error{"permission denied", "unauthorized access"}},
		},
	})
}

func (s *provisionerSuite) TestRemoveVolumeAttachments(c *gc.C) {
	s.setupVolumes(c)
	s.authorizer.EnvironManager = false

	err := s.State.DetachVolume(names.NewMachineTag("0"), names.NewVolumeTag("1"))
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.api.RemoveAttachment(params.MachineStorageIds{
		Ids: []params.MachineStorageId{{
			MachineTag:    "machine-0",
			AttachmentTag: "volume-0-0",
		}, {
			MachineTag:    "machine-0",
			AttachmentTag: "volume-1",
		}, {
			MachineTag:    "machine-2",
			AttachmentTag: "volume-4",
		}, {
			MachineTag:    "machine-0",
			AttachmentTag: "volume-42",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: "removing attachment of volume 0/0 from machine 0: volume attachment is not dying"}},
			{Error: nil},
			{Error: &params.Error{"permission denied", "unauthorized access"}},
			{Error: &params.Error{`removing attachment of volume 42 from machine 0: volume "42" on machine "0" not found`, "not found"}},
		},
	})
}

func (s *provisionerSuite) TestRemoveFilesystemAttachments(c *gc.C) {
	s.setupFilesystems(c)
	s.authorizer.EnvironManager = false

	err := s.State.DetachFilesystem(names.NewMachineTag("0"), names.NewFilesystemTag("1"))
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.api.RemoveAttachment(params.MachineStorageIds{
		Ids: []params.MachineStorageId{{
			MachineTag:    "machine-0",
			AttachmentTag: "filesystem-0-0",
		}, {
			MachineTag:    "machine-0",
			AttachmentTag: "filesystem-1",
		}, {
			MachineTag:    "machine-2",
			AttachmentTag: "filesystem-4",
		}, {
			MachineTag:    "machine-0",
			AttachmentTag: "filesystem-42",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: "removing attachment of filesystem 0/0 from machine 0: filesystem attachment is not dying"}},
			{Error: nil},
			{Error: &params.Error{"permission denied", "unauthorized access"}},
			{Error: &params.Error{`removing attachment of filesystem 42 from machine 0: filesystem "42" on machine "0" not found`, "not found"}},
		},
	})
}

type byMachineAndEntity []params.MachineStorageId

func (b byMachineAndEntity) Len() int {
	return len(b)
}

func (b byMachineAndEntity) Less(i, j int) bool {
	if b[i].MachineTag == b[j].MachineTag {
		return b[i].AttachmentTag < b[j].AttachmentTag
	}
	return b[i].MachineTag < b[j].MachineTag
}

func (b byMachineAndEntity) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}
