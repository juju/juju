// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"context"
	"sort"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/storageprovisioner"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tags"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	storageprovider "github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

type iaasProvisionerSuite struct {
	provisionerSuite

	store objectstore.ObjectStore
}

var _ = tc.Suite(&iaasProvisionerSuite{})

func (s *iaasProvisionerSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- TestRemoveVolumeParams: creates an app that will create a storage instance,
so we can release the storage and show the effects on the RemoveVolumeParams.
- TestRemoveFilesystemParams: creates an application that will create a storage
instance, so we can release the storage and show the effects on the
RemoveFilesystemParams.
`)
}

func (s *iaasProvisionerSuite) SetUpTest(c *tc.C) {
	s.provisionerSuite.SetUpTest(c)
	s.provisionerSuite.storageSetUp = s

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *tc.C) { s.resources.StopAll() })

	s.api = s.newApi(c, s.DefaultModelDomainServices(c).BlockDevice(), nil)
	s.store = jujutesting.NewObjectStore(c, s.ControllerModelUUID())
}

func (s *iaasProvisionerSuite) newApi(c *tc.C, blockDeviceService storageprovisioner.BlockDeviceService, watcherRegistry facade.WatcherRegistry) *storageprovisioner.StorageProvisionerAPIv4 {
	domainServices := s.ControllerDomainServices(c)
	modelInfo, err := domainServices.ModelInfo().GetModelInfo(context.Background())
	c.Assert(err, tc.ErrorIsNil)

	env, err := stateenvirons.GetNewEnvironFunc(environs.New)(s.ControllerModel(c), domainServices.Cloud(), domainServices.Credential(), s.DefaultModelDomainServices(c).Config())
	c.Assert(err, tc.ErrorIsNil)
	registry := storageprovider.NewStorageProviderRegistry(env)
	s.st = s.ControllerModel(c).State()
	svc := s.ControllerDomainServices(c)
	storageService := svc.Storage()

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	backend, storageBackend, err := storageprovisioner.NewStateBackends(s.st)
	c.Assert(err, tc.ErrorIsNil)
	s.storageBackend = storageBackend
	api, err := storageprovisioner.NewStorageProvisionerAPIv4(
		context.Background(),
		watcherRegistry,
		clock.WallClock,
		backend,
		storageBackend,
		blockDeviceService,
		s.ControllerDomainServices(c).Config(),
		s.ControllerDomainServices(c).Machine(),
		s.resources,
		s.authorizer,
		registry,
		storageService,
		loggertesting.WrapCheckLog(c),
		modelInfo.UUID,
		testing.ControllerTag.Id(),
	)
	c.Assert(err, tc.ErrorIsNil)
	return api
}

func (s *iaasProvisionerSuite) setupVolumes(c *tc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	machineService := s.ControllerDomainServices(c).Machine()
	machine0UUID, err := machineService.CreateMachine(context.Background(), machine.Name("0"))
	c.Assert(err, tc.ErrorIsNil)
	err = machineService.SetMachineCloudInstance(context.Background(), machine0UUID, "inst-id", "", nil)
	c.Assert(err, tc.ErrorIsNil)

	f.MakeMachine(c, &factory.MachineParams{
		InstanceId: instance.Id("inst-id"),
		Volumes: []state.HostVolumeParams{
			{Volume: state.VolumeParams{Pool: "machinescoped", Size: 1024}},
			{Volume: state.VolumeParams{Pool: "modelscoped", Size: 2048}},
			{Volume: state.VolumeParams{Pool: "modelscoped", Size: 4096}},
			{
				Volume: state.VolumeParams{Pool: "modelscoped", Size: 4096},
				Attachment: state.VolumeAttachmentParams{
					ReadOnly: true,
				},
			},
		},
	})
	// Only provision the first and third volumes.
	err = s.storageBackend.SetVolumeInfo(names.NewVolumeTag("0/0"), state.VolumeInfo{
		HardwareId: "123",
		VolumeId:   "abc",
		Size:       1024,
		Persistent: true,
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.storageBackend.SetVolumeInfo(names.NewVolumeTag("2"), state.VolumeInfo{
		HardwareId: "456",
		VolumeId:   "def",
		Size:       4096,
	})
	c.Assert(err, tc.ErrorIsNil)

	// Make a machine without storage for tests to use.
	f.MakeMachine(c, nil)
	machine1UUID, err := machineService.CreateMachine(context.Background(), machine.Name("1"))
	c.Assert(err, tc.ErrorIsNil)
	err = machineService.SetMachineCloudInstance(context.Background(), machine1UUID, "inst-id1", "", nil)
	c.Assert(err, tc.ErrorIsNil)

	// Make an unprovisioned machine with storage for tests to use.
	// TODO(axw) extend testing/factory to allow creating unprovisioned
	// machines.
	_, err = s.st.AddOneMachine(
		state.MachineTemplate{
			Base: state.UbuntuBase("12.10"),
			Jobs: []state.MachineJob{state.JobHostUnits},
			Volumes: []state.HostVolumeParams{
				{Volume: state.VolumeParams{Pool: "modelscoped", Size: 2048}},
			},
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	_, err = machineService.CreateMachine(context.Background(), machine.Name("2"))
	c.Assert(err, tc.ErrorIsNil)
}

func (s *iaasProvisionerSuite) setupFilesystems(c *tc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	machineService := s.ControllerDomainServices(c).Machine()
	machine0UUID, err := machineService.CreateMachine(context.Background(), machine.Name("0"))
	c.Assert(err, tc.ErrorIsNil)
	err = machineService.SetMachineCloudInstance(context.Background(), machine0UUID, "inst-id", "", nil)
	c.Assert(err, tc.ErrorIsNil)
	f.MakeMachine(c, &factory.MachineParams{
		InstanceId: instance.Id("inst-id"),
		Filesystems: []state.HostFilesystemParams{{
			Filesystem: state.FilesystemParams{Pool: "machinescoped", Size: 1024},
			Attachment: state.FilesystemAttachmentParams{
				Location: "/srv",
				ReadOnly: true,
			},
		}, {
			Filesystem: state.FilesystemParams{Pool: "modelscoped", Size: 2048},
		}, {
			Filesystem: state.FilesystemParams{Pool: "modelscoped", Size: 4096},
		}},
	})

	// Only provision the first and third filesystems.
	err = s.storageBackend.SetFilesystemInfo(names.NewFilesystemTag("0/0"), state.FilesystemInfo{
		FilesystemId: "abc",
		Size:         1024,
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.storageBackend.SetFilesystemInfo(names.NewFilesystemTag("2"), state.FilesystemInfo{
		FilesystemId: "def",
		Size:         4096,
	})
	c.Assert(err, tc.ErrorIsNil)

	// Make a machine without storage for tests to use.
	f.MakeMachine(c, nil)
	machine1UUID, err := machineService.CreateMachine(context.Background(), machine.Name("1"))
	c.Assert(err, tc.ErrorIsNil)
	err = machineService.SetMachineCloudInstance(context.Background(), machine1UUID, "inst-id1", "", nil)
	c.Assert(err, tc.ErrorIsNil)

	// Make an unprovisioned machine with storage for tests to use.
	// TODO(axw) extend testing/factory to allow creating unprovisioned
	// machines.
	_, err = s.st.AddOneMachine(
		state.MachineTemplate{
			Base: state.UbuntuBase("12.10"),
			Jobs: []state.MachineJob{state.JobHostUnits},
			Filesystems: []state.HostFilesystemParams{{
				Filesystem: state.FilesystemParams{Pool: "modelscoped", Size: 2048},
			}},
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	_, err = machineService.CreateMachine(context.Background(), machine.Name("2"))
	c.Assert(err, tc.ErrorIsNil)
}

func (s *iaasProvisionerSuite) TestHostedVolumes(c *tc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)
	s.authorizer.Controller = false

	results, err := s.api.Volumes(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: "volume-0-0"}, {Tag: "volume-1"}, {Tag: "volume-42"}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.VolumeResults{
		Results: []params.VolumeResult{
			{Result: params.Volume{
				VolumeTag: "volume-0-0",
				Info: params.VolumeInfo{
					VolumeId:   "abc",
					HardwareId: "123",
					Size:       1024,
					Persistent: true,
					Pool:       "machinescoped",
				},
			}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestVolumesModel(c *tc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)
	s.authorizer.Tag = names.NewMachineTag("2") // neither 0 nor 1

	results, err := s.api.Volumes(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: "volume-0-0"},
			{Tag: "volume-1"},
			{Tag: "volume-2"},
			{Tag: "volume-42"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.VolumeResults{
		Results: []params.VolumeResult{
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Error: apiservererrors.ServerError(errors.NotProvisionedf(`volume "1"`))},
			{Result: params.Volume{
				VolumeTag: "volume-2",
				Info: params.VolumeInfo{
					VolumeId:   "def",
					HardwareId: "456",
					Size:       4096,
					Pool:       "modelscoped",
				},
			}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestFilesystems(c *tc.C) {
	s.setupFilesystems(c)
	s.authorizer.Tag = names.NewMachineTag("2") // neither 0 nor 1

	results, err := s.api.Filesystems(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: "filesystem-0-0"},
			{Tag: "filesystem-1"},
			{Tag: "filesystem-2"},
			{Tag: "filesystem-42"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.FilesystemResults{
		Results: []params.FilesystemResult{
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Error: apiservererrors.ServerError(errors.NotProvisionedf(`filesystem "1"`))},
			{Result: params.Filesystem{
				FilesystemTag: "filesystem-2",
				Info: params.FilesystemInfo{
					FilesystemId: "def",
					Size:         4096,
					Pool:         "modelscoped",
				},
			}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestVolumeAttachments(c *tc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)
	s.authorizer.Controller = false

	err := s.storageBackend.SetVolumeAttachmentInfo(
		names.NewMachineTag("0"),
		names.NewVolumeTag("0/0"),
		state.VolumeAttachmentInfo{DeviceName: "xvdf1"},
	)
	c.Assert(err, tc.ErrorIsNil)

	results, err := s.api.VolumeAttachments(context.Background(), params.MachineStorageIds{
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.VolumeAttachmentResults{
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
				Message: `volume attachment "2" on "machine 0" not provisioned`,
			}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestFilesystemAttachments(c *tc.C) {
	s.setupFilesystems(c)
	s.authorizer.Controller = false

	err := s.storageBackend.SetFilesystemAttachmentInfo(
		names.NewMachineTag("0"),
		names.NewFilesystemTag("0/0"),
		state.FilesystemAttachmentInfo{
			MountPoint: "/srv",
			ReadOnly:   true,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	results, err := s.api.FilesystemAttachments(context.Background(), params.MachineStorageIds{
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.FilesystemAttachmentResults{
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
				Message: `filesystem attachment "2" on "machine 0" not provisioned`,
			}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestVolumeParams(c *tc.C) {
	// Set custom resource-tags in model config, and check they show up in the
	// returned volume params
	err := s.ControllerDomainServices(c).Config().UpdateModelConfig(
		context.Background(), map[string]any{
			config.ResourceTagsKey: "origin=v2 owner=Canonical",
		}, nil)
	c.Assert(err, tc.ErrorIsNil)

	// Only IAAS models support block storage right now.
	s.setupVolumes(c)
	results, err := s.api.VolumeParams(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: "volume-0-0"},
			{Tag: "volume-1"},
			{Tag: "volume-3"},
			{Tag: "volume-42"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.VolumeParamsResults{
		Results: []params.VolumeParamsResult{
			{Result: params.VolumeParams{
				VolumeTag: "volume-0-0",
				Size:      1024,
				Provider:  "machinescoped",
				Tags: map[string]string{
					tags.JujuController: testing.ControllerTag.Id(),
					tags.JujuModel:      testing.ModelTag.Id(),
					"origin":            "v2",
					"owner":             "Canonical",
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
				Provider:  "modelscoped",
				Tags: map[string]string{
					tags.JujuController: testing.ControllerTag.Id(),
					tags.JujuModel:      testing.ModelTag.Id(),
					"origin":            "v2",
					"owner":             "Canonical",
				},
				Attachment: &params.VolumeAttachmentParams{
					MachineTag: "machine-0",
					VolumeTag:  "volume-1",
					Provider:   "modelscoped",
					InstanceId: "inst-id",
				},
			}},
			{Result: params.VolumeParams{
				VolumeTag: "volume-3",
				Size:      4096,
				Provider:  "modelscoped",
				Tags: map[string]string{
					tags.JujuController: testing.ControllerTag.Id(),
					tags.JujuModel:      testing.ModelTag.Id(),
					"origin":            "v2",
					"owner":             "Canonical",
				},
				Attachment: &params.VolumeAttachmentParams{
					MachineTag: "machine-0",
					VolumeTag:  "volume-3",
					Provider:   "modelscoped",
					InstanceId: "inst-id",
					ReadOnly:   true,
				},
			}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestFilesystemParams(c *tc.C) {
	// Set custom resource-tags in model config, and check they show up in the
	// returned filesystem params
	err := s.ControllerDomainServices(c).Config().UpdateModelConfig(
		context.Background(), map[string]any{
			config.ResourceTagsKey: "origin=v2 owner=Canonical",
		}, nil)
	c.Assert(err, tc.ErrorIsNil)

	s.setupFilesystems(c)
	results, err := s.api.FilesystemParams(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: "filesystem-0-0"}, {Tag: "filesystem-1"}, {Tag: "filesystem-42"}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.FilesystemParamsResults{
		Results: []params.FilesystemParamsResult{
			{Result: params.FilesystemParams{
				FilesystemTag: "filesystem-0-0",
				Size:          1024,
				Provider:      "machinescoped",
				Tags: map[string]string{
					tags.JujuController: testing.ControllerTag.Id(),
					tags.JujuModel:      testing.ModelTag.Id(),
					"origin":            "v2",
					"owner":             "Canonical",
				},
			}},
			{Result: params.FilesystemParams{
				FilesystemTag: "filesystem-1",
				Size:          2048,
				Provider:      "modelscoped",
				Tags: map[string]string{
					tags.JujuController: testing.ControllerTag.Id(),
					tags.JujuModel:      testing.ModelTag.Id(),
					"origin":            "v2",
					"owner":             "Canonical",
				},
			}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestVolumeAttachmentParams(c *tc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)

	err := s.storageBackend.SetVolumeInfo(names.NewVolumeTag("3"), state.VolumeInfo{
		HardwareId: "123",
		VolumeId:   "xyz",
		Size:       1024,
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.storageBackend.SetVolumeAttachmentInfo(
		names.NewMachineTag("0"),
		names.NewVolumeTag("3"),
		state.VolumeAttachmentInfo{
			DeviceName: "xvdf1",
			ReadOnly:   true,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	results, err := s.api.VolumeAttachmentParams(context.Background(), params.MachineStorageIds{
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.VolumeAttachmentParamsResults{
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
				Provider:   "modelscoped",
			}},
			{Result: params.VolumeAttachmentParams{
				MachineTag: "machine-0",
				VolumeTag:  "volume-3",
				InstanceId: "inst-id",
				VolumeId:   "xyz",
				Provider:   "modelscoped",
				ReadOnly:   true,
			}},
			{Result: params.VolumeAttachmentParams{
				MachineTag: "machine-2",
				VolumeTag:  "volume-4",
				Provider:   "modelscoped",
			}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestFilesystemAttachmentParams(c *tc.C) {
	s.setupFilesystems(c)

	err := s.storageBackend.SetFilesystemInfo(names.NewFilesystemTag("1"), state.FilesystemInfo{
		FilesystemId: "fsid",
		Size:         1024,
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.storageBackend.SetFilesystemAttachmentInfo(
		names.NewMachineTag("0"),
		names.NewFilesystemTag("1"),
		state.FilesystemAttachmentInfo{
			MountPoint: "/in/the/place",
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	results, err := s.api.FilesystemAttachmentParams(context.Background(), params.MachineStorageIds{
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
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(results, tc.DeepEquals, params.FilesystemAttachmentParamsResults{
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
				Provider:      "modelscoped",
				MountPoint:    "/in/the/place",
			}},
			{Result: params.FilesystemAttachmentParams{
				MachineTag:    "machine-2",
				FilesystemTag: "filesystem-3",
				Provider:      "modelscoped",
			}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestSetVolumeAttachmentInfo(c *tc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)

	err := s.storageBackend.SetVolumeInfo(names.NewVolumeTag("4"), state.VolumeInfo{
		VolumeId: "whatever",
		Size:     1024,
	})
	c.Assert(err, tc.ErrorIsNil)

	results, err := s.api.SetVolumeAttachmentInfo(context.Background(), params.VolumeAttachments{
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
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.HasLen, 4)
	c.Check(results.Results[0].Error, tc.IsNil)
	c.Check(results.Results[1].Error.Code, tc.Equals, "not provisioned")
	c.Check(results.Results[1].Error.Message, tc.Matches, ".*not provisioned")
	c.Check(results.Results[2].Error.Code, tc.Equals, "not provisioned")
	c.Check(results.Results[2].Error.Message, tc.Matches, ".*not provisioned")
	c.Check(results.Results[3].Error.Code, tc.Equals, "unauthorized access")
	c.Check(results.Results[3].Error.Message, tc.Matches, "permission denied")
}

func (s *iaasProvisionerSuite) TestSetFilesystemAttachmentInfo(c *tc.C) {
	s.setupFilesystems(c)

	err := s.storageBackend.SetFilesystemInfo(names.NewFilesystemTag("3"), state.FilesystemInfo{
		FilesystemId: "whatever",
		Size:         1024,
	})
	c.Assert(err, tc.ErrorIsNil)

	results, err := s.api.SetFilesystemAttachmentInfo(context.Background(), params.FilesystemAttachments{
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
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.HasLen, 4)
	c.Check(results.Results[0].Error, tc.IsNil)
	c.Check(results.Results[1].Error.Code, tc.Equals, "not provisioned")
	c.Check(results.Results[1].Error.Message, tc.Matches, ".*not provisioned")
	c.Check(results.Results[2].Error.Code, tc.Equals, "not provisioned")
	c.Check(results.Results[2].Error.Message, tc.Matches, ".*not provisioned")
	c.Check(results.Results[3].Error.Code, tc.Equals, "unauthorized access")
	c.Check(results.Results[3].Error.Message, tc.Matches, "permission denied")
}

func (s *iaasProvisionerSuite) TestWatchVolumes(c *tc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	f.MakeMachine(c, nil)
	c.Assert(s.resources.Count(), tc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: names.NewModelTag(s.ControllerModelUUID()).String()},
		{Tag: "environ-adb650da-b77b-4ee8-9cbb-d57a9a592847"},
		{Tag: "machine-1"},
		{Tag: "machine-42"}},
	}
	result, err := s.api.WatchVolumes(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)
	sort.Strings(result.Results[1].Changes)
	c.Assert(result, tc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{StringsWatcherId: "1", Changes: []string{"0/0"}},
			{StringsWatcherId: "2", Changes: []string{"1", "2", "3", "4"}},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), tc.Equals, 2)
	v0Watcher := s.resources.Get("1")
	defer workertest.CleanKill(c, v0Watcher)
	v1Watcher := s.resources.Get("2")
	defer workertest.CleanKill(c, v1Watcher)

	// Check that the Watch has consumed the initial events ("returned" in
	// the Watch call)
	wc := watchertest.NewStringsWatcherC(c, v0Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
	wc = watchertest.NewStringsWatcherC(c, v1Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *iaasProvisionerSuite) TestWatchVolumeAttachments(c *tc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	f.MakeMachine(c, nil)
	c.Assert(s.resources.Count(), tc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: names.NewModelTag(s.ControllerModelUUID()).String()},
		{Tag: "environ-adb650da-b77b-4ee8-9cbb-d57a9a592847"},
		{Tag: "machine-1"},
		{Tag: "machine-42"}},
	}
	result, err := s.api.WatchVolumeAttachments(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)
	sort.Sort(byMachineAndEntity(result.Results[0].Changes))
	sort.Sort(byMachineAndEntity(result.Results[1].Changes))
	c.Assert(result, tc.DeepEquals, params.MachineStorageIdsWatchResults{
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
	c.Assert(s.resources.Count(), tc.Equals, 2)
	v0Watcher := s.resources.Get("1")
	defer workertest.CleanKill(c, v0Watcher)
	v1Watcher := s.resources.Get("2")
	defer workertest.CleanKill(c, v1Watcher)

	// Check that the Watch has consumed the initial events ("returned" in
	// the Watch call)
	wc := watchertest.NewStringsWatcherC(c, v0Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
	wc = watchertest.NewStringsWatcherC(c, v1Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *iaasProvisionerSuite) TestWatchFilesystems(c *tc.C) {
	s.setupFilesystems(c)
	c.Assert(s.resources.Count(), tc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: names.NewModelTag(s.ControllerModelUUID()).String()},
		{Tag: "environ-adb650da-b77b-4ee8-9cbb-d57a9a592847"},
		{Tag: "machine-1"},
		{Tag: "machine-42"}},
	}
	result, err := s.api.WatchFilesystems(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)
	sort.Strings(result.Results[1].Changes)
	c.Assert(result, tc.DeepEquals, params.StringsWatchResults{
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
	c.Assert(s.resources.Count(), tc.Equals, 2)
	v0Watcher := s.resources.Get("1")
	defer workertest.CleanKill(c, v0Watcher)
	v1Watcher := s.resources.Get("2")
	defer workertest.CleanKill(c, v1Watcher)

	// Check that the Watch has consumed the initial events ("returned" in
	// the Watch call)
	wc := watchertest.NewStringsWatcherC(c, v0Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
	wc = watchertest.NewStringsWatcherC(c, v1Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *iaasProvisionerSuite) TestWatchFilesystemAttachments(c *tc.C) {
	s.setupFilesystems(c)
	c.Assert(s.resources.Count(), tc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: names.NewModelTag(s.ControllerModelUUID()).String()},
		{Tag: "environ-adb650da-b77b-4ee8-9cbb-d57a9a592847"},
		{Tag: "machine-1"},
		{Tag: "machine-42"}},
	}
	result, err := s.api.WatchFilesystemAttachments(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)
	sort.Sort(byMachineAndEntity(result.Results[0].Changes))
	sort.Sort(byMachineAndEntity(result.Results[1].Changes))
	c.Assert(result, tc.DeepEquals, params.MachineStorageIdsWatchResults{
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
	c.Assert(s.resources.Count(), tc.Equals, 2)
	v0Watcher := s.resources.Get("1")
	defer workertest.CleanKill(c, v0Watcher)
	v1Watcher := s.resources.Get("2")
	defer workertest.CleanKill(c, v1Watcher)

	// Check that the Watch has consumed the initial events ("returned" in
	// the Watch call)
	wc := watchertest.NewStringsWatcherC(c, v0Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
	wc = watchertest.NewStringsWatcherC(c, v1Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *iaasProvisionerSuite) TestWatchBlockDevices(c *tc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	f.MakeMachine(c, nil)
	c.Assert(s.resources.Count(), tc.Equals, 0)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := make(chan struct{}, 1)
	ch <- struct{}{} // Initial event.
	watcher := watchertest.NewMockNotifyWatcher(ch)
	blockDeviceService := NewMockBlockDeviceService(ctrl)
	blockDeviceService.EXPECT().WatchBlockDevices(gomock.Any(), "0").Return(watcher, nil)

	watcherRegistry := mocks.NewMockWatcherRegistry(ctrl)
	watcherRegistry.EXPECT().Register(watcher).Return("1", nil)

	api := s.newApi(c, blockDeviceService, watcherRegistry)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: "application-mysql"},
		{Tag: "machine-1"},
		{Tag: "machine-42"}},
	}

	results, err := api.WatchBlockDevices(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "1"},
			{Error: &params.Error{Message: `"application-mysql" is not a valid machine tag`}},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Check that the Watch has consumed the initial event.
	wc := watchertest.NewNotifyWatcherC(c, watcher)
	wc.AssertNoChange()
}

func (s *iaasProvisionerSuite) TestVolumeBlockDevices(c *tc.C) {
	s.setupVolumes(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	f.MakeMachine(c, nil)

	err := s.storageBackend.SetVolumeAttachmentInfo(
		names.NewMachineTag("0"),
		names.NewVolumeTag("0/0"),
		state.VolumeAttachmentInfo{},
	)
	c.Assert(err, tc.ErrorIsNil)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	blockDeviceService := NewMockBlockDeviceService(ctrl)
	api := s.newApi(c, blockDeviceService, mocks.NewMockWatcherRegistry(ctrl))

	blockDeviceService.EXPECT().BlockDevices(gomock.Any(), "0").Return([]blockdevice.BlockDevice{{
		DeviceName: "sda",
		SizeMiB:    123,
		HardwareId: "123", // matches volume-0/0
	}}, nil)
	c.Assert(err, tc.ErrorIsNil)

	args := params.MachineStorageIds{Ids: []params.MachineStorageId{
		{MachineTag: "machine-0", AttachmentTag: "volume-0-0"},
		{MachineTag: "machine-0", AttachmentTag: "volume-0-1"},
		{MachineTag: "machine-0", AttachmentTag: "volume-0-2"},
		{MachineTag: "machine-1", AttachmentTag: "volume-1"},
		{MachineTag: "machine-42", AttachmentTag: "volume-42"},
		{MachineTag: "application-mysql", AttachmentTag: "volume-1"},
	}}
	results, err := api.VolumeBlockDevices(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.BlockDeviceResults{
		Results: []params.BlockDeviceResult{
			{Result: params.BlockDevice{
				DeviceName: "sda",
				Size:       123,
				HardwareId: "123",
			}},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: &params.Error{Code: params.CodeNotValid, Message: `volume attachment host tag "application-mysql" not valid`}},
		},
	})
}

func (s *iaasProvisionerSuite) TestVolumeBlockDevicesPlanBlockInfoSet(c *tc.C) {
	s.setupVolumes(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	f.MakeMachine(c, nil)

	err := s.storageBackend.SetVolumeAttachmentInfo(
		names.NewMachineTag("0"),
		names.NewVolumeTag("0/0"),
		state.VolumeAttachmentInfo{},
	)
	c.Assert(err, tc.ErrorIsNil)

	deviceAttrs := map[string]string{
		"iqn":         "bogusIQN",
		"address":     "192.168.1.1",
		"port":        "9999",
		"chap-user":   "example",
		"chap-secret": "supersecretpassword",
	}

	attachmentPlanInfo := state.VolumeAttachmentPlanInfo{
		DeviceType:       storage.DeviceTypeISCSI,
		DeviceAttributes: deviceAttrs,
	}

	err = s.storageBackend.CreateVolumeAttachmentPlan(
		names.NewMachineTag("0"), names.NewVolumeTag("0/0"), attachmentPlanInfo)
	c.Assert(err, tc.ErrorIsNil)

	// The HardwareId set here should override the HardwareId in the volume info.
	blockInfo := state.BlockDeviceInfo{
		WWN: "testWWN",
		DeviceLinks: []string{
			"/dev/sda", "/dev/mapper/testDevice"},
		HardwareId: "test-id",
	}
	err = s.storageBackend.SetVolumeAttachmentPlanBlockInfo(
		names.NewMachineTag("0"), names.NewVolumeTag("0/0"), blockInfo)
	c.Assert(err, tc.ErrorIsNil)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	blockDeviceService := NewMockBlockDeviceService(ctrl)
	api := s.newApi(c, blockDeviceService, mocks.NewMockWatcherRegistry(ctrl))

	blockDeviceService.EXPECT().BlockDevices(gomock.Any(), "0").Return([]blockdevice.BlockDevice{{
		DeviceName: "sda",
		SizeMiB:    123,
		HardwareId: "test-id",
	}}, nil)
	c.Assert(err, tc.ErrorIsNil)

	args := params.MachineStorageIds{Ids: []params.MachineStorageId{
		{MachineTag: "machine-0", AttachmentTag: "volume-0-0"},
	}}
	results, err := api.VolumeBlockDevices(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.BlockDeviceResults{
		Results: []params.BlockDeviceResult{
			{Result: params.BlockDevice{
				DeviceName: "sda",
				Size:       123,
				HardwareId: "test-id",
			}},
		},
	})
}

func (s *iaasProvisionerSuite) TestLife(c *tc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)
	args := params.Entities{Entities: []params.Entity{{Tag: "volume-0-0"}, {Tag: "volume-1"}, {Tag: "volume-42"}}}
	result, err := s.api.Life(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: life.Alive},
			{Life: life.Alive},
			{Error: apiservererrors.ServerError(errors.NotFoundf(`volume "42"`))},
		},
	})
}

func (s *iaasProvisionerSuite) TestAttachmentLife(c *tc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)

	// TODO(axw) test filesystem attachment life
	// TODO(axw) test Dying

	results, err := s.api.AttachmentLife(context.Background(), params.MachineStorageIds{
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: life.Alive},
			{Life: life.Alive},
			{Life: life.Alive},
			{Error: &params.Error{Message: `volume "42" on "machine 0" not found`, Code: "not found"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestEnsureDead(c *tc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)
	args := params.Entities{Entities: []params.Entity{{Tag: "volume-0-0"}, {Tag: "volume-1"}, {Tag: "volume-42"}}}
	result, err := s.api.EnsureDead(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)
	// TODO(wallyworld) - this test will be updated when EnsureDead is supported
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservererrors.ServerError(apiservererrors.NotSupportedError(names.NewVolumeTag("0/0"), "ensuring death"))},
			{Error: apiservererrors.ServerError(apiservererrors.NotSupportedError(names.NewVolumeTag("1"), "ensuring death"))},
			{Error: apiservererrors.ServerError(errors.NotFoundf(`volume "42"`))},
		},
	})
}

func (s *iaasProvisionerSuite) TestRemoveVolumesController(c *tc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)
	args := params.Entities{Entities: []params.Entity{
		{Tag: "volume-1-0"}, {Tag: "volume-1"}, {Tag: "volume-2"}, {Tag: "volume-42"},
		{Tag: "volume-invalid"}, {Tag: "machine-0"},
	}}

	err := s.storageBackend.DetachVolume(names.NewMachineTag("0"), names.NewVolumeTag("1"), false)
	c.Assert(err, tc.ErrorIsNil)
	err = s.storageBackend.RemoveVolumeAttachment(names.NewMachineTag("0"), names.NewVolumeTag("1"), false)
	c.Assert(err, tc.ErrorIsNil)
	err = s.storageBackend.DestroyVolume(names.NewVolumeTag("1"), false)
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.api.Remove(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Error: nil},
			{Error: &params.Error{Message: "removing volume 2: volume is not dead"}},
			{Error: nil},
			{Error: &params.Error{Message: `"volume-invalid" is not a valid volume tag`}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestRemoveFilesystemsController(c *tc.C) {
	s.setupFilesystems(c)
	args := params.Entities{Entities: []params.Entity{
		{Tag: "filesystem-1-0"}, {Tag: "filesystem-1"}, {Tag: "filesystem-2"}, {Tag: "filesystem-42"},
		{Tag: "filesystem-invalid"}, {Tag: "machine-0"},
	}}

	err := s.storageBackend.DetachFilesystem(names.NewMachineTag("0"), names.NewFilesystemTag("1"))
	c.Assert(err, tc.ErrorIsNil)
	err = s.storageBackend.RemoveFilesystemAttachment(names.NewMachineTag("0"), names.NewFilesystemTag("1"), false)
	c.Assert(err, tc.ErrorIsNil)
	err = s.storageBackend.DestroyFilesystem(names.NewFilesystemTag("1"), false)
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.api.Remove(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Error: nil},
			{Error: &params.Error{Message: "removing filesystem 2: filesystem is not dead"}},
			{Error: nil},
			{Error: &params.Error{Message: `"filesystem-invalid" is not a valid filesystem tag`}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestRemoveVolumesMachineAgent(c *tc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)
	s.authorizer.Controller = false
	args := params.Entities{Entities: []params.Entity{
		{Tag: "volume-0-0"}, {Tag: "volume-0-42"}, {Tag: "volume-42"},
		{Tag: "volume-invalid"}, {Tag: "machine-0"},
	}}

	err := s.storageBackend.DestroyVolume(names.NewVolumeTag("0/0"), false)
	c.Assert(err, tc.ErrorIsNil)
	err = s.storageBackend.RemoveVolumeAttachment(names.NewMachineTag("0"), names.NewVolumeTag("0/0"), false)
	c.Assert(err, tc.ErrorIsNil)
	err = s.storageBackend.DestroyVolume(names.NewVolumeTag("0/0"), false)
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.api.Remove(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: nil},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Error: &params.Error{Message: `"volume-invalid" is not a valid volume tag`}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestRemoveFilesystemsMachineAgent(c *tc.C) {
	s.setupFilesystems(c)
	s.authorizer.Controller = false
	args := params.Entities{Entities: []params.Entity{
		{Tag: "filesystem-0-0"}, {Tag: "filesystem-0-42"}, {Tag: "filesystem-42"},
		{Tag: "filesystem-invalid"}, {Tag: "machine-0"},
	}}

	err := s.storageBackend.DestroyFilesystem(names.NewFilesystemTag("0/0"), false)
	c.Assert(err, tc.ErrorIsNil)
	err = s.storageBackend.RemoveFilesystemAttachment(names.NewMachineTag("0"), names.NewFilesystemTag("0/0"), false)
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.api.Remove(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: nil},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Error: &params.Error{Message: `"filesystem-invalid" is not a valid filesystem tag`}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestRemoveVolumeAttachments(c *tc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)
	s.authorizer.Controller = false

	err := s.storageBackend.DetachVolume(names.NewMachineTag("0"), names.NewVolumeTag("1"), false)
	c.Assert(err, tc.ErrorIsNil)

	results, err := s.api.RemoveAttachment(context.Background(), params.MachineStorageIds{
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: "removing attachment of volume 0/0 from machine 0: volume attachment is not dying"}},
			{Error: nil},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Error: &params.Error{Message: `removing attachment of volume 42 from machine 0: volume "42" on "machine 0" not found`, Code: "not found"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestRemoveFilesystemAttachments(c *tc.C) {
	s.setupFilesystems(c)
	s.authorizer.Controller = false

	err := s.storageBackend.DetachFilesystem(names.NewMachineTag("0"), names.NewFilesystemTag("1"))
	c.Assert(err, tc.ErrorIsNil)

	results, err := s.api.RemoveAttachment(context.Background(), params.MachineStorageIds{
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: "removing attachment of filesystem 0/0 from machine 0: filesystem attachment is not dying"}},
			{Error: nil},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Error: &params.Error{Message: `removing attachment of filesystem 42 from machine 0: filesystem "42" on "machine 0" not found`, Code: "not found"}},
		},
	})
}
