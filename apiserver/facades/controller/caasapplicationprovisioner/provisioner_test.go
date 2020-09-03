// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/caasapplicationprovisioner"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

var _ = gc.Suite(&CAASApplicationProvisionerSuite{})

type CAASApplicationProvisionerSuite struct {
	coretesting.BaseSuite

	resources          *common.Resources
	authorizer         *apiservertesting.FakeAuthorizer
	api                *caasapplicationprovisioner.API
	st                 *mockState
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

	s.st = newMockState()
	s.storagePoolManager = &mockStoragePoolManager{}
	s.registry = &mockStorageRegistry{}
	api, err := caasapplicationprovisioner.NewCAASApplicationProvisionerAPI(s.st, s.resources, s.authorizer, s.storagePoolManager, s.registry)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
}

func (s *CAASApplicationProvisionerSuite) TestPermission(c *gc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	_, err := caasapplicationprovisioner.NewCAASApplicationProvisionerAPI(s.st, s.resources, s.authorizer, s.storagePoolManager, s.registry)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *CAASApplicationProvisionerSuite) TestWatchApplications(c *gc.C) {
	applicationNames := []string{"db2", "hadoop"}
	s.st.applicationWatcher.changes <- applicationNames
	result, err := s.api.WatchApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.StringsWatcherId, gc.Equals, "1")
	c.Assert(result.Changes, jc.DeepEquals, applicationNames)

	resource := s.resources.Get("1")
	c.Assert(resource, gc.NotNil)
	c.Assert(resource, gc.Implements, new(state.StringsWatcher))
}

func (s *CAASApplicationProvisionerSuite) TestProvisioningInfo(c *gc.C) {
	s.st.app = &mockApplication{
		life:  state.Alive,
		charm: &mockCharm{meta: &charm.Meta{}},
	}
	result, err := s.api.ProvisioningInfo(params.Entities{Entities: []params.Entity{{"application-gitlab"}}})
	c.Assert(err, jc.ErrorIsNil)

	mc := jc.NewMultiChecker()
	mc.AddExpr(`_.Results[0].CACert`, jc.Ignore)
	c.Assert(result, mc, params.CAASApplicationProvisioningInfoResults{
		Results: []params.CAASApplicationProvisioningInfo{{
			ImagePath:    "jujusolutions/k8sagent:2.6-beta3.666",
			Version:      version.MustParse("2.6-beta3"),
			APIAddresses: []string{"10.0.0.1:1"},
			Tags: map[string]string{
				"juju-model-uuid":      coretesting.ModelTag.Id(),
				"juju-controller-uuid": coretesting.ControllerTag.Id(),
			},
		}},
	})
}

func (s *CAASApplicationProvisionerSuite) TestSetOperatorStatus(c *gc.C) {
	s.st.app = &mockApplication{
		life:  state.Alive,
		charm: &mockCharm{meta: &charm.Meta{}},
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
		life:  state.Alive,
		charm: &mockCharm{meta: &charm.Meta{}},
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
					DeploymentMode: charm.ModeEmbedded,
					DeploymentType: charm.DeploymentStateful,
				},
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
	s.st.app.CheckCallNames(c, "Charm", "AllUnits", "UpdateUnits")
	s.st.model.CheckCallNames(c, "Containers")
	c.Assert(s.st.app.Calls()[2].Args[0].(*state.UpdateUnitsOperation).Deletes, gc.HasLen, 1)
	c.Assert(s.st.app.Calls()[2].Args[0].(*state.UpdateUnitsOperation).Deletes[0], gc.Equals, destroyOp)
	s.st.app.units[1].CheckCallNames(c, "EnsureDead", "DestroyOperation")
}

func (s *CAASApplicationProvisionerSuite) TestGarbageCollectDeployment(c *gc.C) {
	destroyOp := &state.DestroyUnitOperation{}
	s.st.app = &mockApplication{
		life: state.Alive,
		charm: &mockCharm{
			meta: &charm.Meta{
				Deployment: &charm.Deployment{
					DeploymentMode: charm.ModeEmbedded,
					DeploymentType: charm.DeploymentStateless,
				},
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
	s.st.app.CheckCallNames(c, "Charm", "AllUnits", "UpdateUnits")
	s.st.model.CheckCallNames(c, "Containers")
	c.Assert(s.st.app.Calls()[2].Args[0].(*state.UpdateUnitsOperation).Deletes, gc.HasLen, 1)
	c.Assert(s.st.app.Calls()[2].Args[0].(*state.UpdateUnitsOperation).Deletes[0], gc.Equals, destroyOp)
	s.st.app.units[1].CheckCallNames(c, "ContainerInfo", "EnsureDead", "DestroyOperation")
}

func (s *CAASApplicationProvisionerSuite) TestGarbageCollectDaemon(c *gc.C) {
	destroyOp := &state.DestroyUnitOperation{}
	s.st.app = &mockApplication{
		life: state.Alive,
		charm: &mockCharm{
			meta: &charm.Meta{
				Deployment: &charm.Deployment{
					DeploymentMode: charm.ModeEmbedded,
					DeploymentType: charm.DeploymentDaemon,
				},
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
					DeploymentMode: charm.ModeEmbedded,
					DeploymentType: charm.DeploymentDaemon,
				},
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
	s.st.app.CheckCallNames(c, "Life", "Charm", "AllUnits", "UpdateUnits")
	s.st.model.CheckCallNames(c, "Containers")
	c.Assert(s.st.app.Calls()[3].Args[0].(*state.UpdateUnitsOperation).Deletes, gc.HasLen, 4)
	c.Assert(s.st.app.Calls()[3].Args[0].(*state.UpdateUnitsOperation).Deletes[0], gc.Equals, destroyOp)
	c.Assert(s.st.app.Calls()[3].Args[0].(*state.UpdateUnitsOperation).Deletes[1], gc.Equals, destroyOp)
	c.Assert(s.st.app.Calls()[3].Args[0].(*state.UpdateUnitsOperation).Deletes[2], gc.Equals, destroyOp)
	c.Assert(s.st.app.Calls()[3].Args[0].(*state.UpdateUnitsOperation).Deletes[3], gc.Equals, destroyOp)
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
