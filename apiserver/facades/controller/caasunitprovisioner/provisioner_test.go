// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/caasunitprovisioner"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/status"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/workertest"
)

var _ = gc.Suite(&CAASProvisionerSuite{})

type CAASProvisionerSuite struct {
	coretesting.BaseSuite

	st                  *mockState
	applicationsChanges chan []string
	podSpecChanges      chan struct{}
	unitsChanges        chan []string

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	facade     *caasunitprovisioner.Facade
}

func (s *CAASProvisionerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.applicationsChanges = make(chan []string, 1)
	s.podSpecChanges = make(chan struct{}, 1)
	s.unitsChanges = make(chan []string, 1)
	s.st = &mockState{
		application: mockApplication{
			tag:          names.NewApplicationTag("gitlab"),
			life:         state.Alive,
			unitsWatcher: statetesting.NewMockStringsWatcher(s.unitsChanges),
		},
		applicationsWatcher: statetesting.NewMockStringsWatcher(s.applicationsChanges),
		model: mockModel{
			podSpecWatcher: statetesting.NewMockNotifyWatcher(s.podSpecChanges),
		},
		unit: mockUnit{
			life: state.Dying,
		},
	}
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.st.applicationsWatcher) })
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.st.application.unitsWatcher) })
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.st.model.podSpecWatcher) })

	s.resources = common.NewResources()
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}

	facade, err := caasunitprovisioner.NewFacade(s.resources, s.authorizer, s.st)
	c.Assert(err, jc.ErrorIsNil)
	s.facade = facade
}

func (s *CAASProvisionerSuite) TestPermission(c *gc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	_, err := caasunitprovisioner.NewFacade(s.resources, s.authorizer, s.st)
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

func (s *CAASProvisionerSuite) TestWatchUnits(c *gc.C) {
	s.unitsChanges <- []string{"gitlab/0", "gitlab/1"}

	results, err := s.facade.WatchUnits(params.Entities{
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
	c.Assert(results.Results[0].Changes, jc.DeepEquals, []string{"gitlab/0", "gitlab/1"})
	resource := s.resources.Get("1")
	c.Assert(resource, gc.Equals, s.st.application.unitsWatcher)
}

func (s *CAASProvisionerSuite) TestPodSpec(c *gc.C) {
	results, err := s.facade.PodSpec(params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
			{Tag: "unit-gitlab-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{{
			Result: "spec(gitlab)",
		}, {
			Error: &params.Error{
				Message: `"unit-gitlab-0" is not a valid application tag`,
			},
		}},
	})
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
		&mockUnit{name: "gitlab/3", containerInfo: &mockContainerInfo{providerId: "uuid3"}, life: state.Alive},
	}

	units := []params.ApplicationUnitParams{
		{ProviderId: "uuid", Address: "address", Ports: []string{"port"},
			Status: "running", Info: "message"},
		{ProviderId: "another-uuid", Address: "another-address", Ports: []string{"another-port"},
			Status: "running", Info: "another message"},
		{ProviderId: "last-uuid", Address: "last-address", Ports: []string{"last-port"},
			Status: "error", Info: "last message"},
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
	s.st.application.CheckCallNames(c, "Life", "AddOperation")
	s.st.application.CheckCall(c, 1, "AddOperation", state.UnitUpdateProperties{
		ProviderId: strPtr("really-new-uuid"),
		Address:    strPtr("really-new-address"), Ports: &[]string{"really-new-port"},
		UnitStatus:  &status.StatusInfo{Status: status.Active, Message: "really new message"},
		AgentStatus: &status.StatusInfo{Status: status.Idle},
	})
	s.st.application.units[0].(*mockUnit).CheckCallNames(c, "Life", "UpdateOperation")
	s.st.application.units[0].(*mockUnit).CheckCall(c, 1, "UpdateOperation", state.UnitUpdateProperties{
		ProviderId: strPtr("uuid"),
		Address:    strPtr("address"), Ports: &[]string{"port"},
		UnitStatus:  &status.StatusInfo{Status: status.Active, Message: "message"},
		AgentStatus: &status.StatusInfo{Status: status.Idle},
	})
	s.st.application.units[1].(*mockUnit).CheckCallNames(c, "Life", "UpdateOperation")
	s.st.application.units[1].(*mockUnit).CheckCall(c, 1, "UpdateOperation", state.UnitUpdateProperties{
		ProviderId: strPtr("new-uuid"),
		Address:    strPtr("new-address"), Ports: &[]string{"new-port"},
		UnitStatus:  &status.StatusInfo{Status: status.Active, Message: "new message"},
		AgentStatus: &status.StatusInfo{Status: status.Idle},
	})
	s.st.application.units[2].(*mockUnit).CheckCallNames(c, "Life", "UpdateOperation")
	s.st.application.units[2].(*mockUnit).CheckCall(c, 1, "UpdateOperation", state.UnitUpdateProperties{
		ProviderId: strPtr("another-uuid"),
		Address:    strPtr("another-address"), Ports: &[]string{"another-port"},
		UnitStatus:  &status.StatusInfo{Status: status.Active, Message: "another message"},
		AgentStatus: &status.StatusInfo{Status: status.Idle},
	})
	s.st.application.units[3].(*mockUnit).CheckCallNames(c, "Life", "UpdateOperation")
	s.st.application.units[3].(*mockUnit).CheckCall(c, 1, "UpdateOperation", state.UnitUpdateProperties{
		ProviderId: strPtr("last-uuid"),
		Address:    strPtr("last-address"), Ports: &[]string{"last-port"},
		AgentStatus: &status.StatusInfo{Status: status.Error, Message: "last message"},
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
