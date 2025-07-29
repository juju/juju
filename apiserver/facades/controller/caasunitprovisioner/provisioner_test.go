// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner_test

import (
	"fmt"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/caasunitprovisioner"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/mocks"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	storageprovider "github.com/juju/juju/storage/provider"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

var _ = gc.Suite(&CAASProvisionerSuite{})

type CAASProvisionerSuite struct {
	coretesting.BaseSuite

	clock               clock.Clock
	st                  *mockState
	storage             *mockStorage
	storagePoolManager  *mockStoragePoolManager
	registry            *mockStorageRegistry
	devices             *mockDeviceBackend
	applicationsChanges chan []string
	podSpecChanges      chan struct{}
	scaleChanges        chan struct{}
	settingsChanges     chan []string

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	facade     *caasunitprovisioner.Facade
	broker     *mocks.MockBroker

	isRawK8sSpec *bool
}

func boolptr(i bool) *bool {
	return &i
}

func (s *CAASProvisionerSuite) setupFacade(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.broker = mocks.NewMockBroker(ctrl)

	facade, err := caasunitprovisioner.NewFacade(
		s.resources, s.authorizer, s.st, s.storage, s.devices,
		s.storagePoolManager, s.registry, nil, nil, s.clock, s.broker)
	c.Assert(err, jc.ErrorIsNil)
	s.facade = facade
	return ctrl
}

func (s *CAASProvisionerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.applicationsChanges = make(chan []string, 1)
	s.podSpecChanges = make(chan struct{}, 1)
	s.scaleChanges = make(chan struct{}, 1)
	s.settingsChanges = make(chan []string, 1)
	s.isRawK8sSpec = boolptr(false)
	s.st = &mockState{
		application: mockApplication{
			tag:             names.NewApplicationTag("gitlab"),
			life:            state.Alive,
			scaleWatcher:    statetesting.NewMockNotifyWatcher(s.scaleChanges),
			settingsWatcher: statetesting.NewMockStringsWatcher(s.settingsChanges),
			scale:           5,
		},
		applicationsWatcher: statetesting.NewMockStringsWatcher(s.applicationsChanges),
		model: mockModel{
			podSpecWatcher: statetesting.NewMockNotifyWatcher(s.podSpecChanges),
			isRawK8sSpec:   s.isRawK8sSpec,
		},
		unit: mockUnit{
			life: state.Dying,
		},
	}
	s.storage = &mockStorage{
		storageFilesystems: make(map[names.StorageTag]names.FilesystemTag),
		storageVolumes:     make(map[names.StorageTag]names.VolumeTag),
		storageAttachments: make(map[names.UnitTag]names.StorageTag),
		backingVolume:      names.NewVolumeTag("66"),
	}
	s.storagePoolManager = &mockStoragePoolManager{}
	s.devices = &mockDeviceBackend{}
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.st.applicationsWatcher) })
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.st.application.scaleWatcher) })
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.st.application.settingsWatcher) })
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.st.model.podSpecWatcher) })

	s.resources = common.NewResources()
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	s.clock = testclock.NewClock(time.Now())
	s.PatchValue(&jujuversion.OfficialBuild, 0)
}

func (s *CAASProvisionerSuite) TestPermission(c *gc.C) {
	ctrl := gomock.NewController(c)
	s.broker = mocks.NewMockBroker(ctrl)

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	_, err := caasunitprovisioner.NewFacade(
		s.resources, s.authorizer, s.st, s.storage, s.devices,
		s.storagePoolManager, s.registry, nil, nil, s.clock, s.broker)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *CAASProvisionerSuite) TestWatchApplications(c *gc.C) {
	ctrl := s.setupFacade(c)
	defer ctrl.Finish()

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
	ctrl := s.setupFacade(c)
	defer ctrl.Finish()

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
	ctrl := s.setupFacade(c)
	defer ctrl.Finish()

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

func (s *CAASProvisionerSuite) TestWatchApplicationsConfigSetingsHash(c *gc.C) {
	ctrl := s.setupFacade(c)
	defer ctrl.Finish()

	s.settingsChanges <- []string{"hash"}

	results, err := s.facade.WatchApplicationsTrustHash(params.Entities{
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

	c.Assert(results.Results[0].StringsWatcherId, gc.Equals, "1")
	resource := s.resources.Get("1")
	c.Assert(resource, gc.Equals, s.st.application.settingsWatcher)
}

func (s *CAASProvisionerSuite) assertProvisioningInfo(c *gc.C, isRawK8sSpec bool) {
	s.st.application.units = []caasunitprovisioner.Unit{
		&mockUnit{name: "gitlab/0", life: state.Dying},
		&mockUnit{name: "gitlab/1", life: state.Alive},
	}
	s.st.application.charm = &mockCharm{
		meta: charm.Meta{
			Storage: map[string]charm.Storage{
				"data": {
					Name:     "data",
					Type:     charm.StorageFilesystem,
					ReadOnly: true,
				},
				"logs": {
					Name: "logs",
					Type: charm.StorageFilesystem,
				},
			},
			Deployment: &charm.Deployment{
				DeploymentType: charm.DeploymentStateful,
				ServiceType:    charm.ServiceLoadBalancer,
			},
		},
	}
	*s.isRawK8sSpec = isRawK8sSpec
	expectedVersion := jujuversion.Current
	expectedVersion.Build = 666
	s.broker.EXPECT().GetModelOperatorDeploymentImage().Return(fmt.Sprintf("ghcr.io/juju/jujud-operator:%s", expectedVersion.String()), nil)

	results, err := s.facade.ProvisioningInfo(params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
			{Tag: "unit-gitlab-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	// Maps are harder to check...
	// http://ci.jujucharms.com/job/make-check-juju/4853/testReport/junit/github/com_juju_juju_apiserver_facades_controller_caasunitprovisioner/TestAll/
	expectedResult := &params.KubernetesProvisioningInfo{
		DeploymentInfo: &params.KubernetesDeploymentInfo{
			DeploymentType: "stateful",
			ServiceType:    "loadbalancer",
		},
		ImageRepo: params.DockerImageInfo{
			RegistryPath: fmt.Sprintf("ghcr.io/juju/jujud-operator:%s", expectedVersion.String()),
		},
		Devices: []params.KubernetesDeviceParams{
			{
				Type:       "nvidia.com/gpu",
				Count:      3,
				Attributes: map[string]string{"gpu": "nvidia-tesla-p100"},
			},
		},
		Constraints: constraints.MustParse("mem=64G"),
		Tags: map[string]string{
			"juju-model-uuid":      coretesting.ModelTag.Id(),
			"juju-controller-uuid": coretesting.ControllerTag.Id()},
	}
	if isRawK8sSpec {
		expectedResult.RawK8sSpec = "raw spec(gitlab)"
	} else {
		expectedResult.PodSpec = "spec(gitlab)"
	}
	expectedFileSystems := map[string]params.KubernetesFilesystemParams{
		"data": {
			StorageName: "data",
			Provider:    string(k8sconstants.StorageProviderType),
			Size:        100,
			Attributes: map[string]interface{}{
				"storage-class": "k8s-storage",
				"foo":           "bar",
			},
			Tags: map[string]string{
				"juju-storage-owner":   "gitlab",
				"juju-model-uuid":      coretesting.ModelTag.Id(),
				"juju-controller-uuid": coretesting.ControllerTag.Id()},
			Attachment: &params.KubernetesFilesystemAttachmentParams{
				Provider:   string(k8sconstants.StorageProviderType),
				MountPoint: "/var/lib/juju/storage/data/0",
				ReadOnly:   true,
			},
		},
		"logs": {
			StorageName: "logs",
			Provider:    string(storageprovider.RootfsProviderType),
			Size:        200,
			Attributes:  map[string]interface{}{},
			Tags: map[string]string{
				"juju-storage-owner":   "gitlab",
				"juju-model-uuid":      coretesting.ModelTag.Id(),
				"juju-controller-uuid": coretesting.ControllerTag.Id()},
			Attachment: &params.KubernetesFilesystemAttachmentParams{
				Provider:   string(storageprovider.RootfsProviderType),
				MountPoint: "/var/lib/juju/storage/logs/0",
			},
		}}
	c.Assert(results.Results[0].Error, gc.IsNil)
	obtained := results.Results[0].Result
	c.Assert(obtained, gc.NotNil)
	c.Assert(obtained.PodSpec, jc.DeepEquals, expectedResult.PodSpec)
	c.Assert(obtained.RawK8sSpec, jc.DeepEquals, expectedResult.RawK8sSpec)
	c.Assert(obtained.DeploymentInfo, jc.DeepEquals, expectedResult.DeploymentInfo)
	c.Assert(obtained.ImageRepo.RegistryPath, gc.Equals, expectedResult.ImageRepo.RegistryPath)
	c.Assert(obtained.CharmModifiedVersion, jc.DeepEquals, 888)
	c.Assert(len(obtained.Filesystems), gc.Equals, len(expectedFileSystems))
	for _, fs := range obtained.Filesystems {
		c.Assert(fs, gc.DeepEquals, expectedFileSystems[fs.StorageName])
	}
	c.Assert(obtained.Devices, jc.DeepEquals, expectedResult.Devices)
	c.Assert(obtained.Constraints, jc.DeepEquals, expectedResult.Constraints)
	c.Assert(obtained.Tags, jc.DeepEquals, expectedResult.Tags)
	c.Assert(results.Results[1], jc.DeepEquals, params.KubernetesProvisioningInfoResult{
		Error: &params.Error{
			Message: `"unit-gitlab-0" is not a valid application tag`,
		},
	})
	s.st.CheckCallNames(c, "Model", "Application", "ControllerConfig", "ResolveConstraints")
	s.st.CheckCall(c, 3, "ResolveConstraints", constraints.MustParse("mem=64G"))
	s.storagePoolManager.CheckCallNames(c, "Get", "Get")
}

func (s *CAASProvisionerSuite) TestProvisioningInfoK8sSpec(c *gc.C) {
	ctrl := s.setupFacade(c)
	defer ctrl.Finish()

	s.assertProvisioningInfo(c, false)
}
func (s *CAASProvisionerSuite) TestProvisioningInfoRawK8sSpec(c *gc.C) {
	ctrl := s.setupFacade(c)
	defer ctrl.Finish()

	s.assertProvisioningInfo(c, true)
}

func (s *CAASProvisionerSuite) TestApplicationScale(c *gc.C) {
	ctrl := s.setupFacade(c)
	defer ctrl.Finish()

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

func (s *CAASProvisionerSuite) TestDeploymentMode(c *gc.C) {
	ctrl := s.setupFacade(c)
	defer ctrl.Finish()

	s.st.application.charm = &mockCharm{
		meta: charm.Meta{
			Deployment: &charm.Deployment{
				DeploymentMode: charm.ModeWorkload,
			},
		},
	}
	results, err := s.facade.DeploymentMode(params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
			{Tag: "unit-gitlab-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{{
			Result: "workload",
		}, {
			Error: &params.Error{
				Message: `"unit-gitlab-0" is not a valid application tag`,
			},
		}},
	})
	s.st.CheckCallNames(c, "Application")
}

func (s *CAASProvisionerSuite) TestLife(c *gc.C) {
	ctrl := s.setupFacade(c)
	defer ctrl.Finish()

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
			Life: life.Dying,
		}, {
			Life: life.Alive,
		}, {
			Error: &params.Error{
				Code:    "unauthorized access",
				Message: "permission denied",
			},
		}},
	})
}

func (s *CAASProvisionerSuite) TestApplicationConfig(c *gc.C) {
	ctrl := s.setupFacade(c)
	defer ctrl.Finish()

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

func (s *CAASProvisionerSuite) TestClearApplicationsResources(c *gc.C) {
	ctrl := s.setupFacade(c)
	defer ctrl.Finish()

	results, err := s.facade.ClearApplicationsResources(params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
			{Tag: "unit-gitlab-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{},
			{
				Error: &params.Error{
					Message: `"unit-gitlab-0" is not a valid application tag`,
				},
			}},
	})
	s.st.CheckCallNames(c, "Application")
	s.st.application.CheckCallNames(c, "ClearResources")
}

func strPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}
func int64Ptr(i int64) *int64 {
	return &i
}

func (s *CAASProvisionerSuite) TestUpdateApplicationsStatelessUnits(c *gc.C) {
	ctrl := s.setupFacade(c)
	defer ctrl.Finish()

	s.assertUpdateApplicationsStatelessUnits(c, true)
}

func (s *CAASProvisionerSuite) TestUpdateApplicationsStatelessUnitsWithoutGeneration(c *gc.C) {
	ctrl := s.setupFacade(c)
	defer ctrl.Finish()

	s.assertUpdateApplicationsStatelessUnits(c, false)
}

func (s *CAASProvisionerSuite) assertUpdateApplicationsStatelessUnits(c *gc.C, withGeneration bool) {
	s.st.application.units = []caasunitprovisioner.Unit{
		&mockUnit{name: "gitlab/0", containerInfo: &mockContainerInfo{providerId: "uuid"}, life: state.Alive},
		&mockUnit{name: "gitlab/1", life: state.Alive},
		&mockUnit{name: "gitlab/2", containerInfo: &mockContainerInfo{providerId: "uuid2"}, life: state.Alive},
		&mockUnit{name: "gitlab/3", life: state.Alive},
	}
	s.st.application.scale = 4

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

	args := []params.UpdateApplicationUnits{
		{ApplicationTag: "application-another", Units: []params.ApplicationUnitParams{}},
	}
	gitlab := params.UpdateApplicationUnits{ApplicationTag: "application-gitlab", Units: units, Scale: intPtr(4)}
	if withGeneration {
		gitlab.Generation = int64Ptr(1)
	}
	args = append(args, gitlab)

	results, err := s.facade.UpdateApplicationsUnits(params.UpdateApplicationUnitArgs{Args: args})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.UpdateApplicationUnitResults{
		Results: []params.UpdateApplicationUnitResult{
			{Error: &params.Error{Message: "application another not found", Code: "not found"}},
			{Error: nil},
		},
	})
	s.st.application.CheckCallNames(c, "Life", "AddOperation", "Name", "GetScale")
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

func (s *CAASProvisionerSuite) TestUpdateApplicationsScaleChange(c *gc.C) {
	ctrl := s.setupFacade(c)
	defer ctrl.Finish()

	s.st.application.units = []caasunitprovisioner.Unit{
		&mockUnit{name: "gitlab/0", containerInfo: &mockContainerInfo{providerId: "uuid"}, life: state.Alive},
		&mockUnit{name: "gitlab/1", life: state.Alive},
		&mockUnit{name: "gitlab/2", containerInfo: &mockContainerInfo{providerId: "uuid2"}, life: state.Alive},
	}
	s.st.application.scale = 3

	units := []params.ApplicationUnitParams{
		{ProviderId: "uuid", Address: "address", Ports: []string{"port"},
			Status: "allocating", Info: ""},
		{ProviderId: "another-uuid", Address: "another-address", Ports: []string{"another-port"},
			Status: "allocating", Info: "another message"},
	}
	args := params.UpdateApplicationUnitArgs{
		Args: []params.UpdateApplicationUnits{
			{
				ApplicationTag: "application-gitlab",
				Units:          units,
				Scale:          intPtr(2),
				Generation:     int64Ptr(1),
				Status:         params.EntityStatus{Status: status.Active, Info: "working"}},
		},
	}
	results, err := s.facade.UpdateApplicationsUnits(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.UpdateApplicationUnitResults{
		Results: []params.UpdateApplicationUnitResult{
			{},
		},
	})
	s.st.application.CheckCallNames(c, "SetOperatorStatus", "Life", "Name", "GetScale", "SetScale")
	now := s.clock.Now()
	s.st.application.CheckCall(c, 0, "SetOperatorStatus",
		status.StatusInfo{Status: status.Active, Message: "working", Since: &now})
	s.st.application.CheckCall(c, 4, "SetScale", 2)

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
}

func (s *CAASProvisionerSuite) TestUpdateApplicationsUnknownScale(c *gc.C) {
	ctrl := s.setupFacade(c)
	defer ctrl.Finish()

	s.st.application.units = []caasunitprovisioner.Unit{
		&mockUnit{name: "gitlab/0", containerInfo: &mockContainerInfo{providerId: "uuid"}, life: state.Alive},
		&mockUnit{name: "gitlab/1", life: state.Alive},
		&mockUnit{name: "gitlab/2", containerInfo: &mockContainerInfo{providerId: "uuid2"}, life: state.Alive},
	}
	s.st.application.scale = 3

	units := []params.ApplicationUnitParams{
		{ProviderId: "uuid", Address: "address", Ports: []string{"port"},
			Status: "allocating", Info: ""},
		{ProviderId: "another-uuid", Address: "another-address", Ports: []string{"another-port"},
			Status: "allocating", Info: "another message"},
	}
	args := params.UpdateApplicationUnitArgs{
		Args: []params.UpdateApplicationUnits{
			{ApplicationTag: "application-gitlab", Units: units, Scale: nil},
		},
	}
	results, err := s.facade.UpdateApplicationsUnits(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.UpdateApplicationUnitResults{
		Results: []params.UpdateApplicationUnitResult{
			{nil, nil},
		},
	})
	s.st.application.CheckCallNames(c, "Life", "Name")

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
	s.st.application.units[2].(*mockUnit).CheckCallNames(c, "Life", "UpdateOperation")
	s.st.application.units[2].(*mockUnit).CheckCall(c, 1, "UpdateOperation", state.UnitUpdateProperties{
		AgentStatus:          &status.StatusInfo{Status: status.Idle},
		CloudContainerStatus: &status.StatusInfo{Status: status.Terminated, Message: "unit stopped by the cloud"},
	})
}

func (s *CAASProvisionerSuite) TestUpdateApplicationsUnitsNotAlive(c *gc.C) {
	ctrl := s.setupFacade(c)
	defer ctrl.Finish()

	s.st.application.units = []caasunitprovisioner.Unit{
		&mockUnit{name: "gitlab/0", life: state.Alive},
		&mockUnit{name: "gitlab/1", life: state.Alive},
		&mockUnit{name: "gitlab/2", containerInfo: &mockContainerInfo{providerId: "uuid2"}, life: state.Alive},
	}
	s.st.application.scale = 3
	s.st.application.life = state.Dying

	units := []params.ApplicationUnitParams{
		{ProviderId: "uuid", UnitTag: "unit-gitlab-0", Address: "address", Ports: []string{"port"},
			Status: "running", Info: "message"},
		{ProviderId: "another-uuid", UnitTag: "unit-gitlab-1", Address: "another-address", Ports: []string{"another-port"},
			Status: "error", Info: "another message"},
	}
	args := params.UpdateApplicationUnitArgs{
		Args: []params.UpdateApplicationUnits{
			{ApplicationTag: "application-gitlab", Units: units, Scale: intPtr(3), Generation: int64Ptr(1)},
		},
	}
	results, err := s.facade.UpdateApplicationsUnits(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.UpdateApplicationUnitResults{
		Results: []params.UpdateApplicationUnitResult{
			{nil, nil},
		},
	})
	s.st.application.CheckCallNames(c, "Life", "Name", "GetScale")
	s.st.application.units[0].(*mockUnit).CheckCallNames(c, "Life")
	s.st.application.units[1].(*mockUnit).CheckCallNames(c, "Life")
	s.st.application.units[2].(*mockUnit).CheckCallNames(c, "Life")
}

func (s *CAASProvisionerSuite) TestUpdateApplicationsUnitsWithStorage(c *gc.C) {
	ctrl := s.setupFacade(c)
	defer ctrl.Finish()

	s.st.application.units = []caasunitprovisioner.Unit{
		&mockUnit{name: "gitlab/0", containerInfo: &mockContainerInfo{providerId: "uuid"}, life: state.Alive},
		&mockUnit{name: "gitlab/1", life: state.Alive},
		&mockUnit{name: "gitlab/2", containerInfo: &mockContainerInfo{providerId: "gone-uuid"}, life: state.Alive},
		&mockUnit{name: "gitlab/3", containerInfo: &mockContainerInfo{providerId: "gone-uuid2"}, life: state.Alive},
	}
	s.st.model.containers = []state.CloudContainer{
		&mockContainerInfo{unitName: "gitlab/0", providerId: "uuid"},
		&mockContainerInfo{unitName: "gitlab/1", providerId: "another-uuid"},
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
		{ProviderId: "uuid", Address: "address", Ports: []string{"port"},
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
		{ProviderId: "another-uuid", Address: "another-address", Ports: []string{"another-port"},
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
	s.st.application.scale = 3
	args := params.UpdateApplicationUnitArgs{
		Args: []params.UpdateApplicationUnits{
			{ApplicationTag: "application-gitlab", Units: units, Scale: intPtr(3), Generation: int64Ptr(1)},
		},
	}
	results, err := s.facade.UpdateApplicationsUnits(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0], gc.DeepEquals, params.UpdateApplicationUnitResult{
		Info: &params.UpdateApplicationUnitsInfo{
			Units: []params.ApplicationUnitInfo{
				{ProviderId: "uuid", UnitTag: "unit-gitlab-0"},
				{ProviderId: "another-uuid", UnitTag: "unit-gitlab-1"},
			},
		},
	})
	s.st.application.CheckCallNames(c, "Life", "Name", "GetScale")
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
	// Units with state that disappear from the cluster are deleted
	// if they cause the application scale to be exceeded.
	s.st.application.units[2].(*mockUnit).CheckCallNames(c, "Life", "DestroyOperation", "UpdateOperation")
	s.st.application.units[2].(*mockUnit).CheckCall(c, 2, "UpdateOperation", state.UnitUpdateProperties{
		CloudContainerStatus: &status.StatusInfo{Status: status.Terminated, Message: "unit stopped by the cloud"},
		AgentStatus:          &status.StatusInfo{Status: status.Idle},
	})
	// Units with state that disappear from the cluster are not deleted if
	// the application scale is maintained.
	s.st.application.units[3].(*mockUnit).CheckCallNames(c, "Life", "UpdateOperation")
	s.st.application.units[3].(*mockUnit).CheckCall(c, 1, "UpdateOperation", state.UnitUpdateProperties{
		CloudContainerStatus: &status.StatusInfo{Status: status.Terminated, Message: "unit stopped by the cloud"},
		AgentStatus:          &status.StatusInfo{Status: status.Idle},
	})

	s.storage.CheckCallNames(c,
		"UnitStorageAttachments", "UnitStorageAttachments", "UnitStorageAttachments",
		"StorageInstance", "UnitStorageAttachments", "StorageInstance", "AllFilesystems",
		"Volume", "SetVolumeInfo", "SetVolumeAttachmentInfo", "Volume", "SetStatus", "Volume", "SetStatus",
		"Filesystem", "SetFilesystemInfo", "SetFilesystemAttachmentInfo",
		"Filesystem", "SetStatus", "Filesystem", "SetStatus", "Filesystem", "SetStatus", "Filesystem", "SetStatus")
	s.storage.CheckCall(c, 0, "UnitStorageAttachments", names.NewUnitTag("gitlab/2"))
	s.storage.CheckCall(c, 1, "UnitStorageAttachments", names.NewUnitTag("gitlab/3"))
	s.storage.CheckCall(c, 2, "UnitStorageAttachments", names.NewUnitTag("gitlab/0"))
	s.storage.CheckCall(c, 3, "StorageInstance", names.NewStorageTag("data/0"))
	s.storage.CheckCall(c, 4, "UnitStorageAttachments", names.NewUnitTag("gitlab/1"))
	s.storage.CheckCall(c, 5, "StorageInstance", names.NewStorageTag("data/1"))

	now := s.clock.Now()
	s.storage.CheckCall(c, 8, "SetVolumeInfo",
		names.NewVolumeTag("1"),
		state.VolumeInfo{
			Size:       200,
			VolumeId:   "vol-id2",
			Persistent: true,
		})
	s.storage.CheckCall(c, 9, "SetVolumeAttachmentInfo",
		names.NewUnitTag("gitlab/1"), names.NewVolumeTag("1"),
		state.VolumeAttachmentInfo{
			ReadOnly: true,
		})
	s.storage.CheckCall(c, 11, "SetStatus",
		status.StatusInfo{
			Status:  status.Pending,
			Message: "vol not ready",
			Since:   &now,
		})
	s.storage.CheckCall(c, 13, "SetStatus",
		status.StatusInfo{
			Status:  status.Attached,
			Message: "vol ready",
			Since:   &now,
		})

	s.storage.CheckCall(c, 15, "SetFilesystemInfo",
		names.NewFilesystemTag("gitlab/1/0"),
		state.FilesystemInfo{
			Size:         200,
			FilesystemId: "fs-id2",
		})
	s.storage.CheckCall(c, 16, "SetFilesystemAttachmentInfo",
		names.NewUnitTag("gitlab/1"), names.NewFilesystemTag("gitlab/1/0"),
		state.FilesystemAttachmentInfo{
			MountPoint: "/path/to/there",
			ReadOnly:   true,
		})
	s.storage.CheckCall(c, 20, "SetStatus",
		status.StatusInfo{
			Status:  status.Pending,
			Message: "not ready",
			Since:   &now,
		})
	s.storage.CheckCall(c, 22, "SetStatus",
		status.StatusInfo{
			Status:  status.Attached,
			Message: "ready",
			Since:   &now,
		})
	s.storage.CheckCall(c, 24, "SetStatus",
		status.StatusInfo{
			Status: status.Detached,
			Since:  &now,
		})

	s.st.model.CheckCall(c, 0, "Containers", []string{"another-uuid"})
	s.st.model.CheckCall(c, 1, "Containers", []string{"uuid", "another-uuid"})
}

func (s *CAASProvisionerSuite) TestUpdateApplicationsUnitsWithStorageNoBackingVolume(c *gc.C) {
	ctrl := s.setupFacade(c)
	defer ctrl.Finish()

	s.st.application.units = []caasunitprovisioner.Unit{
		&mockUnit{name: "gitlab/0", containerInfo: &mockContainerInfo{providerId: "uuid"}, life: state.Alive},
	}
	s.storage.backingVolume = names.VolumeTag{}
	s.storage.storageFilesystems[names.NewStorageTag("data/0")] = names.NewFilesystemTag("gitlab/0/0")
	s.storage.storageAttachments[names.NewUnitTag("gitlab/0")] = names.NewStorageTag("data/0")

	units := []params.ApplicationUnitParams{
		{ProviderId: "uuid", Address: "address", Ports: []string{"port"},
			Status: "running", Info: "message", Stateful: true,
			FilesystemInfo: []params.KubernetesFilesystemInfo{
				{StorageName: "data", FilesystemId: "fs-id", Size: 100, MountPoint: "/path/to/here", ReadOnly: true,
					Status: "attached",
				},
			},
		},
	}
	s.st.application.scale = 1
	args := params.UpdateApplicationUnitArgs{
		Args: []params.UpdateApplicationUnits{
			{ApplicationTag: "application-gitlab", Units: units, Scale: intPtr(1), Generation: int64Ptr(1)},
		},
	}
	results, err := s.facade.UpdateApplicationsUnits(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.UpdateApplicationUnitResults{
		Results: []params.UpdateApplicationUnitResult{
			{nil, nil},
		},
	})
	s.st.application.CheckCallNames(c, "Life", "Name", "GetScale")
	s.st.application.units[0].(*mockUnit).CheckCallNames(c, "Life", "UpdateOperation")
	s.st.application.units[0].(*mockUnit).CheckCall(c, 1, "UpdateOperation", state.UnitUpdateProperties{
		ProviderId: strPtr("uuid"),
		Address:    strPtr("address"), Ports: &[]string{"port"},
		CloudContainerStatus: &status.StatusInfo{Status: status.Running, Message: "message"},
		AgentStatus:          &status.StatusInfo{Status: status.Idle},
	})

	s.storage.CheckCallNames(c,
		"UnitStorageAttachments", "StorageInstance", "AllFilesystems", "Filesystem",
		"SetFilesystemInfo", "SetFilesystemAttachmentInfo", "Filesystem", "SetStatus")
	s.storage.CheckCall(c, 0, "UnitStorageAttachments", names.NewUnitTag("gitlab/0"))
	s.storage.CheckCall(c, 1, "StorageInstance", names.NewStorageTag("data/0"))

	now := s.clock.Now()
	s.storage.CheckCall(c, 4, "SetFilesystemInfo",
		names.NewFilesystemTag("gitlab/0/0"),
		state.FilesystemInfo{
			Size:         100,
			FilesystemId: "fs-id",
		})
	s.storage.CheckCall(c, 5, "SetFilesystemAttachmentInfo",
		names.NewUnitTag("gitlab/0"), names.NewFilesystemTag("gitlab/0/0"),
		state.FilesystemAttachmentInfo{
			MountPoint: "/path/to/here",
			ReadOnly:   true,
		})
	s.storage.CheckCall(c, 7, "SetStatus",
		status.StatusInfo{
			Status: status.Attached,
			Since:  &now,
		})
}

func (s *CAASProvisionerSuite) TestUpdateApplicationsService(c *gc.C) {
	ctrl := s.setupFacade(c)
	defer ctrl.Finish()

	addr := network.NewSpaceAddress("10.0.0.1")
	results, err := s.facade.UpdateApplicationsService(params.UpdateApplicationServiceArgs{
		Args: []params.UpdateApplicationServiceArg{
			{
				ApplicationTag: "application-gitlab",
				ProviderId:     "id",
				Addresses:      params.FromMachineAddresses(addr.MachineAddress),
			}, {
				ApplicationTag: "unit-gitlab-0",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[1].Error, jc.DeepEquals, &params.Error{
		Message: `"unit-gitlab-0" is not a valid application tag`,
	})
	c.Assert(s.st.application.providerId, gc.Equals, "id")
	c.Assert(s.st.application.addresses, jc.DeepEquals, []network.SpaceAddress{addr})
}

func (s *CAASProvisionerSuite) TestSetOperatorStatus(c *gc.C) {
	ctrl := s.setupFacade(c)
	defer ctrl.Finish()

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
