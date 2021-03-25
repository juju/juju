// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"time"

	"github.com/juju/charm/v9"
	"github.com/juju/charm/v9/resource"
	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/systems"
	"github.com/juju/systems/channel"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/caasapplicationprovisioner"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/status"
	jujuresource "github.com/juju/juju/resource"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

var _ = gc.Suite(&CAASApplicationProvisionerSuite{})

type CAASApplicationProvisionerSuite struct {
	coretesting.BaseSuite
	clock clock.Clock

	resources          *common.Resources
	authorizer         *apiservertesting.FakeAuthorizer
	api                *caasapplicationprovisioner.API
	st                 *mockState
	storage            *mockStorage
	storagePoolManager *mockStoragePoolManager
	registry           *mockStorageRegistry
}

func (s *CAASApplicationProvisionerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })
	s.PatchValue(&jujuversion.OfficialBuild, 666)

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}

	s.clock = testclock.NewClock(time.Now())
	s.st = newMockState()
	s.storage = &mockStorage{
		storageFilesystems: make(map[names.StorageTag]names.FilesystemTag),
		storageVolumes:     make(map[names.StorageTag]names.VolumeTag),
		storageAttachments: make(map[names.UnitTag]names.StorageTag),
		backingVolume:      names.NewVolumeTag("66"),
	}
	s.storagePoolManager = &mockStoragePoolManager{}
	s.registry = &mockStorageRegistry{}
	newResourceOpener := func(appName string) (jujuresource.Opener, error) {
		return &mockResourceOpener{appName: appName, resources: s.st.resource}, nil
	}
	api, err := caasapplicationprovisioner.NewCAASApplicationProvisionerAPI(
		s.st, s.st, s.resources, newResourceOpener, s.authorizer, s.storage, s.storagePoolManager, s.registry, s.clock)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
}

func (s *CAASApplicationProvisionerSuite) TestPermission(c *gc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	_, err := caasapplicationprovisioner.NewCAASApplicationProvisionerAPI(
		s.st, s.st, s.resources, nil, s.authorizer, s.storage, s.storagePoolManager, s.registry, s.clock)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *CAASApplicationProvisionerSuite) TestProvisioningInfo(c *gc.C) {
	s.st.app = &mockApplication{
		life: state.Alive,
		charm: &mockCharm{
			meta: &charm.Meta{},
			url: &charm.URL{
				Schema:   "cs",
				Name:     "gitlab",
				Revision: -1,
			},
		},
		charmModifiedVersion: 10,
	}
	result, err := s.api.ProvisioningInfo(params.Entities{Entities: []params.Entity{{"application-gitlab"}}})
	c.Assert(err, jc.ErrorIsNil)

	mc := jc.NewMultiChecker()
	mc.AddExpr(`_.Results[0].CACert`, jc.Ignore)
	c.Assert(result, mc, params.CAASApplicationProvisioningInfoResults{
		Results: []params.CAASApplicationProvisioningInfo{{
			ImagePath:    "jujusolutions/jujud-operator:2.6-beta3.666",
			Version:      version.MustParse("2.6-beta3"),
			APIAddresses: []string{"10.0.0.1:1"},
			Tags: map[string]string{
				"juju-model-uuid":      coretesting.ModelTag.Id(),
				"juju-controller-uuid": coretesting.ControllerTag.Id(),
			},
			CharmURL:             "cs:gitlab",
			CharmModifiedVersion: 10,
		}},
	})
}

func (s *CAASApplicationProvisionerSuite) TestSetOperatorStatus(c *gc.C) {
	s.st.app = &mockApplication{
		life: state.Alive,
		charm: &mockCharm{
			meta: &charm.Meta{},
			url: &charm.URL{
				Schema:   "cs",
				Name:     "gitlab",
				Revision: -1,
			},
		},
	}
	result, err := s.api.SetOperatorStatus(params.SetStatus{
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
	s.st.app = &mockApplication{
		life: state.Alive,
		charm: &mockCharm{
			meta: &charm.Meta{},
			url: &charm.URL{
				Schema:   "cs",
				Name:     "gitlab",
				Revision: -1,
			},
		},
		units: []*mockUnit{
			{tag: names.NewUnitTag("gitlab/0")},
			{tag: names.NewUnitTag("gitlab/1")},
			{tag: names.NewUnitTag("gitlab/2")},
		},
	}
	result, err := s.api.Units(params.Entities{
		Entities: []params.Entity{{
			Tag: "application-gitlab",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].Entities, gc.DeepEquals, []params.Entity{
		{Tag: "unit-gitlab-0"},
		{Tag: "unit-gitlab-1"},
		{Tag: "unit-gitlab-2"},
	})
}

func (s *CAASApplicationProvisionerSuite) TestGarbageCollectStateful(c *gc.C) {
	destroyOp := &state.DestroyUnitOperation{}
	s.st.app = &mockApplication{
		life: state.Alive,
		charm: &mockCharm{
			meta: &charm.Meta{
				Deployment: &charm.Deployment{
					DeploymentType: charm.DeploymentStateful,
				},
				// charm.FormatV2.
				Bases: []systems.Base{
					{
						Name: "ubuntu",
						Channel: channel.Channel{
							Name:  "20.04/stable",
							Risk:  "stable",
							Track: "20.04",
						},
					},
				},
			},
			url: &charm.URL{
				Schema:   "cs",
				Name:     "gitlab",
				Revision: -1,
			},
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
				destroyOp: destroyOp,
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
	result, err := s.api.CAASApplicationGarbageCollect(params.CAASApplicationGarbageCollectArgs{
		Args: []params.CAASApplicationGarbageCollectArg{{
			Application:     params.Entity{Tag: "application-gitlab"},
			ObservedUnits:   params.Entities{Entities: []params.Entity{{"unit-gitlab-0"}, {"unit-gitlab-1"}}},
			DesiredReplicas: 1,
			ActivePodNames:  []string{"gitlab-0"},
			Force:           false,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Error, gc.IsNil)
	s.st.CheckCallNames(c, "Application", "Model")
	s.st.app.CheckCallNames(c, "AllUnits", "UpdateUnits")
	s.st.model.CheckCallNames(c, "Containers")
	c.Assert(s.st.app.Calls()[1].Args[0].(*state.UpdateUnitsOperation).Deletes, gc.HasLen, 1)
	c.Assert(s.st.app.Calls()[1].Args[0].(*state.UpdateUnitsOperation).Deletes[0], gc.Equals, destroyOp)
	s.st.app.units[1].CheckCallNames(c, "UpdateOperation", "DestroyOperation")
}

func (s *CAASApplicationProvisionerSuite) TestGarbageCollectDeployment(c *gc.C) {
	c.Skip("skip for now, because of the TODO in CAASApplicationGarbageCollect facade: hardcoded deploymentType := caas.DeploymentStateful")

	destroyOp := &state.DestroyUnitOperation{}
	s.st.app = &mockApplication{
		life: state.Alive,
		charm: &mockCharm{
			meta: &charm.Meta{
				Deployment: &charm.Deployment{
					DeploymentType: charm.DeploymentStateless,
				},
				// charm.FormatV2.
				Bases: []systems.Base{
					{
						Name: "ubuntu",
						Channel: channel.Channel{
							Name:  "20.04/stable",
							Risk:  "stable",
							Track: "20.04",
						},
					},
				},
			},
			url: &charm.URL{
				Schema:   "cs",
				Name:     "gitlab",
				Revision: -1,
			},
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
				destroyOp: destroyOp,
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
	result, err := s.api.CAASApplicationGarbageCollect(params.CAASApplicationGarbageCollectArgs{
		Args: []params.CAASApplicationGarbageCollectArg{{
			Application:     params.Entity{Tag: "application-gitlab"},
			ObservedUnits:   params.Entities{Entities: []params.Entity{{"unit-gitlab-0"}, {"unit-gitlab-1"}}},
			DesiredReplicas: 3,
			ActivePodNames:  []string{"gitlab-0"},
			Force:           false,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Error, gc.IsNil)
	s.st.CheckCallNames(c, "Application", "Model")
	s.st.app.CheckCallNames(c, "AllUnits", "UpdateUnits")
	s.st.model.CheckCallNames(c, "Containers")
	c.Assert(s.st.app.Calls()[1].Args[0].(*state.UpdateUnitsOperation).Deletes, gc.HasLen, 1)
	c.Assert(s.st.app.Calls()[1].Args[0].(*state.UpdateUnitsOperation).Deletes[0], gc.Equals, destroyOp)
	s.st.app.units[1].CheckCallNames(c, "ContainerInfo", "EnsureDead", "DestroyOperation")
}

func (s *CAASApplicationProvisionerSuite) TestGarbageCollectDaemon(c *gc.C) {
	c.Skip("skip for now, because of the TODO in CAASApplicationGarbageCollect facade: hardcoded deploymentType := caas.DeploymentStateful")

	destroyOp := &state.DestroyUnitOperation{}
	s.st.app = &mockApplication{
		life: state.Alive,
		charm: &mockCharm{
			meta: &charm.Meta{
				Deployment: &charm.Deployment{
					DeploymentType: charm.DeploymentDaemon,
				},
				// charm.FormatV2.
				Bases: []systems.Base{
					{
						Name: "ubuntu",
						Channel: channel.Channel{
							Name:  "20.04/stable",
							Risk:  "stable",
							Track: "20.04",
						},
					},
				},
			},
			url: &charm.URL{
				Schema:   "cs",
				Name:     "gitlab",
				Revision: -1,
			},
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
				destroyOp: destroyOp,
			},
			{
				tag: names.NewUnitTag("gitlab/2"),
				containerInfo: &mockCloudContainer{
					unit:       "gitlab/2",
					providerId: "gitlab-2",
				},
			},
			{
				tag: names.NewUnitTag("gitlab/3"),
			},
		},
	}
	s.st.app.units[3].SetErrors(errors.NotFoundf("cloud container"))
	result, err := s.api.CAASApplicationGarbageCollect(params.CAASApplicationGarbageCollectArgs{
		Args: []params.CAASApplicationGarbageCollectArg{{
			Application:     params.Entity{Tag: "application-gitlab"},
			ObservedUnits:   params.Entities{Entities: []params.Entity{{"unit-gitlab-0"}, {"unit-gitlab-1"}, {"unit-gitlab-3"}}},
			DesiredReplicas: 3,
			ActivePodNames:  []string{"gitlab-0"},
			Force:           false,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Error, gc.IsNil)
	s.st.CheckCallNames(c, "Application", "Model")
	s.st.app.CheckCallNames(c, "Charm", "AllUnits", "UpdateUnits")
	s.st.model.CheckCallNames(c, "Containers")
	c.Assert(s.st.app.Calls()[2].Args[0].(*state.UpdateUnitsOperation).Deletes, gc.HasLen, 1)
	c.Assert(s.st.app.Calls()[2].Args[0].(*state.UpdateUnitsOperation).Deletes[0], gc.Equals, destroyOp)
	s.st.app.units[1].CheckCallNames(c, "ContainerInfo", "EnsureDead", "DestroyOperation")
}

func (s *CAASApplicationProvisionerSuite) TestGarbageCollectForced(c *gc.C) {
	destroyOp := &state.DestroyUnitOperation{}
	s.st.app = &mockApplication{
		life: state.Dying,
		charm: &mockCharm{
			meta: &charm.Meta{
				Deployment: &charm.Deployment{
					DeploymentType: charm.DeploymentDaemon,
				},
				// charm.FormatV2.
				Bases: []systems.Base{
					{
						Name: "ubuntu",
						Channel: channel.Channel{
							Name:  "20.04/stable",
							Risk:  "stable",
							Track: "20.04",
						},
					},
				},
			},
			url: &charm.URL{
				Schema:   "cs",
				Name:     "gitlab",
				Revision: -1,
			},
		},
		units: []*mockUnit{
			{
				tag: names.NewUnitTag("gitlab/0"),
				containerInfo: &mockCloudContainer{
					unit:       "gitlab/0",
					providerId: "gitlab-0",
				},
				destroyOp: destroyOp,
			},
			{
				tag: names.NewUnitTag("gitlab/1"),
				containerInfo: &mockCloudContainer{
					unit:       "gitlab/1",
					providerId: "gitlab-1",
				},
				destroyOp: destroyOp,
			},
			{
				tag: names.NewUnitTag("gitlab/2"),
				containerInfo: &mockCloudContainer{
					unit:       "gitlab/2",
					providerId: "gitlab-2",
				},
				destroyOp: destroyOp,
			},
			{
				tag:       names.NewUnitTag("gitlab/3"),
				destroyOp: destroyOp,
			},
		},
	}
	result, err := s.api.CAASApplicationGarbageCollect(params.CAASApplicationGarbageCollectArgs{
		Args: []params.CAASApplicationGarbageCollectArg{{
			Application:     params.Entity{Tag: "application-gitlab"},
			ObservedUnits:   params.Entities{Entities: []params.Entity{{"unit-gitlab-0"}, {"unit-gitlab-1"}, {"unit-gitlab-3"}}},
			DesiredReplicas: 3,
			ActivePodNames:  []string{"gitlab-0"},
			Force:           true,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Error, gc.IsNil)
	s.st.CheckCallNames(c, "Application", "Model")
	s.st.app.CheckCallNames(c, "Life", "AllUnits", "UpdateUnits")
	s.st.model.CheckCallNames(c, "Containers")
	c.Assert(s.st.app.Calls()[2].Args[0].(*state.UpdateUnitsOperation).Deletes, gc.HasLen, 4)
	c.Assert(s.st.app.Calls()[2].Args[0].(*state.UpdateUnitsOperation).Deletes[0], gc.Equals, destroyOp)
	c.Assert(s.st.app.Calls()[2].Args[0].(*state.UpdateUnitsOperation).Deletes[1], gc.Equals, destroyOp)
	c.Assert(s.st.app.Calls()[2].Args[0].(*state.UpdateUnitsOperation).Deletes[2], gc.Equals, destroyOp)
	c.Assert(s.st.app.Calls()[2].Args[0].(*state.UpdateUnitsOperation).Deletes[3], gc.Equals, destroyOp)
}

func (s *CAASApplicationProvisionerSuite) TestApplicationCharmURLs(c *gc.C) {
	s.st.app = &mockApplication{
		life: state.Alive,
		charm: &mockCharm{
			meta: &charm.Meta{},
			url: &charm.URL{
				Schema:   "cs",
				Name:     "gitlab",
				Revision: -1,
			},
		},
	}
	result, err := s.api.ApplicationCharmURLs(params.Entities{
		Entities: []params.Entity{{
			Tag: "application-gitlab",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].Result, gc.Equals, "cs:gitlab")
}

func (s *CAASApplicationProvisionerSuite) TestApplicationOCIResources(c *gc.C) {
	s.st.app = &mockApplication{
		tag:  names.NewApplicationTag("gitlab"),
		life: state.Alive,
		charm: &mockCharm{
			meta: &charm.Meta{
				Resources: map[string]resource.Meta{
					"gitlab-image": {
						Name: "gitlab-image",
						Type: resource.TypeContainerImage,
					},
				},
			},
			url: &charm.URL{
				Schema:   "cs",
				Name:     "gitlab",
				Revision: -1,
			},
		},
	}
	s.st.resource = &mockResources{
		resource: &resources.DockerImageDetails{
			RegistryPath: "gitlab:latest",
			Username:     "jujuqa",
			Password:     "pwd",
		},
	}

	result, err := s.api.ApplicationOCIResources(params.Entities{
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
	s.st.app = &mockApplication{
		tag:  names.NewApplicationTag("gitlab"),
		life: state.Alive,
		charm: &mockCharm{
			meta: &charm.Meta{
				Deployment: &charm.Deployment{
					DeploymentType: charm.DeploymentStateful,
				},
				// charm.FormatV2.
				Bases: []systems.Base{
					{
						Name: "ubuntu",
						Channel: channel.Channel{
							Name:  "20.04/stable",
							Risk:  "stable",
							Track: "20.04",
						},
					},
				},
			},
			url: &charm.URL{
				Schema:   "cs",
				Name:     "gitlab",
				Revision: -1,
			},
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
			{ApplicationTag: "application-gitlab", Units: units},
		},
	}
	results, err := s.api.UpdateApplicationsUnits(args)
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

	s.storage.CheckCallNames(c,
		"UnitStorageAttachments", "StorageInstance", "UnitStorageAttachments", "StorageInstance",
		"AllFilesystems", "Volume", "SetVolumeInfo", "SetVolumeAttachmentInfo", "Volume", "SetStatus",
		"Volume", "SetStatus", "Filesystem", "SetFilesystemInfo", "SetFilesystemAttachmentInfo",
		"Filesystem", "SetStatus", "Filesystem", "SetStatus")
	s.storage.CheckCall(c, 0, "UnitStorageAttachments", names.NewUnitTag("gitlab/0"))
	s.storage.CheckCall(c, 1, "StorageInstance", names.NewStorageTag("data/0"))
	s.storage.CheckCall(c, 2, "UnitStorageAttachments", names.NewUnitTag("gitlab/1"))
	s.storage.CheckCall(c, 3, "StorageInstance", names.NewStorageTag("data/1"))

	now := s.clock.Now()
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
	s.st.app = &mockApplication{
		tag:  names.NewApplicationTag("gitlab"),
		life: state.Alive,
		charm: &mockCharm{
			meta: &charm.Meta{
				Deployment: &charm.Deployment{
					DeploymentType: charm.DeploymentStateful,
				},
				// charm.FormatV2.
				Bases: []systems.Base{
					{
						Name: "ubuntu",
						Channel: channel.Channel{
							Name:  "20.04/stable",
							Risk:  "stable",
							Track: "20.04",
						},
					},
				},
			},
			url: &charm.URL{
				Schema:   "cs",
				Name:     "gitlab",
				Revision: -1,
			},
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
	results, err := s.api.UpdateApplicationsUnits(args)
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
