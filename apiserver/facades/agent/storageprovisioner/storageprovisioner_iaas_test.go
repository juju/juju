// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"context"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/agent/storageprovisioner"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/poolmanager"
	jujujujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type iaasProvisionerSuite struct {
	provisionerSuite
}

var _ = gc.Suite(&iaasProvisionerSuite{})

func (s *iaasProvisionerSuite) SetUpTest(c *gc.C) {
	s.provisionerSuite.SetUpTest(c)
	s.provisionerSuite.storageSetUp = s

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	serviceFactory := s.ServiceFactory(jujujujutesting.DefaultModelUUID)

	env, err := stateenvirons.GetNewEnvironFunc(environs.New)(s.ControllerModel(c), serviceFactory.Cloud(), serviceFactory.Credential())
	c.Assert(err, jc.ErrorIsNil)
	registry := stateenvirons.NewStorageProviderRegistry(env)
	s.st = s.ControllerModel(c).State()
	pm := poolmanager.New(state.NewStateSettings(s.st), registry)

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	backend, storageBackend, err := storageprovisioner.NewStateBackends(s.st)
	c.Assert(err, jc.ErrorIsNil)
	s.storageBackend = storageBackend
	s.api, err = storageprovisioner.NewStorageProvisionerAPIv4(backend, storageBackend, s.resources, s.authorizer, registry, pm, loggo.GetLogger("juju.apiserver.storageprovisioner"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *iaasProvisionerSuite) setupVolumes(c *gc.C) {
	f, release := s.NewFactory(c, s.st.ModelUUID())
	defer release()

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
	err := s.storageBackend.SetVolumeInfo(names.NewVolumeTag("0/0"), state.VolumeInfo{
		HardwareId: "123",
		VolumeId:   "abc",
		Size:       1024,
		Persistent: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.SetVolumeInfo(names.NewVolumeTag("2"), state.VolumeInfo{
		HardwareId: "456",
		VolumeId:   "def",
		Size:       4096,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Make a machine without storage for tests to use.
	f.MakeMachine(c, nil)

	// Make an unprovisioned machine with storage for tests to use.
	// TODO(axw) extend testing/factory to allow creating unprovisioned
	// machines.
	_, err = s.st.AddOneMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
		Volumes: []state.HostVolumeParams{
			{Volume: state.VolumeParams{Pool: "modelscoped", Size: 2048}},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *iaasProvisionerSuite) setupFilesystems(c *gc.C) {
	f, release := s.NewFactory(c, s.st.ModelUUID())
	defer release()

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
	err := s.storageBackend.SetFilesystemInfo(names.NewFilesystemTag("0/0"), state.FilesystemInfo{
		FilesystemId: "abc",
		Size:         1024,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.SetFilesystemInfo(names.NewFilesystemTag("2"), state.FilesystemInfo{
		FilesystemId: "def",
		Size:         4096,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Make a machine without storage for tests to use.
	f.MakeMachine(c, nil)

	// Make an unprovisioned machine with storage for tests to use.
	// TODO(axw) extend testing/factory to allow creating unprovisioned
	// machines.
	_, err = s.st.AddOneMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
		Filesystems: []state.HostFilesystemParams{{
			Filesystem: state.FilesystemParams{Pool: "modelscoped", Size: 2048},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *iaasProvisionerSuite) TestHostedVolumes(c *gc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)
	s.authorizer.Controller = false

	results, err := s.api.Volumes(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: "volume-0-0"}, {Tag: "volume-1"}, {Tag: "volume-42"}},
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
					Pool:       "machinescoped",
				},
			}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestVolumesModel(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.VolumeResults{
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

func (s *iaasProvisionerSuite) TestFilesystems(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.FilesystemResults{
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

func (s *iaasProvisionerSuite) TestVolumeAttachments(c *gc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)
	s.authorizer.Controller = false

	err := s.storageBackend.SetVolumeAttachmentInfo(
		names.NewMachineTag("0"),
		names.NewVolumeTag("0/0"),
		state.VolumeAttachmentInfo{DeviceName: "xvdf1"},
	)
	c.Assert(err, jc.ErrorIsNil)

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
				Message: `volume attachment "2" on "machine 0" not provisioned`,
			}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestFilesystemAttachments(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)

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
				Message: `filesystem attachment "2" on "machine 0" not provisioned`,
			}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestVolumeParams(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.VolumeParamsResults{
		Results: []params.VolumeParamsResult{
			{Result: params.VolumeParams{
				VolumeTag: "volume-0-0",
				Size:      1024,
				Provider:  "machinescoped",
				Tags: map[string]string{
					tags.JujuController: testing.ControllerTag.Id(),
					tags.JujuModel:      testing.ModelTag.Id(),
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

func (s *iaasProvisionerSuite) TestRemoveVolumeParams(c *gc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)

	f, release := s.NewFactory(c, s.st.ModelUUID())
	defer release()

	// Deploy an application that will create a storage instance,
	// so we can release the storage and show the effects on the
	// RemoveVolumeParams.
	application := f.MakeApplication(c, &factory.ApplicationParams{
		Charm: f.MakeCharm(c, &factory.CharmParams{
			Name: "storage-block",
		}),
		Storage: map[string]state.StorageConstraints{
			"data": {
				Count: 1,
				Size:  1,
				Pool:  "modelscoped",
			},
		},
	})
	unit := f.MakeUnit(c, &factory.UnitParams{
		Application: application,
	})
	testStorage, err := s.storageBackend.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testStorage, gc.HasLen, 1)
	storageVolume, err := s.storageBackend.StorageInstanceVolume(testStorage[0].StorageTag())
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.SetVolumeInfo(storageVolume.VolumeTag(), state.VolumeInfo{
		VolumeId:   "zing",
		Size:       1,
		Persistent: true,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Make volumes 0/0 and 3 Dead.
	for _, volumeId := range []string{"0/0", "3"} {
		volumeTag := names.NewVolumeTag(volumeId)
		machineTag := names.NewMachineTag("0")
		err = s.storageBackend.DestroyVolume(volumeTag, false)
		c.Assert(err, jc.ErrorIsNil)
		err = s.storageBackend.DetachVolume(machineTag, volumeTag, false)
		c.Assert(err, jc.ErrorIsNil)
		err = s.storageBackend.RemoveVolumeAttachment(machineTag, volumeTag, false)
		c.Assert(err, jc.ErrorIsNil)
	}

	// Make the "data" storage volume Dead, releasing.
	err = unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.ReleaseStorageInstance(testStorage[0].StorageTag(), true, false, dontWait)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.DetachStorage(testStorage[0].StorageTag(), unit.UnitTag(), false, dontWait)
	c.Assert(err, jc.ErrorIsNil)
	unitMachineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	unitMachineTag := names.NewMachineTag(unitMachineId)
	err = s.storageBackend.DetachVolume(unitMachineTag, storageVolume.VolumeTag(), false)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveVolumeAttachment(unitMachineTag, storageVolume.VolumeTag(), false)
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.api.RemoveVolumeParams(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: "volume-0-0"},
			{Tag: storageVolume.Tag().String()},
			{Tag: "volume-1"},
			{Tag: "volume-3"},
			{Tag: "volume-42"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.RemoveVolumeParamsResults{
		Results: []params.RemoveVolumeParamsResult{{
			Result: params.RemoveVolumeParams{
				Provider: "machinescoped",
				VolumeId: "abc",
				Destroy:  true,
			},
		}, {
			Result: params.RemoveVolumeParams{
				Provider: "modelscoped",
				VolumeId: "zing",
				Destroy:  false,
			},
		}, {
			Error: &params.Error{Message: `volume 1 is not dead (alive)`},
		}, {
			Error: &params.Error{Message: `volume "3" not provisioned`, Code: "not provisioned"},
		}, {
			Error: &params.Error{Message: "permission denied", Code: "unauthorized access"},
		}},
	})
}

func (s *iaasProvisionerSuite) TestFilesystemParams(c *gc.C) {
	s.setupFilesystems(c)
	results, err := s.api.FilesystemParams(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: "filesystem-0-0"}, {Tag: "filesystem-1"}, {Tag: "filesystem-42"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.FilesystemParamsResults{
		Results: []params.FilesystemParamsResult{
			{Result: params.FilesystemParams{
				FilesystemTag: "filesystem-0-0",
				Size:          1024,
				Provider:      "machinescoped",
				Tags: map[string]string{
					tags.JujuController: testing.ControllerTag.Id(),
					tags.JujuModel:      testing.ModelTag.Id(),
				},
			}},
			{Result: params.FilesystemParams{
				FilesystemTag: "filesystem-1",
				Size:          2048,
				Provider:      "modelscoped",
				Tags: map[string]string{
					tags.JujuController: testing.ControllerTag.Id(),
					tags.JujuModel:      testing.ModelTag.Id(),
				},
			}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestRemoveFilesystemParams(c *gc.C) {
	s.setupFilesystems(c)

	f, release := s.NewFactory(c, s.st.ModelUUID())
	defer release()

	// Deploy an application that will create a storage instance,
	// so we can release the storage and show the effects on the
	// RemoveFilesystemParams.
	application := f.MakeApplication(c, &factory.ApplicationParams{
		Charm: f.MakeCharm(c, &factory.CharmParams{
			Name: "storage-filesystem",
		}),
		Storage: map[string]state.StorageConstraints{
			"data": {
				Count: 1,
				Size:  1,
				Pool:  "modelscoped",
			},
		},
	})
	unit := f.MakeUnit(c, &factory.UnitParams{
		Application: application,
	})
	testStorage, err := s.storageBackend.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testStorage, gc.HasLen, 1)
	storageFilesystem, err := s.storageBackend.StorageInstanceFilesystem(testStorage[0].StorageTag())
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.SetFilesystemInfo(storageFilesystem.FilesystemTag(), state.FilesystemInfo{
		FilesystemId: "zing",
		Size:         1,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Make filesystems 0/0 and 1 Dead.
	for _, filesystemId := range []string{"0/0", "1"} {
		filesystemTag := names.NewFilesystemTag(filesystemId)
		machineTag := names.NewMachineTag("0")
		err = s.storageBackend.DestroyFilesystem(filesystemTag, false)
		c.Assert(err, jc.ErrorIsNil)
		err = s.storageBackend.DetachFilesystem(machineTag, filesystemTag)
		c.Assert(err, jc.ErrorIsNil)
		err = s.storageBackend.RemoveFilesystemAttachment(machineTag, filesystemTag, false)
		c.Assert(err, jc.ErrorIsNil)
	}

	// Make the "data" storage filesystem Dead, releasing.
	err = unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.ReleaseStorageInstance(testStorage[0].StorageTag(), true, false, dontWait)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.DetachStorage(testStorage[0].StorageTag(), unit.UnitTag(), false, dontWait)
	c.Assert(err, jc.ErrorIsNil)
	unitMachineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	unitMachineTag := names.NewMachineTag(unitMachineId)
	err = s.storageBackend.DetachFilesystem(unitMachineTag, storageFilesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveFilesystemAttachment(unitMachineTag, storageFilesystem.FilesystemTag(), false)
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.api.RemoveFilesystemParams(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: "filesystem-0-0"},
			{Tag: storageFilesystem.Tag().String()},
			{Tag: "filesystem-1"},
			{Tag: "filesystem-2"},
			{Tag: "filesystem-42"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.RemoveFilesystemParamsResults{
		Results: []params.RemoveFilesystemParamsResult{{
			Result: params.RemoveFilesystemParams{
				Provider:     "machinescoped",
				FilesystemId: "abc",
				Destroy:      true,
			},
		}, {
			Result: params.RemoveFilesystemParams{
				Provider:     "modelscoped",
				FilesystemId: "zing",
				Destroy:      false,
			},
		}, {
			Error: &params.Error{Message: `filesystem "1" not provisioned`, Code: "not provisioned"},
		}, {
			Error: &params.Error{Message: `filesystem 2 is not dead (alive)`},
		}, {
			Error: &params.Error{Message: "permission denied", Code: "unauthorized access"},
		}},
	})
}

func (s *iaasProvisionerSuite) TestVolumeAttachmentParams(c *gc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)

	err := s.storageBackend.SetVolumeInfo(names.NewVolumeTag("3"), state.VolumeInfo{
		HardwareId: "123",
		VolumeId:   "xyz",
		Size:       1024,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.storageBackend.SetVolumeAttachmentInfo(
		names.NewMachineTag("0"),
		names.NewVolumeTag("3"),
		state.VolumeAttachmentInfo{
			DeviceName: "xvdf1",
			ReadOnly:   true,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

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

func (s *iaasProvisionerSuite) TestFilesystemAttachmentParams(c *gc.C) {
	s.setupFilesystems(c)

	err := s.storageBackend.SetFilesystemInfo(names.NewFilesystemTag("1"), state.FilesystemInfo{
		FilesystemId: "fsid",
		Size:         1024,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.storageBackend.SetFilesystemAttachmentInfo(
		names.NewMachineTag("0"),
		names.NewFilesystemTag("1"),
		state.FilesystemAttachmentInfo{
			MountPoint: "/in/the/place",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

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

func (s *iaasProvisionerSuite) TestSetVolumeAttachmentInfo(c *gc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)

	err := s.storageBackend.SetVolumeInfo(names.NewVolumeTag("4"), state.VolumeInfo{
		VolumeId: "whatever",
		Size:     1024,
	})
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{},
			{Error: &params.Error{Message: `cannot set info for volume attachment 1:0: volume "1" not provisioned`, Code: "not provisioned"}},
			{Error: &params.Error{Message: `cannot set info for volume attachment 4:2: machine 2 not provisioned`, Code: "not provisioned"}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestSetFilesystemAttachmentInfo(c *gc.C) {
	s.setupFilesystems(c)

	err := s.storageBackend.SetFilesystemInfo(names.NewFilesystemTag("3"), state.FilesystemInfo{
		FilesystemId: "whatever",
		Size:         1024,
	})
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{},
			{Error: &params.Error{Message: `cannot set info for filesystem attachment 1:0: filesystem "1" not provisioned`, Code: "not provisioned"}},
			{Error: &params.Error{Message: `cannot set info for filesystem attachment 3:2: machine 2 not provisioned`, Code: "not provisioned"}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestWatchVolumes(c *gc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)

	f, release := s.NewFactory(c, s.st.ModelUUID())
	defer release()

	f.MakeMachine(c, nil)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: names.NewModelTag(s.st.ModelUUID()).String()},
		{Tag: "environ-adb650da-b77b-4ee8-9cbb-d57a9a592847"},
		{Tag: "machine-1"},
		{Tag: "machine-42"}},
	}
	result, err := s.api.WatchVolumes(context.Background(), args)
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
	defer workertest.CleanKill(c, v0Watcher)
	v1Watcher := s.resources.Get("2")
	defer workertest.CleanKill(c, v1Watcher)

	// Check that the Watch has consumed the initial events ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, v0Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
	wc = statetesting.NewStringsWatcherC(c, v1Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *iaasProvisionerSuite) TestWatchVolumeAttachments(c *gc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)

	f, release := s.NewFactory(c, s.st.ModelUUID())
	defer release()

	f.MakeMachine(c, nil)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: names.NewModelTag(s.st.ModelUUID()).String()},
		{Tag: "environ-adb650da-b77b-4ee8-9cbb-d57a9a592847"},
		{Tag: "machine-1"},
		{Tag: "machine-42"}},
	}
	result, err := s.api.WatchVolumeAttachments(context.Background(), args)
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
	defer workertest.CleanKill(c, v0Watcher)
	v1Watcher := s.resources.Get("2")
	defer workertest.CleanKill(c, v1Watcher)

	// Check that the Watch has consumed the initial events ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, v0Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
	wc = statetesting.NewStringsWatcherC(c, v1Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *iaasProvisionerSuite) TestWatchFilesystems(c *gc.C) {
	s.setupFilesystems(c)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: names.NewModelTag(s.st.ModelUUID()).String()},
		{Tag: "environ-adb650da-b77b-4ee8-9cbb-d57a9a592847"},
		{Tag: "machine-1"},
		{Tag: "machine-42"}},
	}
	result, err := s.api.WatchFilesystems(context.Background(), args)
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
	defer workertest.CleanKill(c, v0Watcher)
	v1Watcher := s.resources.Get("2")
	defer workertest.CleanKill(c, v1Watcher)

	// Check that the Watch has consumed the initial events ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, v0Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
	wc = statetesting.NewStringsWatcherC(c, v1Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *iaasProvisionerSuite) TestWatchFilesystemAttachments(c *gc.C) {
	s.setupFilesystems(c)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: names.NewModelTag(s.st.ModelUUID()).String()},
		{Tag: "environ-adb650da-b77b-4ee8-9cbb-d57a9a592847"},
		{Tag: "machine-1"},
		{Tag: "machine-42"}},
	}
	result, err := s.api.WatchFilesystemAttachments(context.Background(), args)
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
	defer workertest.CleanKill(c, v0Watcher)
	v1Watcher := s.resources.Get("2")
	defer workertest.CleanKill(c, v1Watcher)

	// Check that the Watch has consumed the initial events ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, v0Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
	wc = statetesting.NewStringsWatcherC(c, v1Watcher.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *iaasProvisionerSuite) TestWatchBlockDevices(c *gc.C) {
	f, release := s.NewFactory(c, s.st.ModelUUID())
	defer release()

	f.MakeMachine(c, nil)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
		{Tag: "application-mysql"},
		{Tag: "machine-1"},
		{Tag: "machine-42"}},
	}
	results, err := s.api.WatchBlockDevices(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "1"},
			{Error: &params.Error{Message: `"application-mysql" is not a valid machine tag`}},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resources were registered and stop them when done.
	c.Assert(s.resources.Count(), gc.Equals, 1)
	watcher := s.resources.Get("1")
	defer workertest.CleanKill(c, watcher)

	// Check that the Watch has consumed the initial event.
	wc := statetesting.NewNotifyWatcherC(c, watcher.(state.NotifyWatcher))
	wc.AssertNoChange()

	m, err := s.st.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetMachineBlockDevices(state.BlockDeviceInfo{
		DeviceName: "sda",
		Size:       123,
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *iaasProvisionerSuite) TestVolumeBlockDevices(c *gc.C) {
	s.setupVolumes(c)

	f, release := s.NewFactory(c, s.st.ModelUUID())
	defer release()

	f.MakeMachine(c, nil)

	err := s.storageBackend.SetVolumeAttachmentInfo(
		names.NewMachineTag("0"),
		names.NewVolumeTag("0/0"),
		state.VolumeAttachmentInfo{},
	)
	c.Assert(err, jc.ErrorIsNil)

	machine0, err := s.st.Machine("0")
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
		{MachineTag: "application-mysql", AttachmentTag: "volume-1"},
	}}
	results, err := s.api.VolumeBlockDevices(context.Background(), args)
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
			{Error: &params.Error{Code: params.CodeNotValid, Message: `volume attachment host tag "application-mysql" not valid`}},
		},
	})
}

func (s *iaasProvisionerSuite) TestVolumeBlockDevicesPlanBlockInfoSet(c *gc.C) {
	s.setupVolumes(c)

	f, release := s.NewFactory(c, s.st.ModelUUID())
	defer release()

	f.MakeMachine(c, nil)

	err := s.storageBackend.SetVolumeAttachmentInfo(
		names.NewMachineTag("0"),
		names.NewVolumeTag("0/0"),
		state.VolumeAttachmentInfo{},
	)
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(err, jc.ErrorIsNil)

	// The HardwareId set here should override the HardwareId in the volume info.
	blockInfo := state.BlockDeviceInfo{
		WWN: "testWWN",
		DeviceLinks: []string{
			"/dev/sda", "/dev/mapper/testDevice"},
		HardwareId: "test-id",
	}
	err = s.storageBackend.SetVolumeAttachmentPlanBlockInfo(
		names.NewMachineTag("0"), names.NewVolumeTag("0/0"), blockInfo)
	c.Assert(err, jc.ErrorIsNil)

	machine0, err := s.st.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	err = machine0.SetMachineBlockDevices(state.BlockDeviceInfo{
		DeviceName: "sda",
		Size:       123,
		HardwareId: "test-id",
	})
	c.Assert(err, jc.ErrorIsNil)

	args := params.MachineStorageIds{Ids: []params.MachineStorageId{
		{MachineTag: "machine-0", AttachmentTag: "volume-0-0"},
	}}
	results, err := s.api.VolumeBlockDevices(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.BlockDeviceResults{
		Results: []params.BlockDeviceResult{
			{Result: storage.BlockDevice{
				DeviceName: "sda",
				Size:       123,
				HardwareId: "test-id",
			}},
		},
	})
}

func (s *iaasProvisionerSuite) TestLife(c *gc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)
	args := params.Entities{Entities: []params.Entity{{Tag: "volume-0-0"}, {Tag: "volume-1"}, {Tag: "volume-42"}}}
	result, err := s.api.Life(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: life.Alive},
			{Life: life.Alive},
			{Error: apiservererrors.ServerError(errors.NotFoundf(`volume "42"`))},
		},
	})
}

func (s *iaasProvisionerSuite) TestAttachmentLife(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: life.Alive},
			{Life: life.Alive},
			{Life: life.Alive},
			{Error: &params.Error{Message: `volume "42" on "machine 0" not found`, Code: "not found"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestEnsureDead(c *gc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)
	args := params.Entities{Entities: []params.Entity{{Tag: "volume-0-0"}, {Tag: "volume-1"}, {Tag: "volume-42"}}}
	result, err := s.api.EnsureDead(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	// TODO(wallyworld) - this test will be updated when EnsureDead is supported
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservererrors.ServerError(apiservererrors.NotSupportedError(names.NewVolumeTag("0/0"), "ensuring death"))},
			{Error: apiservererrors.ServerError(apiservererrors.NotSupportedError(names.NewVolumeTag("1"), "ensuring death"))},
			{Error: apiservererrors.ServerError(errors.NotFoundf(`volume "42"`))},
		},
	})
}

func (s *iaasProvisionerSuite) TestRemoveVolumesController(c *gc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)
	args := params.Entities{Entities: []params.Entity{
		{Tag: "volume-1-0"}, {Tag: "volume-1"}, {Tag: "volume-2"}, {Tag: "volume-42"},
		{Tag: "volume-invalid"}, {Tag: "machine-0"},
	}}

	err := s.storageBackend.DetachVolume(names.NewMachineTag("0"), names.NewVolumeTag("1"), false)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveVolumeAttachment(names.NewMachineTag("0"), names.NewVolumeTag("1"), false)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.DestroyVolume(names.NewVolumeTag("1"), false)
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.api.Remove(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
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

func (s *iaasProvisionerSuite) TestRemoveFilesystemsController(c *gc.C) {
	s.setupFilesystems(c)
	args := params.Entities{Entities: []params.Entity{
		{Tag: "filesystem-1-0"}, {Tag: "filesystem-1"}, {Tag: "filesystem-2"}, {Tag: "filesystem-42"},
		{Tag: "filesystem-invalid"}, {Tag: "machine-0"},
	}}

	err := s.storageBackend.DetachFilesystem(names.NewMachineTag("0"), names.NewFilesystemTag("1"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveFilesystemAttachment(names.NewMachineTag("0"), names.NewFilesystemTag("1"), false)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.DestroyFilesystem(names.NewFilesystemTag("1"), false)
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.api.Remove(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ErrorResults{
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

func (s *iaasProvisionerSuite) TestRemoveVolumesMachineAgent(c *gc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)
	s.authorizer.Controller = false
	args := params.Entities{Entities: []params.Entity{
		{Tag: "volume-0-0"}, {Tag: "volume-0-42"}, {Tag: "volume-42"},
		{Tag: "volume-invalid"}, {Tag: "machine-0"},
	}}

	err := s.storageBackend.DestroyVolume(names.NewVolumeTag("0/0"), false)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveVolumeAttachment(names.NewMachineTag("0"), names.NewVolumeTag("0/0"), false)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.DestroyVolume(names.NewVolumeTag("0/0"), false)
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.api.Remove(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: nil},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Error: &params.Error{Message: `"volume-invalid" is not a valid volume tag`}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestRemoveFilesystemsMachineAgent(c *gc.C) {
	s.setupFilesystems(c)
	s.authorizer.Controller = false
	args := params.Entities{Entities: []params.Entity{
		{Tag: "filesystem-0-0"}, {Tag: "filesystem-0-42"}, {Tag: "filesystem-42"},
		{Tag: "filesystem-invalid"}, {Tag: "machine-0"},
	}}

	err := s.storageBackend.DestroyFilesystem(names.NewFilesystemTag("0/0"), false)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveFilesystemAttachment(names.NewMachineTag("0"), names.NewFilesystemTag("0/0"), false)
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.api.Remove(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: nil},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Error: &params.Error{Message: `"filesystem-invalid" is not a valid filesystem tag`}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestRemoveVolumeAttachments(c *gc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)
	s.authorizer.Controller = false

	err := s.storageBackend.DetachVolume(names.NewMachineTag("0"), names.NewVolumeTag("1"), false)
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: "removing attachment of volume 0/0 from machine 0: volume attachment is not dying"}},
			{Error: nil},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Error: &params.Error{Message: `removing attachment of volume 42 from machine 0: volume "42" on "machine 0" not found`, Code: "not found"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestRemoveFilesystemAttachments(c *gc.C) {
	s.setupFilesystems(c)
	s.authorizer.Controller = false

	err := s.storageBackend.DetachFilesystem(names.NewMachineTag("0"), names.NewFilesystemTag("1"))
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: "removing attachment of filesystem 0/0 from machine 0: filesystem attachment is not dying"}},
			{Error: nil},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Error: &params.Error{Message: `removing attachment of filesystem 42 from machine 0: filesystem "42" on "machine 0" not found`, Code: "not found"}},
		},
	})
}
