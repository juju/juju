// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/caasapplicationprovisioner"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/model"
	jujuresource "github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/status"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/application/service"
	envconfig "github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/docker"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

var _ = gc.Suite(&CAASApplicationProvisionerSuite{})

type CAASApplicationProvisionerSuite struct {
	coretesting.BaseSuite
	clock clock.Clock

	resources               *common.Resources
	authorizer              *apiservertesting.FakeAuthorizer
	api                     *caasapplicationprovisioner.API
	st                      *mockState
	storage                 *mockStorage
	storagePoolGetter       *mockStoragePoolGetter
	controllerConfigService *MockControllerConfigService
	modelConfigService      *MockModelConfigService
	modelInfoService        *MockModelInfoService
	applicationService      *MockApplicationService
	leadershipRevoker       *MockRevoker
	registry                *mockStorageRegistry
	store                   *mockObjectStore
}

func (s *CAASApplicationProvisionerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })
	s.PatchValue(&jujuversion.OfficialBuild, 0)

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}

	s.store = &mockObjectStore{}
	s.clock = testclock.NewClock(time.Now())
	s.st = newMockState()
	s.storage = &mockStorage{
		storageFilesystems: make(map[names.StorageTag]names.FilesystemTag),
		storageVolumes:     make(map[names.StorageTag]names.VolumeTag),
		storageAttachments: make(map[names.UnitTag]names.StorageTag),
		backingVolume:      names.NewVolumeTag("66"),
	}
	s.storagePoolGetter = &mockStoragePoolGetter{}
	s.registry = &mockStorageRegistry{}
}

func (s *CAASApplicationProvisionerSuite) setupAPI(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.modelInfoService = NewMockModelInfoService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.leadershipRevoker = NewMockRevoker(ctrl)

	newResourceOpener := func(appName string) (jujuresource.Opener, error) {
		return &mockResourceOpener{appName: appName, resources: s.st.resource}, nil
	}
	api, err := caasapplicationprovisioner.NewCAASApplicationProvisionerAPI(
		s.st, s.st,
		s.resources, newResourceOpener,
		s.authorizer,
		s.storage,
		s.storagePoolGetter,
		s.controllerConfigService,
		s.modelConfigService,
		s.modelInfoService,
		s.applicationService,
		s.leadershipRevoker,
		s.registry,
		s.store,
		s.clock,
		loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)
	s.api = api

	return ctrl
}

func (s *CAASApplicationProvisionerSuite) TestPermission(c *gc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	_, err := caasapplicationprovisioner.NewCAASApplicationProvisionerAPI(
		s.st, s.st,
		s.resources, nil,
		s.authorizer,
		s.storage,
		s.storagePoolGetter,
		s.controllerConfigService,
		s.modelConfigService,
		s.modelInfoService,
		s.applicationService,
		s.leadershipRevoker,
		s.registry,
		s.store,
		s.clock,
		loggertesting.WrapCheckLog(c))
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *CAASApplicationProvisionerSuite) TestProvisioningInfo(c *gc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.st.app = &mockApplication{
		life: state.Alive,
		charm: &mockCharm{
			meta: &charm.Meta{},
			url:  "ch:gitlab",
		},
		charmModifiedVersion: 10,
		config: config.ConfigAttributes{
			"trust": true,
		},
	}
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(coretesting.FakeControllerConfig(), nil)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(s.fakeModelConfig())

	modelInfo := model.ReadOnlyModel{
		UUID: model.UUID(coretesting.ModelTag.Id()),
	}
	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(modelInfo, nil)
	s.applicationService.EXPECT().GetApplicationScale(gomock.Any(), "gitlab").Return(3, nil)

	result, err := s.api.ProvisioningInfo(context.Background(), params.Entities{Entities: []params.Entity{{"application-gitlab"}}})
	c.Assert(err, jc.ErrorIsNil)

	mc := jc.NewMultiChecker()
	mc.AddExpr(`_.Results[0].CACert`, jc.Ignore)
	c.Assert(result, mc, params.CAASApplicationProvisioningInfoResults{
		Results: []params.CAASApplicationProvisioningInfo{{
			ImageRepo:    params.DockerImageInfo{RegistryPath: "docker.io/jujusolutions/jujud-operator:2.6-beta3.666"},
			Version:      version.MustParse("2.6-beta3.666"),
			APIAddresses: []string{"10.0.0.1:1"},
			Tags: map[string]string{
				"juju-model-uuid":      coretesting.ModelTag.Id(),
				"juju-controller-uuid": coretesting.ControllerTag.Id(),
			},
			CharmURL:             "ch:gitlab",
			CharmModifiedVersion: 10,
			Scale:                3,
			Trust:                true,
		}},
	})
}

func (s *CAASApplicationProvisionerSuite) TestProvisioningInfoPendingCharmError(c *gc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.st.app = &mockApplication{
		life:         state.Alive,
		charmPending: true,
		charm: &mockCharm{
			meta: &charm.Meta{},
			url:  "ch:gitlab",
		},
	}
	result, err := s.api.ProvisioningInfo(context.Background(), params.Entities{Entities: []params.Entity{{Tag: "application-gitlab"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, `charm "ch:gitlab" pending not provisioned`)
}

func (s *CAASApplicationProvisionerSuite) TestWatchProvisioningInfo(c *gc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	appChanged := make(chan struct{}, 1)
	portsChanged := make(chan struct{}, 1)
	modelConfigChanged := make(chan []string, 1)
	controllerConfigChanged := make(chan []string, 1)
	s.st.apiHostPortsForAgentsWatcher = statetesting.NewMockNotifyWatcher(portsChanged)
	s.controllerConfigService.EXPECT().WatchControllerConfig().Return(watchertest.NewMockStringsWatcher(controllerConfigChanged), nil)
	s.modelConfigService.EXPECT().Watch().Return(watchertest.NewMockStringsWatcher(modelConfigChanged), nil)
	s.st.app = &mockApplication{
		life: state.Alive,
		charm: &mockCharm{
			meta: &charm.Meta{},
			url:  "cs:gitlab",
		},
		watcher: statetesting.NewMockNotifyWatcher(appChanged),
	}
	appChanged <- struct{}{}
	portsChanged <- struct{}{}
	modelConfigChanged <- []string{}
	controllerConfigChanged <- []string{}

	results, err := s.api.WatchProvisioningInfo(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
		},
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	res := s.resources.Get("1")
	c.Assert(res, gc.FitsTypeOf, (*eventsource.MultiWatcher[struct{}])(nil))
}

func (s *CAASApplicationProvisionerSuite) TestSetOperatorStatus(c *gc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.st.app = &mockApplication{
		life: state.Alive,
		charm: &mockCharm{
			meta: &charm.Meta{},
			url:  "ch:gitlab",
		},
	}
	result, err := s.api.SetOperatorStatus(context.Background(), params.SetStatus{
		Entities: []params.EntityStatusArgs{{
			Tag:    "application-gitlab",
			Status: "started",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Error, gc.IsNil)

	s.st.app.CheckCallNames(c, "SetOperatorStatus")
	c.Assert(s.st.app.Calls()[0].Args[0], gc.DeepEquals, status.StatusInfo{Status: "started"})
}

func (s *CAASApplicationProvisionerSuite) TestUnits(c *gc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.st.app = &mockApplication{
		life: state.Alive,
		charm: &mockCharm{
			meta: &charm.Meta{},
			url:  "ch:gitlab",
		},
		units: []*mockUnit{
			{
				tag: names.NewUnitTag("gitlab/0"),
				status: status.StatusInfo{
					Status: status.Active,
				},
			},
			{
				tag: names.NewUnitTag("gitlab/1"),
				status: status.StatusInfo{
					Status: status.Maintenance,
				},
			},
			{
				tag: names.NewUnitTag("gitlab/2"),
				status: status.StatusInfo{
					Status: status.Unknown,
				},
			},
		},
	}
	result, err := s.api.Units(context.Background(), params.Entities{
		Entities: []params.Entity{{
			Tag: "application-gitlab",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].Units, gc.DeepEquals, []params.CAASUnitInfo{
		{
			Tag: "unit-gitlab-0",
			UnitStatus: &params.UnitStatus{
				AgentStatus:    params.DetailedStatus{Status: "active"},
				WorkloadStatus: params.DetailedStatus{Status: "active"},
			},
		},
		{
			Tag: "unit-gitlab-1",
			UnitStatus: &params.UnitStatus{
				AgentStatus:    params.DetailedStatus{Status: "maintenance"},
				WorkloadStatus: params.DetailedStatus{Status: "maintenance"},
			},
		},
		{
			Tag: "unit-gitlab-2",
			UnitStatus: &params.UnitStatus{
				AgentStatus:    params.DetailedStatus{Status: "unknown"},
				WorkloadStatus: params.DetailedStatus{Status: "unknown"},
			},
		},
	})
}

func (s *CAASApplicationProvisionerSuite) TestApplicationOCIResources(c *gc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.st.app = &mockApplication{
		tag:  names.NewApplicationTag("gitlab"),
		life: state.Alive,
		charm: &mockCharm{
			meta: &charm.Meta{
				Resources: map[string]charmresource.Meta{
					"gitlab-image": {
						Name: "gitlab-image",
						Type: charmresource.TypeContainerImage,
					},
				},
			},
			url: "ch:gitlab",
		},
	}
	s.st.resource = &mockResources{
		resource: &docker.DockerImageDetails{
			RegistryPath: "gitlab:latest",
			ImageRepoDetails: docker.ImageRepoDetails{
				BasicAuthConfig: docker.BasicAuthConfig{
					Username: "jujuqa",
					Password: "pwd",
				},
			},
		},
	}

	result, err := s.api.ApplicationOCIResources(context.Background(), params.Entities{
		Entities: []params.Entity{{
			Tag: "application-gitlab",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].Result, gc.DeepEquals, &params.CAASApplicationOCIResources{
		Images: map[string]params.DockerImageInfo{
			"gitlab-image": {
				RegistryPath: "gitlab:latest",
				Username:     "jujuqa",
				Password:     "pwd",
			},
		},
	})
}

func (s *CAASApplicationProvisionerSuite) TestUpdateApplicationsUnitsWithStorage(c *gc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.st.app = &mockApplication{
		tag:  names.NewApplicationTag("gitlab"),
		life: state.Alive,
		charm: &mockCharm{
			meta: &charm.Meta{},
			manifest: &charm.Manifest{
				// charm.FormatV2.
				Bases: []charm.Base{
					{
						Name: "ubuntu",
						Channel: charm.Channel{
							Risk:  "stable",
							Track: "20.04",
						},
					},
				},
			},
			url: "ch:gitlab",
		},
		units: []*mockUnit{
			{
				tag: names.NewUnitTag("gitlab/0"),
				containerInfo: &mockCloudContainer{
					unit:       "gitlab/0",
					providerId: "gitlab-0",
				},
			},
			{
				tag: names.NewUnitTag("gitlab/1"),
				containerInfo: &mockCloudContainer{
					unit:       "gitlab/1",
					providerId: "gitlab-1",
				},
			},
			{
				tag: names.NewUnitTag("gitlab/2"),
				containerInfo: &mockCloudContainer{
					unit:       "gitlab/2",
					providerId: "gitlab-2",
				},
			},
		},
	}
	s.storage.storageFilesystems[names.NewStorageTag("data/0")] = names.NewFilesystemTag("gitlab/0/0")
	s.storage.storageFilesystems[names.NewStorageTag("data/1")] = names.NewFilesystemTag("gitlab/1/0")
	s.storage.storageFilesystems[names.NewStorageTag("data/2")] = names.NewFilesystemTag("gitlab/2/0")
	s.storage.storageVolumes[names.NewStorageTag("data/0")] = names.NewVolumeTag("0")
	s.storage.storageVolumes[names.NewStorageTag("data/1")] = names.NewVolumeTag("1")
	s.storage.storageAttachments[names.NewUnitTag("gitlab/0")] = names.NewStorageTag("data/0")
	s.storage.storageAttachments[names.NewUnitTag("gitlab/1")] = names.NewStorageTag("data/1")
	s.storage.storageAttachments[names.NewUnitTag("gitlab/2")] = names.NewStorageTag("data/2")

	units := []params.ApplicationUnitParams{
		{ProviderId: "gitlab-0", Address: "address", Ports: []string{"port"},
			Status: "running", Info: "message", Stateful: true,
			FilesystemInfo: []params.KubernetesFilesystemInfo{
				{StorageName: "data", FilesystemId: "fs-id", Size: 100, MountPoint: "/path/to/here", ReadOnly: true,
					Status: "pending", Info: "not ready",
					Volume: params.KubernetesVolumeInfo{
						VolumeId: "vol-id", Size: 100, Persistent: true,
						Status: "pending", Info: "vol not ready",
					}},
			},
		},
		{ProviderId: "gitlab-1", Address: "another-address", Ports: []string{"another-port"},
			Status: "running", Info: "another message", Stateful: true,
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
			{
				ApplicationTag: "application-gitlab",
				Units:          units,
				Status:         params.EntityStatus{Status: status.Active, Info: "working"},
			},
		},
	}

	results, err := s.api.UpdateApplicationsUnits(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0], gc.DeepEquals, params.UpdateApplicationUnitResult{
		Info: &params.UpdateApplicationUnitsInfo{
			Units: []params.ApplicationUnitInfo{
				{ProviderId: "gitlab-0", UnitTag: "unit-gitlab-0"},
				{ProviderId: "gitlab-1", UnitTag: "unit-gitlab-1"},
			},
		},
	})
	s.st.app.CheckCallNames(c, "Life", "SetOperatorStatus", "AllUnits", "UpdateUnits", "Name")
	now := s.clock.Now()
	s.st.app.CheckCall(c, 1, "SetOperatorStatus",
		status.StatusInfo{Status: status.Active, Message: "working", Since: &now})
	s.st.app.units[0].CheckCallNames(c, "UpdateOperation")
	s.st.app.units[0].CheckCall(c, 0, "UpdateOperation", state.UnitUpdateProperties{
		ProviderId: strPtr("gitlab-0"),
		Address:    strPtr("address"), Ports: &[]string{"port"},
		CloudContainerStatus: &status.StatusInfo{Status: status.Running, Message: "message"},
		AgentStatus:          &status.StatusInfo{Status: status.Idle},
	})
	s.st.app.units[1].CheckCallNames(c, "UpdateOperation")
	s.st.app.units[1].CheckCall(c, 0, "UpdateOperation", state.UnitUpdateProperties{
		ProviderId: strPtr("gitlab-1"),
		Address:    strPtr("another-address"), Ports: &[]string{"another-port"},
		CloudContainerStatus: &status.StatusInfo{Status: status.Running, Message: "another message"},
		AgentStatus:          &status.StatusInfo{Status: status.Idle},
	})
	// Missing units are handled by the GarbageCollect call.
	s.st.app.units[2].CheckCallNames(c)

	s.storage.CheckCallNames(c,
		"UnitStorageAttachments", "StorageInstance", "UnitStorageAttachments", "StorageInstance",
		"AllFilesystems", "Volume", "SetVolumeInfo", "SetVolumeAttachmentInfo", "Volume", "SetStatus",
		"Volume", "SetStatus", "Filesystem", "SetFilesystemInfo", "SetFilesystemAttachmentInfo",
		"Filesystem", "SetStatus", "Filesystem", "SetStatus")
	s.storage.CheckCall(c, 0, "UnitStorageAttachments", names.NewUnitTag("gitlab/0"))
	s.storage.CheckCall(c, 1, "StorageInstance", names.NewStorageTag("data/0"))
	s.storage.CheckCall(c, 2, "UnitStorageAttachments", names.NewUnitTag("gitlab/1"))
	s.storage.CheckCall(c, 3, "StorageInstance", names.NewStorageTag("data/1"))

	s.storage.CheckCall(c, 6, "SetVolumeInfo",
		names.NewVolumeTag("1"),
		state.VolumeInfo{
			Size:       200,
			VolumeId:   "vol-id2",
			Persistent: true,
		})
	s.storage.CheckCall(c, 7, "SetVolumeAttachmentInfo",
		names.NewUnitTag("gitlab/1"), names.NewVolumeTag("1"),
		state.VolumeAttachmentInfo{
			ReadOnly: true,
		})
	s.storage.CheckCall(c, 9, "SetStatus",
		status.StatusInfo{
			Status:  status.Pending,
			Message: "vol not ready",
			Since:   &now,
		})
	s.storage.CheckCall(c, 11, "SetStatus",
		status.StatusInfo{
			Status:  status.Attached,
			Message: "vol ready",
			Since:   &now,
		})

	s.storage.CheckCall(c, 13, "SetFilesystemInfo",
		names.NewFilesystemTag("gitlab/1/0"),
		state.FilesystemInfo{
			Size:         200,
			FilesystemId: "fs-id2",
		})
	s.storage.CheckCall(c, 14, "SetFilesystemAttachmentInfo",
		names.NewUnitTag("gitlab/1"), names.NewFilesystemTag("gitlab/1/0"),
		state.FilesystemAttachmentInfo{
			MountPoint: "/path/to/there",
			ReadOnly:   true,
		})
	s.storage.CheckCall(c, 16, "SetStatus",
		status.StatusInfo{
			Status:  status.Pending,
			Message: "not ready",
			Since:   &now,
		})
	s.storage.CheckCall(c, 18, "SetStatus",
		status.StatusInfo{
			Status:  status.Attached,
			Message: "ready",
			Since:   &now,
		})

	s.st.model.CheckCallNames(c, "Containers")
	s.st.model.CheckCall(c, 0, "Containers", []string{"gitlab-0", "gitlab-1"})
}

func (s *CAASApplicationProvisionerSuite) TestUpdateApplicationsUnitsWithoutStorage(c *gc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.st.app = &mockApplication{
		tag:  names.NewApplicationTag("gitlab"),
		life: state.Alive,
		charm: &mockCharm{
			meta: &charm.Meta{},
			manifest: &charm.Manifest{
				// charm.FormatV2.
				Bases: []charm.Base{
					{
						Name: "ubuntu",
						Channel: charm.Channel{
							Risk:  "stable",
							Track: "20.04",
						},
					},
				},
			},
			url: "ch:gitlab",
		},
		units: []*mockUnit{
			{
				tag: names.NewUnitTag("gitlab/0"),
				containerInfo: &mockCloudContainer{
					unit:       "gitlab/0",
					providerId: "gitlab-0",
				},
			},
			{
				tag: names.NewUnitTag("gitlab/1"),
				containerInfo: &mockCloudContainer{
					unit:       "gitlab/1",
					providerId: "gitlab-1",
				},
			},
			{
				tag: names.NewUnitTag("gitlab/2"),
				containerInfo: &mockCloudContainer{
					unit:       "gitlab/2",
					providerId: "gitlab-2",
				},
			},
		},
	}

	units := []params.ApplicationUnitParams{
		{
			ProviderId: "gitlab-0", Address: "address", Ports: []string{"port"},
			Status: "running", Info: "message", Stateful: true,
		},
		{
			ProviderId: "gitlab-1", Address: "another-address", Ports: []string{"another-port"},
			Status: "running", Info: "another message", Stateful: true,
		},
	}

	args := params.UpdateApplicationUnitArgs{
		Args: []params.UpdateApplicationUnits{
			{ApplicationTag: "application-gitlab", Units: units},
		},
	}
	results, err := s.api.UpdateApplicationsUnits(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0], gc.DeepEquals, params.UpdateApplicationUnitResult{
		Info: &params.UpdateApplicationUnitsInfo{
			Units: []params.ApplicationUnitInfo{
				{ProviderId: "gitlab-0", UnitTag: "unit-gitlab-0"},
				{ProviderId: "gitlab-1", UnitTag: "unit-gitlab-1"},
			},
		},
	})
	s.st.app.CheckCallNames(c, "Life", "AllUnits", "UpdateUnits", "Name")
	s.st.app.units[0].CheckCallNames(c, "UpdateOperation")
	s.st.app.units[0].CheckCall(c, 0, "UpdateOperation", state.UnitUpdateProperties{
		ProviderId: strPtr("gitlab-0"),
		Address:    strPtr("address"), Ports: &[]string{"port"},
		CloudContainerStatus: &status.StatusInfo{Status: status.Running, Message: "message"},
		AgentStatus:          &status.StatusInfo{Status: status.Idle},
	})
	s.st.app.units[1].CheckCallNames(c, "UpdateOperation")
	s.st.app.units[1].CheckCall(c, 0, "UpdateOperation", state.UnitUpdateProperties{
		ProviderId: strPtr("gitlab-1"),
		Address:    strPtr("another-address"), Ports: &[]string{"another-port"},
		CloudContainerStatus: &status.StatusInfo{Status: status.Running, Message: "another message"},
		AgentStatus:          &status.StatusInfo{Status: status.Idle},
	})
	// Missing units are handled by the GarbageCollect call.
	s.st.app.units[2].CheckCallNames(c)

	s.storage.CheckCallNames(c, "AllFilesystems")
	s.st.model.CheckCallNames(c, "Containers")
	s.st.model.CheckCall(c, 0, "Containers", []string{"gitlab-0", "gitlab-1"})
}

func strPtr(s string) *string {
	return &s
}

func (s *CAASApplicationProvisionerSuite) TestClearApplicationsResources(c *gc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.st.app = &mockApplication{
		life: state.Alive,
		charm: &mockCharm{
			meta: &charm.Meta{},
			url:  "ch:gitlab",
		},
	}

	result, err := s.api.ClearApplicationsResources(context.Background(), params.Entities{
		Entities: []params.Entity{{
			Tag: "application-gitlab",
		}},
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Error, gc.IsNil)
	s.st.app.CheckCallNames(c, "ClearResources")
}

func (s *CAASApplicationProvisionerSuite) TestWatchUnits(c *gc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	unitsChanges := make(chan []string, 1)
	s.st.app = &mockApplication{
		life: state.Alive,
		charm: &mockCharm{
			meta: &charm.Meta{},
			url:  "ch:gitlab",
		},
		unitsChanges: unitsChanges,
		unitsWatcher: statetesting.NewMockStringsWatcher(unitsChanges),
	}
	unitsChanges <- []string{"gitlab/0", "gitlab/1"}

	results, err := s.api.WatchUnits(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
		},
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[0].StringsWatcherId, gc.Equals, "1")
	c.Assert(results.Results[0].Changes, jc.DeepEquals, []string{"gitlab/0", "gitlab/1"})
	res := s.resources.Get("1")
	c.Assert(res, gc.Equals, s.st.app.unitsWatcher)
}

func (s *CAASApplicationProvisionerSuite) TestProvisioningState(c *gc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.applicationService.EXPECT().SetApplicationScalingState(gomock.Any(), "gitlab", 10, true)
	s.applicationService.EXPECT().GetApplicationScalingState(gomock.Any(), "gitlab").Return(service.ScalingState{
		Scaling:     true,
		ScaleTarget: 10,
	}, nil)

	setResult, err := s.api.SetProvisioningState(context.Background(), params.CAASApplicationProvisioningStateArg{
		Application: params.Entity{Tag: "application-gitlab"},
		ProvisioningState: params.CAASApplicationProvisioningState{
			Scaling:     true,
			ScaleTarget: 10,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(setResult.Error, gc.IsNil)

	result, err := s.api.ProvisioningState(context.Background(), params.Entity{Tag: "application-gitlab"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.ProvisioningState, jc.DeepEquals, &params.CAASApplicationProvisioningState{
		Scaling:     true,
		ScaleTarget: 10,
	})
}

func (s *CAASApplicationProvisionerSuite) TestProvisionerConfig(c *gc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	result, err := s.api.ProvisionerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.ProvisionerConfig, gc.NotNil)
	c.Assert(result.ProvisionerConfig.UnmanagedApplications.Entities, gc.HasLen, 0)

	s.st.isController = true
	result, err = s.api.ProvisionerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.ProvisionerConfig, gc.NotNil)
	c.Assert(result.ProvisionerConfig.UnmanagedApplications.Entities, gc.DeepEquals, []params.Entity{{Tag: "application-controller"}})
}

func (s *CAASApplicationProvisionerSuite) TestCharmStorageParamsPoolNotFound(c *gc.C) {
	cfg, err := envconfig.New(envconfig.NoDefaults, coretesting.FakeConfig())
	c.Assert(err, jc.ErrorIsNil)
	p, err := caasapplicationprovisioner.CharmStorageParams(
		context.Background(), coretesting.ControllerTag.Id(),
		"notpool", cfg, model.UUID(coretesting.ModelTag.Id()), "", s.storagePoolGetter, s.registry,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p, jc.DeepEquals, &params.KubernetesFilesystemParams{
		StorageName: "charm",
		Size:        1024,
		Provider:    "kubernetes",
		Attributes:  map[string]any{"storage-class": "notpool"},
		Tags:        map[string]string{"juju-controller-uuid": coretesting.ControllerTag.Id(), "juju-model-uuid": coretesting.ModelTag.Id()},
	})
}

func (s *CAASApplicationProvisionerSuite) fakeModelConfig() (*envconfig.Config, error) {
	attrs := coretesting.FakeConfig()
	attrs["operator-storage"] = "k8s-storage"
	attrs["agent-version"] = "2.6-beta3.666"
	return envconfig.New(envconfig.UseDefaults, attrs)
}

func (s *CAASApplicationProvisionerSuite) TestRemove(c *gc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.applicationService.EXPECT().RemoveUnit(gomock.Any(), "gitlab/0", s.leadershipRevoker)

	result, err := s.api.Remove(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: "unit-gitlab-0",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{
		Error: nil,
	}}})
}
