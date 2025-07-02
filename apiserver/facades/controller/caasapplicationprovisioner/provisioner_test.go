// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/controller/caasapplicationprovisioner"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	jujuresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/application"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/removal"
	envconfig "github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/docker"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

func TestCAASApplicationProvisionerSuite(t *testing.T) {
	tc.Run(t, &CAASApplicationProvisionerSuite{})
}

type CAASApplicationProvisionerSuite struct {
	coretesting.BaseSuite
	clock clock.Clock

	resources               *common.Resources
	watcherRegistry         *facademocks.MockWatcherRegistry
	authorizer              *apiservertesting.FakeAuthorizer
	api                     *caasapplicationprovisioner.API
	st                      *mockState
	storage                 *mockStorage
	storagePoolGetter       *mockStoragePoolGetter
	applicationService      *MockApplicationService
	controllerConfigService *MockControllerConfigService
	controllerNodeService   *MockControllerNodeService
	modelConfigService      *MockModelConfigService
	modelInfoService        *MockModelInfoService
	statusService           *MockStatusService
	removalService          *MockRemovalService
	leadershipRevoker       *MockRevoker
	resourceOpener          *MockOpener
	registry                *mockStorageRegistry
	store                   *mockObjectStore
}

func (s *CAASApplicationProvisionerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *tc.C) { s.resources.StopAll() })
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

func (s *CAASApplicationProvisionerSuite) setupAPI(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.controllerNodeService = NewMockControllerNodeService(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.modelInfoService = NewMockModelInfoService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.statusService = NewMockStatusService(ctrl)
	s.leadershipRevoker = NewMockRevoker(ctrl)
	s.resourceOpener = NewMockOpener(ctrl)
	s.removalService = NewMockRemovalService(ctrl)
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)
	newResourceOpener := func(context.Context, string) (jujuresource.Opener, error) {
		return s.resourceOpener, nil
	}
	api, err := caasapplicationprovisioner.NewCAASApplicationProvisionerAPI(
		s.st,
		s.resources, newResourceOpener,
		s.authorizer,
		s.storage,
		s.storagePoolGetter,
		caasapplicationprovisioner.Services{
			ApplicationService:      s.applicationService,
			ControllerConfigService: s.controllerConfigService,
			ControllerNodeService:   s.controllerNodeService,
			ModelConfigService:      s.modelConfigService,
			ModelInfoService:        s.modelInfoService,
			StatusService:           s.statusService,
			RemovalService:          s.removalService,
		},
		s.leadershipRevoker,
		s.store,
		s.clock,
		loggertesting.WrapCheckLog(c),
		s.watcherRegistry)

	c.Assert(err, tc.ErrorIsNil)
	s.api = api

	c.Cleanup(func() {
		s.applicationService = nil
		s.controllerNodeService = nil
		s.modelConfigService = nil
		s.modelInfoService = nil
		s.statusService = nil
		s.leadershipRevoker = nil
		s.resourceOpener = nil
		s.watcherRegistry = nil
		s.api = nil
	})

	return ctrl
}

func (s *CAASApplicationProvisionerSuite) TestPermission(c *tc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	_, err := caasapplicationprovisioner.NewCAASApplicationProvisionerAPI(
		s.st,
		s.resources, nil,
		s.authorizer,
		s.storage,
		s.storagePoolGetter,
		caasapplicationprovisioner.Services{},
		s.leadershipRevoker,
		s.store,
		s.clock,
		loggertesting.WrapCheckLog(c),
		s.watcherRegistry)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *CAASApplicationProvisionerSuite) TestProvisioningInfo(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.st.app = &mockApplication{
		life:                 state.Alive,
		charmModifiedVersion: 10,
		config: config.ConfigAttributes{
			"trust": true,
		},
		charmURL: "ch:gitlab",
	}

	locator := applicationcharm.CharmLocator{
		Name:     "gitlab",
		Source:   applicationcharm.CharmHubSource,
		Revision: -1,
	}
	s.applicationService.EXPECT().GetCharmLocatorByApplicationName(gomock.Any(), "gitlab").Return(locator, nil)
	s.applicationService.EXPECT().IsCharmAvailable(gomock.Any(), locator).Return(true, nil)
	s.applicationService.EXPECT().GetCharmMetadataStorage(gomock.Any(), locator).Return(map[string]charm.Storage{}, nil)

	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(coretesting.FakeControllerConfig(), nil)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(s.fakeModelConfig())
	addrs := []string{"10.0.0.1:1"}
	s.controllerNodeService.EXPECT().GetAllAPIAddressesForAgents(gomock.Any()).Return(addrs, nil)

	modelInfo := model.ModelInfo{
		UUID: model.UUID(coretesting.ModelTag.Id()),
	}
	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(modelInfo, nil)
	s.applicationService.EXPECT().GetApplicationScale(gomock.Any(), "gitlab").Return(3, nil)
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "gitlab").Return(coreapplication.ID("deadbeef"), nil)
	s.applicationService.EXPECT().GetApplicationConstraints(gomock.Any(), coreapplication.ID("deadbeef")).Return(constraints.Value{}, nil)
	s.applicationService.EXPECT().GetDeviceConstraints(gomock.Any(), "gitlab").Return(map[string]devices.Constraints{}, nil)
	s.applicationService.EXPECT().GetApplicationCharmOrigin(gomock.Any(), "gitlab").Return(application.CharmOrigin{
		Platform: deployment.Platform{
			Channel: "stable",
			OSType:  deployment.Ubuntu,
		},
	}, nil)
	s.applicationService.EXPECT().GetCharmModifiedVersion(gomock.Any(), coreapplication.ID("deadbeef")).Return(10, nil)
	s.applicationService.EXPECT().GetApplicationTrustSetting(gomock.Any(), "gitlab").Return(true, nil)

	result, err := s.api.ProvisioningInfo(c.Context(), params.Entities{Entities: []params.Entity{{Tag: "application-gitlab"}}})
	c.Assert(err, tc.ErrorIsNil)

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.Results[0].CACert`, tc.Ignore)
	c.Assert(result, mc, params.CAASApplicationProvisioningInfoResults{
		Results: []params.CAASApplicationProvisioningInfo{{
			ImageRepo:    params.DockerImageInfo{RegistryPath: "docker.io/jujusolutions/jujud-operator:2.6-beta3.666"},
			Version:      semversion.MustParse("2.6-beta3.666"),
			APIAddresses: []string{"10.0.0.1:1"},
			Tags: map[string]string{
				"juju-model-uuid":      coretesting.ModelTag.Id(),
				"juju-controller-uuid": coretesting.ControllerTag.Id(),
			},
			CharmURL:             "ch:amd64/gitlab",
			CharmModifiedVersion: 10,
			Scale:                3,
			Trust:                true,
			Base: params.Base{
				Name:    "ubuntu",
				Channel: "stable",
			},
		}},
	})
}

func (s *CAASApplicationProvisionerSuite) TestProvisioningInfoPendingCharmError(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.st.app = &mockApplication{
		life: state.Alive,
	}

	locator := applicationcharm.CharmLocator{
		Name:     "gitlab",
		Source:   applicationcharm.CharmHubSource,
		Revision: -1,
	}
	s.applicationService.EXPECT().GetCharmLocatorByApplicationName(gomock.Any(), "gitlab").Return(locator, nil)
	s.applicationService.EXPECT().IsCharmAvailable(gomock.Any(), locator).Return(false, nil)

	result, err := s.api.ProvisioningInfo(c.Context(), params.Entities{Entities: []params.Entity{{Tag: "application-gitlab"}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results[0].Error, tc.ErrorMatches, `charm for application "gitlab" not provisioned`)
}

func (s *CAASApplicationProvisionerSuite) TestWatchProvisioningInfo(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	appChanged := make(chan struct{}, 1)
	legacyAppChanged := make(chan struct{}, 1)
	portsChanged := make(chan struct{}, 1)
	modelConfigChanged := make(chan []string, 1)
	controllerConfigChanged := make(chan []string, 1)
	s.controllerNodeService.EXPECT().WatchControllerAPIAddresses(gomock.Any()).Return(watchertest.NewMockNotifyWatcher(portsChanged), nil)
	s.controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).Return(watchertest.NewMockStringsWatcher(controllerConfigChanged), nil)
	s.modelConfigService.EXPECT().Watch().Return(watchertest.NewMockStringsWatcher(modelConfigChanged), nil)
	s.applicationService.EXPECT().WatchApplication(gomock.Any(), "gitlab").Return(watchertest.NewMockNotifyWatcher(appChanged), nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("42", nil)

	s.st.app = &mockApplication{
		life:    state.Alive,
		watcher: watchertest.NewMockNotifyWatcher(legacyAppChanged),
	}
	appChanged <- struct{}{}
	legacyAppChanged <- struct{}{}
	portsChanged <- struct{}{}
	modelConfigChanged <- []string{}
	controllerConfigChanged <- []string{}

	results, err := s.api.WatchProvisioningInfo(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
	c.Assert(results.Results[0].NotifyWatcherId, tc.Equals, "42")
}

func (s *CAASApplicationProvisionerSuite) TestApplicationOCIResources(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.st.app = &mockApplication{
		tag:  names.NewApplicationTag("gitlab"),
		life: state.Alive,
	}
	res := &docker.DockerImageDetails{
		RegistryPath: "gitlab:latest",
		ImageRepoDetails: docker.ImageRepoDetails{
			BasicAuthConfig: docker.BasicAuthConfig{
				Username: "jujuqa",
				Password: "pwd",
			},
		},
	}
	s.st.resource = &mockResources{
		resource: res,
	}

	// Return the marshalled resource.
	out, err := json.Marshal(res)
	c.Assert(err, tc.ErrorIsNil)
	s.resourceOpener.EXPECT().OpenResource(gomock.Any(), "gitlab-image").Return(
		jujuresource.Opened{
			ReadCloser: io.NopCloser(bytes.NewBuffer(out)),
		},
		nil,
	)
	s.resourceOpener.EXPECT().SetResourceUsed(gomock.Any(), "gitlab-image")

	locator := applicationcharm.CharmLocator{
		Name:     "gitlab",
		Source:   applicationcharm.CharmHubSource,
		Revision: -1,
	}
	s.applicationService.EXPECT().GetCharmLocatorByApplicationName(gomock.Any(), "gitlab").Return(locator, nil)
	s.applicationService.EXPECT().GetCharmMetadataResources(gomock.Any(), locator).Return(map[string]charmresource.Meta{
		"gitlab-image": {
			Name: "gitlab-image",
			Type: charmresource.TypeContainerImage,
		},
	}, nil)

	result, err := s.api.ApplicationOCIResources(c.Context(), params.Entities{
		Entities: []params.Entity{{
			Tag: "application-gitlab",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results[0].Error, tc.IsNil)
	c.Assert(result.Results[0].Result, tc.DeepEquals, &params.CAASApplicationOCIResources{
		Images: map[string]params.DockerImageInfo{
			"gitlab-image": {
				RegistryPath: "gitlab:latest",
				Username:     "jujuqa",
				Password:     "pwd",
			},
		},
	})
}

func (s *CAASApplicationProvisionerSuite) TestUpdateApplicationsUnitsWithStorage(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.st.app = &mockApplication{
		tag:  names.NewApplicationTag("gitlab"),
		life: state.Alive,
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
		{
			ProviderId: "gitlab-0", Address: "address", Ports: []string{"port"},
			Status: "running", Info: "message", Stateful: true,
			FilesystemInfo: []params.KubernetesFilesystemInfo{
				{
					StorageName: "data", FilesystemId: "fs-id", Size: 100, MountPoint: "/path/to/here", ReadOnly: true,
					Status: "pending", Info: "not ready",
					Volume: params.KubernetesVolumeInfo{
						VolumeId: "vol-id", Size: 100, Persistent: true,
						Status: "pending", Info: "vol not ready",
					},
				},
			},
		},
		{
			ProviderId: "gitlab-1", Address: "another-address", Ports: []string{"another-port"},
			Status: "running", Info: "another message", Stateful: true,
			FilesystemInfo: []params.KubernetesFilesystemInfo{
				{
					StorageName: "data", FilesystemId: "fs-id2", Size: 200, MountPoint: "/path/to/there", ReadOnly: true,
					Status: "attached", Info: "ready",
					Volume: params.KubernetesVolumeInfo{
						VolumeId: "vol-id2", Size: 200, Persistent: true,
						Status: "attached", Info: "vol ready",
					},
				},
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

	s.applicationService.EXPECT().GetApplicationLifeByName(gomock.Any(), "gitlab").Return(life.Alive, nil)
	s.applicationService.EXPECT().GetUnitNamesForApplication(gomock.Any(), "gitlab").
		Return([]coreunit.Name{coreunit.Name("gitlab/0"), coreunit.Name("gitlab/1")}, nil)
	s.applicationService.EXPECT().UpdateCAASUnit(gomock.Any(), coreunit.Name("gitlab/0"), service.UpdateCAASUnitParams{
		ProviderID:           strPtr("gitlab-0"),
		Address:              strPtr("address"),
		Ports:                &[]string{"port"},
		AgentStatus:          &status.StatusInfo{Status: status.Idle},
		CloudContainerStatus: &status.StatusInfo{Status: status.Running, Message: "message"},
	})
	s.applicationService.EXPECT().UpdateCAASUnit(gomock.Any(), coreunit.Name("gitlab/1"), service.UpdateCAASUnitParams{
		ProviderID:           strPtr("gitlab-1"),
		Address:              strPtr("another-address"),
		Ports:                &[]string{"another-port"},
		AgentStatus:          &status.StatusInfo{Status: status.Idle},
		CloudContainerStatus: &status.StatusInfo{Status: status.Running, Message: "another message"},
	})

	now := s.clock.Now()
	s.statusService.EXPECT().SetApplicationStatus(gomock.Any(), "gitlab", status.StatusInfo{
		Status:  status.Active,
		Message: "working",
		Since:   &now,
	})

	results, err := s.api.UpdateApplicationsUnits(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results[0], tc.DeepEquals, params.UpdateApplicationUnitResult{
		Info: &params.UpdateApplicationUnitsInfo{
			Units: []params.ApplicationUnitInfo{
				{ProviderId: "gitlab-0", UnitTag: "unit-gitlab-0"},
				{ProviderId: "gitlab-1", UnitTag: "unit-gitlab-1"},
			},
		},
	})

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

func (s *CAASApplicationProvisionerSuite) TestUpdateApplicationsUnitsWithoutStorage(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.st.app = &mockApplication{
		tag:  names.NewApplicationTag("gitlab"),
		life: state.Alive,
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

	s.applicationService.EXPECT().GetApplicationLifeByName(gomock.Any(), "gitlab").Return(life.Alive, nil)
	s.applicationService.EXPECT().GetUnitNamesForApplication(gomock.Any(), "gitlab").
		Return([]coreunit.Name{coreunit.Name("gitlab/0"), coreunit.Name("gitlab/1")}, nil)
	s.applicationService.EXPECT().UpdateCAASUnit(gomock.Any(), coreunit.Name("gitlab/0"), service.UpdateCAASUnitParams{
		ProviderID:           strPtr("gitlab-0"),
		Address:              strPtr("address"),
		Ports:                &[]string{"port"},
		AgentStatus:          &status.StatusInfo{Status: status.Idle},
		CloudContainerStatus: &status.StatusInfo{Status: status.Running, Message: "message"},
	})
	s.applicationService.EXPECT().UpdateCAASUnit(gomock.Any(), coreunit.Name("gitlab/1"), service.UpdateCAASUnitParams{
		ProviderID:           strPtr("gitlab-1"),
		Address:              strPtr("another-address"),
		Ports:                &[]string{"another-port"},
		AgentStatus:          &status.StatusInfo{Status: status.Idle},
		CloudContainerStatus: &status.StatusInfo{Status: status.Running, Message: "another message"},
	})

	results, err := s.api.UpdateApplicationsUnits(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results[0], tc.DeepEquals, params.UpdateApplicationUnitResult{
		Info: &params.UpdateApplicationUnitsInfo{
			Units: []params.ApplicationUnitInfo{
				{ProviderId: "gitlab-0", UnitTag: "unit-gitlab-0"},
				{ProviderId: "gitlab-1", UnitTag: "unit-gitlab-1"},
			},
		},
	})

	s.storage.CheckCallNames(c, "AllFilesystems")
	s.st.model.CheckCallNames(c, "Containers")
	s.st.model.CheckCall(c, 0, "Containers", []string{"gitlab-0", "gitlab-1"})
}

func strPtr(s string) *string {
	return &s
}

func (s *CAASApplicationProvisionerSuite) TestClearApplicationsResources(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.st.app = &mockApplication{
		life: state.Alive,
	}

	result, err := s.api.ClearApplicationsResources(c.Context(), params.Entities{
		Entities: []params.Entity{{
			Tag: "application-gitlab",
		}},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results[0].Error.Code, tc.Equals, params.CodeNotImplemented)
	c.Assert(result.Results[0].Error.Message, tc.Equals, "ClearResources not implemented")
}

func (s *CAASApplicationProvisionerSuite) TestCharmStorageParamsPoolNotFound(c *tc.C) {
	cfg, err := envconfig.New(envconfig.NoDefaults, coretesting.FakeConfig())
	c.Assert(err, tc.ErrorIsNil)
	p, err := caasapplicationprovisioner.CharmStorageParams(
		c.Context(), coretesting.ControllerTag.Id(),
		"notpool", cfg, model.UUID(coretesting.ModelTag.Id()), "", s.storagePoolGetter,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(p, tc.DeepEquals, &params.KubernetesFilesystemParams{
		StorageName: "charm",
		Size:        1024,
		Provider:    "kubernetes",
		Attributes:  map[string]any{"storage-class": "notpool"},
		Tags:        map[string]string{"juju-controller-uuid": coretesting.ControllerTag.Id(), "juju-model-uuid": coretesting.ModelTag.Id()},
	})
}

func (s *CAASApplicationProvisionerSuite) fakeModelConfig() (*envconfig.Config, error) {
	attrs := coretesting.FakeConfig()
	attrs["agent-version"] = "2.6-beta3.666"
	return envconfig.New(envconfig.UseDefaults, attrs)
}

func (s *CAASApplicationProvisionerSuite) TestRemove(c *tc.C) {
	ctrl := s.setupAPI(c)
	defer ctrl.Finish()

	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("gitlab/0")).Return(coreunit.UUID("unit-uuid"), nil)
	s.removalService.EXPECT().MarkUnitAsDead(gomock.Any(), coreunit.UUID("unit-uuid")).Return(nil)
	s.removalService.EXPECT().RemoveUnit(gomock.Any(), coreunit.UUID("unit-uuid"), false, time.Duration(0)).Return(removal.UUID("removal-uuid"), nil)

	result, err := s.api.Remove(c.Context(), params.Entities{Entities: []params.Entity{{
		Tag: "unit-gitlab-0",
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{
		Error: nil,
	}}})
}

func (s *CAASApplicationProvisionerSuite) TestDestroyUnits(c *tc.C) {
	defer s.setupAPI(c).Finish()

	// Arrange
	unitName := coreunit.Name("foo/0")
	unitUUID := unittesting.GenUnitUUID(c)
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.removalService.EXPECT().RemoveUnit(gomock.Any(), unitUUID, false, time.Duration(0)).Return(removal.UUID(""), nil)

	// Act
	res, err := s.api.DestroyUnits(c.Context(), params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{{
			UnitTag: names.NewUnitTag(unitName.String()).String(),
		}},
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.IsNil)
}

func (s *CAASApplicationProvisionerSuite) TestDestroyUnitsForce(c *tc.C) {
	defer s.setupAPI(c).Finish()

	d := time.Hour

	// Arrange
	unitName := coreunit.Name("foo/0")
	unitUUID := unittesting.GenUnitUUID(c)
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.removalService.EXPECT().RemoveUnit(gomock.Any(), unitUUID, true, time.Hour).Return(removal.UUID(""), nil)

	// Act
	res, err := s.api.DestroyUnits(c.Context(), params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{{
			UnitTag: names.NewUnitTag(unitName.String()).String(),
			Force:   true,
			MaxWait: &d,
		}},
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.IsNil)
}

func (s *CAASApplicationProvisionerSuite) TestDestroyUnitsNotFound(c *tc.C) {
	defer s.setupAPI(c).Finish()

	// Arrange
	unitName := coreunit.Name("foo/0")
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(coreunit.UUID(""), applicationerrors.UnitNotFound)

	// Act
	res, err := s.api.DestroyUnits(c.Context(), params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{{
			UnitTag: names.NewUnitTag(unitName.String()).String(),
		}},
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.IsNil)
}
