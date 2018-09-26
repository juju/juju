// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner_test

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/caasunitprovisioner"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&CAASProvisionerSuite{})

type CAASProvisionerSuite struct {
	coretesting.BaseSuite

	clock                   clock.Clock
	st                      *mockState
	storage                 *mockStorage
	storageProviderRegistry *mockStorageProviderRegistry
	storagePoolManager      *mockStoragePoolManager
	devices                 *mockDeviceBackend
	applicationsChanges     chan []string
	podSpecChanges          chan struct{}
	scaleChanges            chan struct{}

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	facade     *caasunitprovisioner.Facade
}

func (s *CAASProvisionerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.applicationsChanges = make(chan []string, 1)
	s.podSpecChanges = make(chan struct{}, 1)
	s.scaleChanges = make(chan struct{}, 1)
	s.st = &mockState{
		application: mockApplication{
			tag:          names.NewApplicationTag("gitlab"),
			life:         state.Alive,
			scaleWatcher: statetesting.NewMockNotifyWatcher(s.scaleChanges),
		},
		applicationsWatcher: statetesting.NewMockStringsWatcher(s.applicationsChanges),
		model: mockModel{
			podSpecWatcher: statetesting.NewMockNotifyWatcher(s.podSpecChanges),
		},
		unit: mockUnit{
			life: state.Dying,
		},
	}
	s.storage = &mockStorage{
		storageFilesystems: make(map[names.StorageTag]names.FilesystemTag),
		storageVolumes:     make(map[names.StorageTag]names.VolumeTag),
		storageAttachments: make(map[names.UnitTag]names.StorageTag),
	}
	s.storageProviderRegistry = &mockStorageProviderRegistry{}
	s.storagePoolManager = &mockStoragePoolManager{}
	s.devices = &mockDeviceBackend{}
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.st.applicationsWatcher) })
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.st.application.scaleWatcher) })
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.st.model.podSpecWatcher) })

	s.resources = common.NewResources()
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	s.clock = testclock.NewClock(time.Now())

	facade, err := caasunitprovisioner.NewFacade(
		s.resources, s.authorizer, s.st, s.storage, s.devices, s.storageProviderRegistry, s.storagePoolManager, s.clock)
	c.Assert(err, jc.ErrorIsNil)
	s.facade = facade
}

func (s *CAASProvisionerSuite) TestPermission(c *gc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	_, err := caasunitprovisioner.NewFacade(
		s.resources, s.authorizer, s.st, s.storage, s.devices, s.storageProviderRegistry, s.storagePoolManager, s.clock)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *CAASProvisionerSuite) TestWatchApplications(c *gc.C) {
	applicationNames := []string{"db2", "hadoop"}
	s.applicationsChanges <- applicationNames
	result, err := s.facade.WatchApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.StringsWatcherId, gc.Equals, "1")
	c.Assert(result.Changes, jc.DeepEquals, applicationNames)

	resource := s.resources.Get("1")
	c.Assert(resource, gc.Equals, s.st.applicationsWatcher)
}

func (s *CAASProvisionerSuite) TestWatchPodSpec(c *gc.C) {
	s.podSpecChanges <- struct{}{}

	results, err := s.facade.WatchPodSpec(params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
			{Tag: "unit-gitlab-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[1].Error, jc.DeepEquals, &params.Error{
		Message: `"unit-gitlab-0" is not a valid application tag`,
	})

	c.Assert(results.Results[0].NotifyWatcherId, gc.Equals, "1")
	resource := s.resources.Get("1")
	c.Assert(resource, gc.Equals, s.st.model.podSpecWatcher)
}

func (s *CAASProvisionerSuite) TestWatchApplicationsScale(c *gc.C) {
	s.scaleChanges <- struct{}{}

	results, err := s.facade.WatchApplicationsScale(params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
			{Tag: "unit-gitlab-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[1].Error, jc.DeepEquals, &params.Error{
		Message: `"unit-gitlab-0" is not a valid application tag`,
	})

	c.Assert(results.Results[0].NotifyWatcherId, gc.Equals, "1")
	resource := s.resources.Get("1")
	c.Assert(resource, gc.Equals, s.st.application.scaleWatcher)
}

func (s *CAASProvisionerSuite) TestProvisioningInfo(c *gc.C) {
	s.st.application.units = []caasunitprovisioner.Unit{
		&mockUnit{name: "gitlab/0", life: state.Dying},
		&mockUnit{name: "gitlab/1", life: state.Alive},
	}
	s.storage.storageFilesystems[names.NewStorageTag("data/0")] = names.NewFilesystemTag("gitlab/1/0")
	s.storage.storageAttachments[names.NewUnitTag("gitlab/1")] = names.NewStorageTag("data/0")

	results, err := s.facade.ProvisioningInfo(params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
			{Tag: "unit-gitlab-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.KubernetesProvisioningInfoResults{
		Results: []params.KubernetesProvisioningInfoResult{{
			Result: &params.KubernetesProvisioningInfo{
				PodSpec: "spec(gitlab)",
				Filesystems: []params.KubernetesFilesystemParams{{
					StorageName: "data",
					Provider:    string(provider.K8s_ProviderType),
					Size:        100,
					Attributes:  map[string]interface{}{"foo": "bar"},
					Tags: map[string]string{
						"juju-storage-instance": "data/0",
						"juju-storage-owner":    "gitlab",
						"juju-model-uuid":       coretesting.ModelTag.Id(),
						"juju-controller-uuid":  coretesting.ControllerTag.Id()},
					Attachment: &params.KubernetesFilesystemAttachmentParams{
						Provider:   string(provider.K8s_ProviderType),
						MountPoint: "/path/to/here",
						ReadOnly:   true,
					},
				}},
				Devices: []params.KubernetesDeviceParams{
					{
						Type:       "nvidia.com/gpu",
						Count:      3,
						Attributes: map[string]string{"gpu": "nvidia-tesla-p100"},
					},
				},
				Placement:   "placement",
				Constraints: constraints.MustParse("mem=64G"),
				Tags: map[string]string{
					"juju-model-uuid":      coretesting.ModelTag.Id(),
					"juju-controller-uuid": coretesting.ControllerTag.Id()},
			},
		}, {
			Error: &params.Error{
				Message: `"unit-gitlab-0" is not a valid application tag`,
			},
		}},
	})
	s.st.CheckCallNames(c, "Model", "Application", "ControllerConfig")
	s.storage.CheckCallNames(c, "UnitStorageAttachments", "StorageInstance", "FilesystemAttachment")
	s.storageProviderRegistry.CheckNoCalls(c)
	s.storagePoolManager.CheckCallNames(c, "Get")
}

func (s *CAASProvisionerSuite) TestApplicationScale(c *gc.C) {
	results, err := s.facade.ApplicationsScale(params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
			{Tag: "unit-gitlab-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.IntResults{
		Results: []params.IntResult{{
			Result: 5,
		}, {
			Error: &params.Error{
				Message: `"unit-gitlab-0" is not a valid application tag`,
			},
		}},
	})
	s.st.CheckCallNames(c, "Application")
}

func (s *CAASProvisionerSuite) TestLife(c *gc.C) {
	results, err := s.facade.Life(params.Entities{
		Entities: []params.Entity{
			{Tag: "unit-gitlab-0"},
			{Tag: "application-gitlab"},
			{Tag: "machine-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{{
			Life: params.Dying,
		}, {
			Life: params.Alive,
		}, {
			Error: &params.Error{
				Code:    "unauthorized access",
				Message: "permission denied",
			},
		}},
	})
}

func (s *CAASProvisionerSuite) TestApplicationConfig(c *gc.C) {
	results, err := s.facade.ApplicationsConfig(params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
			{Tag: "unit-gitlab-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[1].Error, jc.DeepEquals, &params.Error{
		Message: `"unit-gitlab-0" is not a valid application tag`,
	})
	c.Assert(results.Results[0].Config, jc.DeepEquals, map[string]interface{}{"foo": "bar"})
}

func strPtr(s string) *string {
	return &s
}

func (s *CAASProvisionerSuite) TestUpdateApplicationsUnits(c *gc.C) {
	s.st.application.units = []caasunitprovisioner.Unit{
		&mockUnit{name: "gitlab/0", containerInfo: &mockContainerInfo{providerId: "uuid"}, life: state.Alive},
		&mockUnit{name: "gitlab/1", life: state.Alive},
		&mockUnit{name: "gitlab/2", containerInfo: &mockContainerInfo{providerId: "uuid2"}, life: state.Alive},
		&mockUnit{name: "gitlab/3", life: state.Alive},
	}

	units := []params.ApplicationUnitParams{
		{ProviderId: "uuid", Address: "address", Ports: []string{"port"},
			Status: "allocating", Info: ""},
		{ProviderId: "another-uuid", Address: "another-address", Ports: []string{"another-port"},
			Status: "allocating", Info: "another message"},
		{ProviderId: "new-uuid", Address: "new-address", Ports: []string{"new-port"},
			Status: "running", Info: "new message"},
		{ProviderId: "really-new-uuid", Address: "really-new-address", Ports: []string{"really-new-port"},
			Status: "running", Info: "really new message"},
	}
	args := params.UpdateApplicationUnitArgs{
		Args: []params.UpdateApplicationUnits{
			{ApplicationTag: "application-gitlab", Units: units},
			{ApplicationTag: "application-another", Units: []params.ApplicationUnitParams{}},
		},
	}
	results, err := s.facade.UpdateApplicationsUnits(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{&params.Error{Message: "application another not found", Code: "not found"}},
		},
	})
	s.st.application.CheckCallNames(c, "Life", "AddOperation", "Name")
	s.st.application.CheckCall(c, 1, "AddOperation", state.UnitUpdateProperties{
		ProviderId: strPtr("really-new-uuid"),
		Address:    strPtr("really-new-address"), Ports: &[]string{"really-new-port"},
		CloudContainerStatus: &status.StatusInfo{Status: status.Running, Message: "really new message"},
		AgentStatus:          &status.StatusInfo{Status: status.Idle},
	})
	s.st.application.units[0].(*mockUnit).CheckCallNames(c, "Life", "UpdateOperation")
	// CloudContainer message is not overwritten based on agent status
	s.st.application.units[0].(*mockUnit).CheckCall(c, 1, "UpdateOperation", state.UnitUpdateProperties{
		ProviderId: strPtr("uuid"),
		Address:    strPtr("address"), Ports: &[]string{"port"},
		CloudContainerStatus: &status.StatusInfo{Status: status.Waiting, Message: ""},
		AgentStatus:          &status.StatusInfo{Status: status.Allocating},
	})
	s.st.application.units[1].(*mockUnit).CheckCallNames(c, "Life", "UpdateOperation")
	// CloudContainer message is not overwritten based on agent status
	s.st.application.units[1].(*mockUnit).CheckCall(c, 1, "UpdateOperation", state.UnitUpdateProperties{
		ProviderId: strPtr("another-uuid"),
		Address:    strPtr("another-address"), Ports: &[]string{"another-port"},
		CloudContainerStatus: &status.StatusInfo{Status: status.Waiting, Message: "another message"},
		AgentStatus:          &status.StatusInfo{Status: status.Allocating, Message: "another message"},
	})
	s.st.application.units[2].(*mockUnit).CheckCallNames(c, "Life", "DestroyOperation", "UpdateOperation")
	s.st.application.units[2].(*mockUnit).CheckCall(c, 2, "UpdateOperation", state.UnitUpdateProperties{
		AgentStatus:          &status.StatusInfo{Status: status.Idle},
		CloudContainerStatus: &status.StatusInfo{Status: status.Terminated, Message: "unit stopped by the cloud"},
	})
	s.st.application.units[3].(*mockUnit).CheckCallNames(c, "Life", "UpdateOperation")
	s.st.application.units[3].(*mockUnit).CheckCall(c, 1, "UpdateOperation", state.UnitUpdateProperties{
		ProviderId: strPtr("new-uuid"),
		Address:    strPtr("new-address"), Ports: &[]string{"new-port"},
		CloudContainerStatus: &status.StatusInfo{Status: status.Running, Message: "new message"},
		AgentStatus:          &status.StatusInfo{Status: status.Idle},
	})
}

func (s *CAASProvisionerSuite) TestUpdateApplicationsUnitsNotAlive(c *gc.C) {
	s.st.application.units = []caasunitprovisioner.Unit{
		&mockUnit{name: "gitlab/0", life: state.Alive},
		&mockUnit{name: "gitlab/1", life: state.Alive},
		&mockUnit{name: "gitlab/2", containerInfo: &mockContainerInfo{providerId: "uuid2"}, life: state.Alive},
	}
	s.st.application.life = state.Dying

	units := []params.ApplicationUnitParams{
		{ProviderId: "uuid", UnitTag: "unit-gitlab-0", Address: "address", Ports: []string{"port"},
			Status: "running", Info: "message"},
		{ProviderId: "another-uuid", UnitTag: "unit-gitlab-1", Address: "another-address", Ports: []string{"another-port"},
			Status: "error", Info: "another message"},
	}
	args := params.UpdateApplicationUnitArgs{
		Args: []params.UpdateApplicationUnits{
			{ApplicationTag: "application-gitlab", Units: units},
		},
	}
	results, err := s.facade.UpdateApplicationsUnits(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
		},
	})
	s.st.application.CheckCallNames(c, "Life", "Name")
	s.st.application.units[0].(*mockUnit).CheckCallNames(c, "Life")
	s.st.application.units[1].(*mockUnit).CheckCallNames(c, "Life")
	s.st.application.units[2].(*mockUnit).CheckCallNames(c, "Life")
}

func (s *CAASProvisionerSuite) TestUpdateApplicationsUnitsWithStorage(c *gc.C) {
	s.st.application.units = []caasunitprovisioner.Unit{
		&mockUnit{name: "gitlab/0", containerInfo: &mockContainerInfo{providerId: "uuid"}, life: state.Alive},
		&mockUnit{name: "gitlab/1", life: state.Alive},
		&mockUnit{name: "gitlab/2", containerInfo: &mockContainerInfo{providerId: "gone-uuid"}, life: state.Alive},
	}
	s.st.model.containers = []state.CloudContainer{&mockContainerInfo{unitName: "gitlab/1", providerId: "another-uuid"}}
	s.storage.storageFilesystems[names.NewStorageTag("data/0")] = names.NewFilesystemTag("gitlab/0/0")
	s.storage.storageFilesystems[names.NewStorageTag("data/1")] = names.NewFilesystemTag("gitlab/1/0")
	s.storage.storageFilesystems[names.NewStorageTag("data/2")] = names.NewFilesystemTag("gitlab/2/0")
	s.storage.storageVolumes[names.NewStorageTag("data/0")] = names.NewVolumeTag("0")
	s.storage.storageVolumes[names.NewStorageTag("data/1")] = names.NewVolumeTag("1")
	s.storage.storageAttachments[names.NewUnitTag("gitlab/0")] = names.NewStorageTag("data/0")
	s.storage.storageAttachments[names.NewUnitTag("gitlab/1")] = names.NewStorageTag("data/1")
	s.storage.storageAttachments[names.NewUnitTag("gitlab/2")] = names.NewStorageTag("data/2")

	units := []params.ApplicationUnitParams{
		{ProviderId: "uuid", Address: "address", Ports: []string{"port"},
			Status: "running", Info: "message",
			FilesystemInfo: []params.KubernetesFilesystemInfo{
				{StorageName: "data", FilesystemId: "fs-id", Size: 100, MountPoint: "/path/to/here", ReadOnly: true,
					Status: "pending", Info: "not ready",
					Volume: params.KubernetesVolumeInfo{
						VolumeId: "vol-id", Size: 100, Persistent: true,
						Status: "pending", Info: "vol not ready",
					}},
			},
		},
		{ProviderId: "another-uuid", Address: "another-address", Ports: []string{"another-port"},
			Status: "running", Info: "another message",
			FilesystemInfo: []params.KubernetesFilesystemInfo{
				{StorageName: "data", FilesystemId: "fs-id2", Size: 200, MountPoint: "/path/to/there", ReadOnly: true,
					Status: "attached", Info: "ready",
					Volume: params.KubernetesVolumeInfo{
						VolumeId: "vol-id2", Size: 200, Persistent: true,
						Status: "attached", Info: "vol ready",
					}},
			},
		},
	}
	args := params.UpdateApplicationUnitArgs{
		Args: []params.UpdateApplicationUnits{
			{ApplicationTag: "application-gitlab", Units: units},
		},
	}
	results, err := s.facade.UpdateApplicationsUnits(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
		},
	})
	s.st.application.CheckCallNames(c, "Life", "Name")
	s.st.application.units[0].(*mockUnit).CheckCallNames(c, "Life", "UpdateOperation")
	s.st.application.units[0].(*mockUnit).CheckCall(c, 1, "UpdateOperation", state.UnitUpdateProperties{
		ProviderId: strPtr("uuid"),
		Address:    strPtr("address"), Ports: &[]string{"port"},
		CloudContainerStatus: &status.StatusInfo{Status: status.Running, Message: "message"},
		AgentStatus:          &status.StatusInfo{Status: status.Idle},
	})
	s.st.application.units[1].(*mockUnit).CheckCallNames(c, "Life", "UpdateOperation")
	s.st.application.units[1].(*mockUnit).CheckCall(c, 1, "UpdateOperation", state.UnitUpdateProperties{
		ProviderId: strPtr("another-uuid"),
		Address:    strPtr("another-address"), Ports: &[]string{"another-port"},
		CloudContainerStatus: &status.StatusInfo{Status: status.Running, Message: "another message"},
		AgentStatus:          &status.StatusInfo{Status: status.Idle},
	})
	s.storage.CheckCallNames(c,
		"UnitStorageAttachments", "UnitStorageAttachments", "StorageInstance",
		"UnitStorageAttachments", "StorageInstance", "AllFilesystems",
		"Volume", "SetVolumeInfo", "SetVolumeAttachmentInfo", "Volume", "SetStatus", "Volume", "SetStatus",
		"Filesystem", "SetFilesystemInfo", "SetFilesystemAttachmentInfo",
		"Filesystem", "SetStatus", "Filesystem", "SetStatus", "Filesystem", "SetStatus")
	s.storage.CheckCall(c, 0, "UnitStorageAttachments", names.NewUnitTag("gitlab/2"))
	s.storage.CheckCall(c, 1, "UnitStorageAttachments", names.NewUnitTag("gitlab/0"))
	s.storage.CheckCall(c, 2, "StorageInstance", names.NewStorageTag("data/0"))
	s.storage.CheckCall(c, 3, "UnitStorageAttachments", names.NewUnitTag("gitlab/1"))
	s.storage.CheckCall(c, 4, "StorageInstance", names.NewStorageTag("data/1"))

	now := s.clock.Now()
	s.storage.CheckCall(c, 7, "SetVolumeInfo",
		names.NewVolumeTag("1"),
		state.VolumeInfo{
			Size:       200,
			VolumeId:   "vol-id2",
			Persistent: true,
		})
	s.storage.CheckCall(c, 8, "SetVolumeAttachmentInfo",
		names.NewUnitTag("gitlab/1"), names.NewVolumeTag("1"),
		state.VolumeAttachmentInfo{
			ReadOnly: true,
		})
	s.storage.CheckCall(c, 10, "SetStatus",
		status.StatusInfo{
			Status:  status.Pending,
			Message: "vol not ready",
			Since:   &now,
		})
	s.storage.CheckCall(c, 12, "SetStatus",
		status.StatusInfo{
			Status:  status.Attached,
			Message: "vol ready",
			Since:   &now,
		})

	s.storage.CheckCall(c, 14, "SetFilesystemInfo",
		names.NewFilesystemTag("gitlab/1/0"),
		state.FilesystemInfo{
			Size:         200,
			FilesystemId: "fs-id2",
		})
	s.storage.CheckCall(c, 15, "SetFilesystemAttachmentInfo",
		names.NewUnitTag("gitlab/1"), names.NewFilesystemTag("gitlab/1/0"),
		state.FilesystemAttachmentInfo{
			MountPoint: "/path/to/there",
			ReadOnly:   true,
		})
	s.storage.CheckCall(c, 19, "SetStatus",
		status.StatusInfo{
			Status:  status.Attached,
			Message: "ready",
			Since:   &now,
		})
	s.storage.CheckCall(c, 21, "SetStatus",
		status.StatusInfo{
			Status: status.Detached,
			Since:  &now,
		})
}

func (s *CAASProvisionerSuite) TestUpdateApplicationsService(c *gc.C) {
	results, err := s.facade.UpdateApplicationsService(params.UpdateApplicationServiceArgs{
		Args: []params.UpdateApplicationServiceArg{
			{ApplicationTag: "application-gitlab", ProviderId: "id", Addresses: []params.Address{{Value: "10.0.0.1"}}},
			{ApplicationTag: "unit-gitlab-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[1].Error, jc.DeepEquals, &params.Error{
		Message: `"unit-gitlab-0" is not a valid application tag`,
	})
	c.Assert(s.st.application.providerId, gc.Equals, "id")
	c.Assert(s.st.application.addresses, jc.DeepEquals, []network.Address{{Value: "10.0.0.1"}})
}

func (s *CAASProvisionerSuite) TestSetOperatorStatus(c *gc.C) {
	results, err := s.facade.SetOperatorStatus(params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: "application-gitlab", Status: "error", Info: "broken", Data: map[string]interface{}{"foo": "bar"}},
			{Tag: "unit-gitlab-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[1].Error, jc.DeepEquals, &params.Error{
		Message: `"unit-gitlab-0" is not a valid application tag`,
	})
	now := s.clock.Now()
	s.st.application.CheckCall(c, 0, "SetOperatorStatus", status.StatusInfo{
		Status:  status.Error,
		Message: "broken",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
}
