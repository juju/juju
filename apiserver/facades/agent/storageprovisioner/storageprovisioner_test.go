// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"sort"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/storageprovisioner"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

var _ = gc.Suite(&iaasProvisionerSuite{})
var _ = gc.Suite(&caasProvisionerSuite{})

type iaasProvisionerSuite struct {
	provisionerSuite
}

type caasProvisionerSuite struct {
	provisionerSuite
}

type storageSetUp interface {
	setupVolumes(c *gc.C)
	setupFilesystems(c *gc.C)
}

type provisionerSuite struct {
	// TODO(wallyworld) remove JujuConnSuite
	jujutesting.JujuConnSuite

	storageSetUp

	resources      *common.Resources
	authorizer     *apiservertesting.FakeAuthorizer
	api            *storageprovisioner.StorageProvisionerAPIv4
	storageBackend storageprovisioner.StorageBackend
}

func (s *provisionerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
}

func (s *iaasProvisionerSuite) SetUpTest(c *gc.C) {
	s.provisionerSuite.SetUpTest(c)
	s.provisionerSuite.storageSetUp = s

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	env, err := stateenvirons.GetNewEnvironFunc(environs.New)(s.State)
	c.Assert(err, jc.ErrorIsNil)
	registry := stateenvirons.NewStorageProviderRegistry(env)
	pm := poolmanager.New(state.NewStateSettings(s.State), registry)

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	backend, storageBackend, err := storageprovisioner.NewStateBackends(s.State)
	c.Assert(err, jc.ErrorIsNil)
	s.storageBackend = storageBackend
	v3, err := storageprovisioner.NewStorageProvisionerAPIv3(backend, storageBackend, s.resources, s.authorizer, registry, pm)
	c.Assert(err, jc.ErrorIsNil)
	s.api = storageprovisioner.NewStorageProvisionerAPIv4(v3)
}

func (s *caasProvisionerSuite) SetUpTest(c *gc.C) {
	s.provisionerSuite.SetUpTest(c)
	s.provisionerSuite.storageSetUp = s

	caasSt := s.Factory.MakeCAASModel(c, nil)
	s.AddCleanup(func(_ *gc.C) { caasSt.Close() })
	s.State = caasSt
	var err error
	s.Model, err = caasSt.Model()
	c.Assert(err, jc.ErrorIsNil)

	s.Factory = factory.NewFactory(s.State, s.StatePool)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	broker, err := stateenvirons.GetNewCAASBrokerFunc(caas.New)(s.State)
	c.Assert(err, jc.ErrorIsNil)
	registry := stateenvirons.NewStorageProviderRegistry(broker)
	pm := poolmanager.New(state.NewStateSettings(s.State), registry)

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	backend, storageBackend, err := storageprovisioner.NewStateBackends(s.State)
	c.Assert(err, jc.ErrorIsNil)
	s.storageBackend = storageBackend
	v3, err := storageprovisioner.NewStorageProvisionerAPIv3(backend, storageBackend, s.resources, s.authorizer, registry, pm)
	c.Assert(err, jc.ErrorIsNil)
	s.api = storageprovisioner.NewStorageProvisionerAPIv4(v3)
}

func (s *provisionerSuite) TestNewStorageProvisionerAPINonMachine(c *gc.C) {
	tag := names.NewUnitTag("mysql/0")
	authorizer := &apiservertesting.FakeAuthorizer{Tag: tag}
	backend, storageBackend, err := storageprovisioner.NewStateBackends(s.State)
	c.Assert(err, jc.ErrorIsNil)
	_, err = storageprovisioner.NewStorageProvisionerAPIv3(backend, storageBackend, common.NewResources(), authorizer, nil, nil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *iaasProvisionerSuite) setupVolumes(c *gc.C) {
	s.Factory.MakeMachine(c, &factory.MachineParams{
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
	s.Factory.MakeMachine(c, nil)

	// Make an unprovisioned machine with storage for tests to use.
	// TODO(axw) extend testing/factory to allow creating unprovisioned
	// machines.
	_, err = s.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
		Volumes: []state.HostVolumeParams{
			{Volume: state.VolumeParams{Pool: "modelscoped", Size: 2048}},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *iaasProvisionerSuite) setupFilesystems(c *gc.C) {
	s.Factory.MakeMachine(c, &factory.MachineParams{
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
	s.Factory.MakeMachine(c, nil)

	// Make an unprovisioned machine with storage for tests to use.
	// TODO(axw) extend testing/factory to allow creating unprovisioned
	// machines.
	_, err = s.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
		Filesystems: []state.HostFilesystemParams{{
			Filesystem: state.FilesystemParams{Pool: "modelscoped", Size: 2048},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *caasProvisionerSuite) setupFilesystems(c *gc.C) {
	ch := s.Factory.MakeCharm(c, &factory.CharmParams{
		Name:   "storage-filesystem",
		Series: "kubernetes",
	})
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: ch,
		Name:  "mariadb",
		Storage: map[string]state.StorageConstraints{
			"data":  {Count: 1, Size: 1024},
			"cache": {Count: 2, Size: 1024},
		},
	})
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: app})

	// Only provision the first and third backing volumes.
	err := s.storageBackend.SetVolumeInfo(names.NewVolumeTag("0"), state.VolumeInfo{
		HardwareId: "123",
		VolumeId:   "abc",
		Size:       1024,
		Persistent: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.SetVolumeAttachmentInfo(
		names.NewUnitTag("mariadb/0"),
		names.NewVolumeTag("0"),
		state.VolumeAttachmentInfo{ReadOnly: false},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = s.storageBackend.SetVolumeInfo(names.NewVolumeTag("2"), state.VolumeInfo{
		HardwareId: "456",
		VolumeId:   "def",
		Size:       4096,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.SetVolumeAttachmentInfo(
		names.NewUnitTag("mariadb/0"),
		names.NewVolumeTag("2"),
		state.VolumeAttachmentInfo{ReadOnly: false},
	)
	c.Assert(err, jc.ErrorIsNil)

	// Only provision the first and third filesystems.
	err = s.storageBackend.SetFilesystemInfo(names.NewFilesystemTag("0"), state.FilesystemInfo{
		FilesystemId: "abc",
		Size:         1024,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.SetFilesystemInfo(names.NewFilesystemTag("2"), state.FilesystemInfo{
		FilesystemId: "def",
		Size:         4096,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *iaasProvisionerSuite) TestHostedVolumes(c *gc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)
	s.authorizer.Controller = false

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
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Error: common.ServerError(errors.NotProvisionedf(`volume "1"`))},
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

func (s *provisionerSuite) TestVolumesEmptyArgs(c *gc.C) {
	results, err := s.api.Volumes(params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)
}

func (s *iaasProvisionerSuite) TestFilesystems(c *gc.C) {
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
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Error: common.ServerError(errors.NotProvisionedf(`filesystem "1"`))},
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
				Message: `filesystem attachment "2" on "machine 0" not provisioned`,
			}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestVolumeParams(c *gc.C) {
	// Only IAAS models support block storage right now.
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

func (s *provisionerSuite) TestVolumeParamsEmptyArgs(c *gc.C) {
	results, err := s.api.VolumeParams(params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)
}

func (s *iaasProvisionerSuite) TestRemoveVolumeParams(c *gc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)

	// Deploy an application that will create a storage instance,
	// so we can release the storage and show the effects on the
	// RemoveVolumeParams.
	application := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
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
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: application,
	})
	storage, err := s.storageBackend.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storage, gc.HasLen, 1)
	storageVolume, err := s.storageBackend.StorageInstanceVolume(storage[0].StorageTag())
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
		err = s.storageBackend.DestroyVolume(volumeTag)
		c.Assert(err, jc.ErrorIsNil)
		err = s.storageBackend.DetachVolume(machineTag, volumeTag)
		c.Assert(err, jc.ErrorIsNil)
		err = s.storageBackend.RemoveVolumeAttachment(machineTag, volumeTag)
		c.Assert(err, jc.ErrorIsNil)
	}

	// Make the "data" storage volume Dead, releasing.
	err = unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.ReleaseStorageInstance(storage[0].StorageTag(), true)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.DetachStorage(storage[0].StorageTag(), unit.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	unitMachineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	unitMachineTag := names.NewMachineTag(unitMachineId)
	err = s.storageBackend.DetachVolume(unitMachineTag, storageVolume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveVolumeAttachment(unitMachineTag, storageVolume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.api.RemoveVolumeParams(params.Entities{
		Entities: []params.Entity{
			{"volume-0-0"},
			{storageVolume.Tag().String()},
			{"volume-1"},
			{"volume-3"},
			{"volume-42"},
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

	// Deploy an application that will create a storage instance,
	// so we can release the storage and show the effects on the
	// RemoveFilesystemParams.
	application := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
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
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: application,
	})
	storage, err := s.storageBackend.AllStorageInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storage, gc.HasLen, 1)
	storageFilesystem, err := s.storageBackend.StorageInstanceFilesystem(storage[0].StorageTag())
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
		err = s.storageBackend.DestroyFilesystem(filesystemTag)
		c.Assert(err, jc.ErrorIsNil)
		err = s.storageBackend.DetachFilesystem(machineTag, filesystemTag)
		c.Assert(err, jc.ErrorIsNil)
		err = s.storageBackend.RemoveFilesystemAttachment(machineTag, filesystemTag)
		c.Assert(err, jc.ErrorIsNil)
	}

	// Make the "data" storage filesystem Dead, releasing.
	err = unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.ReleaseStorageInstance(storage[0].StorageTag(), true)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.DetachStorage(storage[0].StorageTag(), unit.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	unitMachineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	unitMachineTag := names.NewMachineTag(unitMachineId)
	err = s.storageBackend.DetachFilesystem(unitMachineTag, storageFilesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveFilesystemAttachment(unitMachineTag, storageFilesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.api.RemoveFilesystemParams(params.Entities{
		Entities: []params.Entity{
			{"filesystem-0-0"},
			{storageFilesystem.Tag().String()},
			{"filesystem-1"},
			{"filesystem-2"},
			{"filesystem-42"},
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
			{Error: &params.Error{Message: `cannot set info for filesystem attachment 1:0: filesystem "1" not provisioned`, Code: "not provisioned"}},
			{Error: &params.Error{Message: `cannot set info for filesystem attachment 3:2: machine 2 not provisioned`, Code: "not provisioned"}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *caasProvisionerSuite) TestWatchApplications(c *gc.C) {
	ch := s.Factory.MakeCharm(c, &factory.CharmParams{
		Name:   "storage-filesystem",
		Series: "kubernetes",
	})
	s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: ch,
		Name:  "mariadb",
		Storage: map[string]state.StorageConstraints{
			"data": {Count: 1, Size: 1024},
		},
	})

	result, err := s.api.WatchApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.StringsWatcherId, gc.Equals, "1")
	c.Assert(result.Changes, jc.DeepEquals, []string{"mariadb"})

	w := s.resources.Get("1").(state.StringsWatcher)
	s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: ch,
		Name:  "mysql",
		Storage: map[string]state.StorageConstraints{
			"data": {Count: 1, Size: 1024},
		},
	})
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange("mysql")
}

func (s *iaasProvisionerSuite) TestWatchVolumes(c *gc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)
	s.Factory.MakeMachine(c, nil)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{"machine-0"},
		{s.Model.ModelTag().String()},
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

func (s *iaasProvisionerSuite) TestWatchVolumeAttachments(c *gc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)
	s.Factory.MakeMachine(c, nil)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{"machine-0"},
		{s.Model.ModelTag().String()},
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

func (s *iaasProvisionerSuite) TestWatchFilesystems(c *gc.C) {
	s.setupFilesystems(c)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{"machine-0"},
		{s.Model.ModelTag().String()},
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

func (s *iaasProvisionerSuite) TestWatchFilesystemAttachments(c *gc.C) {
	s.setupFilesystems(c)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{"machine-0"},
		{s.Model.ModelTag().String()},
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

func (s *iaasProvisionerSuite) TestWatchBlockDevices(c *gc.C) {
	s.Factory.MakeMachine(c, nil)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{"machine-0"},
		{"application-mysql"},
		{"machine-1"},
		{"machine-42"}},
	}
	results, err := s.api.WatchBlockDevices(args)
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

func (s *iaasProvisionerSuite) TestVolumeBlockDevices(c *gc.C) {
	s.setupVolumes(c)
	s.Factory.MakeMachine(c, nil)

	err := s.storageBackend.SetVolumeAttachmentInfo(
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
		{MachineTag: "application-mysql", AttachmentTag: "volume-1"},
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
			{Error: &params.Error{Message: `volume attachment host tag "application-mysql" not valid`}},
		},
	})
}

func (s *iaasProvisionerSuite) TestVolumeBlockDevicesPlanBlockInfoSet(c *gc.C) {
	s.setupVolumes(c)
	s.Factory.MakeMachine(c, nil)

	err := s.storageBackend.SetVolumeAttachmentInfo(
		names.NewMachineTag("0"),
		names.NewVolumeTag("0/0"),
		state.VolumeAttachmentInfo{},
	)

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

	machine0, err := s.State.Machine("0")
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
	results, err := s.api.VolumeBlockDevices(args)
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

func (s *iaasProvisionerSuite) TestAttachmentLife(c *gc.C) {
	// Only IAAS models support block storage right now.
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
			{Error: &params.Error{Message: `volume "42" on "machine 0" not found`, Code: "not found"}},
		},
	})
}

func (s *iaasProvisionerSuite) TestEnsureDead(c *gc.C) {
	// Only IAAS models support block storage right now.
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

func (s *iaasProvisionerSuite) TestRemoveVolumesController(c *gc.C) {
	// Only IAAS models support block storage right now.
	s.setupVolumes(c)
	args := params.Entities{Entities: []params.Entity{
		{"volume-1-0"}, {"volume-1"}, {"volume-2"}, {"volume-42"},
		{"volume-invalid"}, {"machine-0"},
	}}

	err := s.storageBackend.DetachVolume(names.NewMachineTag("0"), names.NewVolumeTag("1"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveVolumeAttachment(names.NewMachineTag("0"), names.NewVolumeTag("1"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.DestroyVolume(names.NewVolumeTag("1"))
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.api.Remove(args)
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
		{"filesystem-1-0"}, {"filesystem-1"}, {"filesystem-2"}, {"filesystem-42"},
		{"filesystem-invalid"}, {"machine-0"},
	}}

	err := s.storageBackend.DetachFilesystem(names.NewMachineTag("0"), names.NewFilesystemTag("1"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveFilesystemAttachment(names.NewMachineTag("0"), names.NewFilesystemTag("1"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.DestroyFilesystem(names.NewFilesystemTag("1"))
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.api.Remove(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Error: nil},
			{Error: &params.Error{Message: "removing filesystem 2: filesystem is not dead"}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
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
		{"volume-0-0"}, {"volume-0-42"}, {"volume-42"},
		{"volume-invalid"}, {"machine-0"},
	}}

	err := s.storageBackend.DestroyVolume(names.NewVolumeTag("0/0"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveVolumeAttachment(names.NewMachineTag("0"), names.NewVolumeTag("0/0"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.DestroyVolume(names.NewVolumeTag("0/0"))
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.api.Remove(args)
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
		{"filesystem-0-0"}, {"filesystem-0-42"}, {"filesystem-42"},
		{"filesystem-invalid"}, {"machine-0"},
	}}

	err := s.storageBackend.DestroyFilesystem(names.NewFilesystemTag("0/0"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveFilesystemAttachment(names.NewMachineTag("0"), names.NewFilesystemTag("0/0"))
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.api.Remove(args)
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

	err := s.storageBackend.DetachVolume(names.NewMachineTag("0"), names.NewVolumeTag("1"))
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
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Error: &params.Error{Message: `removing attachment of filesystem 42 from machine 0: filesystem "42" on "machine 0" not found`, Code: "not found"}},
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

func (s *caasProvisionerSuite) TestWatchFilesystemAttachments(c *gc.C) {
	s.setupFilesystems(c)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{"application-mariadb"},
		{s.Model.ModelTag().String()},
		{"environ-adb650da-b77b-4ee8-9cbb-d57a9a592847"},
		{"unit-mysql-0"}},
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
					MachineTag:    "unit-mariadb-0",
					AttachmentTag: "filesystem-0",
				}, {
					MachineTag:    "unit-mariadb-0",
					AttachmentTag: "filesystem-1",
				}, {
					MachineTag:    "unit-mariadb-0",
					AttachmentTag: "filesystem-2",
				}},
			}, {
				MachineStorageIdsWatcherId: "2",
				Changes:                    []params.MachineStorageId{},
			},
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

func (s *caasProvisionerSuite) TestRemoveFilesystemAttachments(c *gc.C) {
	s.setupFilesystems(c)

	err := s.storageBackend.DetachFilesystem(names.NewUnitTag("mariadb/0"), names.NewFilesystemTag("1"))
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.api.RemoveAttachment(params.MachineStorageIds{
		Ids: []params.MachineStorageId{{
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "filesystem-0",
		}, {
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "filesystem-1",
		}, {
			MachineTag:    "unit-mysql-2",
			AttachmentTag: "filesystem-4",
		}, {
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "filesystem-42",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: "removing attachment of filesystem 0 from unit mariadb/0: filesystem attachment is not dying"}},
			{Error: nil},
			{Error: &params.Error{Message: `removing attachment of filesystem 4 from unit mysql/2: filesystem "4" on "unit mysql/2" not found`, Code: "not found"}},
			{Error: &params.Error{Message: `removing attachment of filesystem 42 from unit mariadb/0: filesystem "42" on "unit mariadb/0" not found`, Code: "not found"}},
		},
	})
}

func (s *caasProvisionerSuite) TestRemoveFilesystemsApplicationAgent(c *gc.C) {
	s.setupFilesystems(c)
	s.authorizer.Controller = false
	args := params.Entities{Entities: []params.Entity{
		{"filesystem-42"},
		{"filesystem-invalid"}, {"machine-0"},
	}}

	err := s.storageBackend.DestroyFilesystem(names.NewFilesystemTag("0"))
	c.Assert(err, gc.ErrorMatches, "destroying filesystem 0: filesystem is assigned to storage cache/0")
	err = s.storageBackend.RemoveFilesystemAttachment(names.NewUnitTag("mariadb/0"), names.NewFilesystemTag("0"))
	c.Assert(err, gc.ErrorMatches, "removing attachment of filesystem 0 from unit mariadb/0: filesystem attachment is not dying")

	result, err := s.api.Remove(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Error: &params.Error{Message: `"filesystem-invalid" is not a valid filesystem tag`}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *caasProvisionerSuite) TestFilesystemLife(c *gc.C) {
	s.setupFilesystems(c)
	args := params.Entities{Entities: []params.Entity{{"filesystem-0"}, {"filesystem-1"}, {"filesystem-42"}}}
	result, err := s.api.Life(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: params.Alive},
			{Life: params.Alive},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s caasProvisionerSuite) TestFilesystemAttachmentLife(c *gc.C) {
	s.setupFilesystems(c)

	results, err := s.api.AttachmentLife(params.MachineStorageIds{
		Ids: []params.MachineStorageId{{
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "filesystem-0",
		}, {
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "filesystem-1",
		}, {
			MachineTag:    "unit-mariadb-0",
			AttachmentTag: "filesystem-42",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: params.Alive},
			{Life: params.Alive},
			{Error: &params.Error{Message: `filesystem "42" on "unit mariadb/0" not found`, Code: "not found"}},
		},
	})
}
