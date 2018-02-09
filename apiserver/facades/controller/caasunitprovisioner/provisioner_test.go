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
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/status"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/workertest"
)

var _ = gc.Suite(&CAASProvisionerSuite{})

type CAASProvisionerSuite struct {
	coretesting.BaseSuite

	st                   *mockState
	applicationsChanges  chan []string
	containerSpecChanges chan struct{}
	unitsChanges         chan []string

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	facade     *caasunitprovisioner.Facade
}

func (s *CAASProvisionerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.applicationsChanges = make(chan []string, 1)
	s.containerSpecChanges = make(chan struct{}, 1)
	s.unitsChanges = make(chan []string, 1)
	s.st = &mockState{
		application: mockApplication{
			tag:          names.NewApplicationTag("gitlab"),
			life:         state.Alive,
			unitsWatcher: statetesting.NewMockStringsWatcher(s.unitsChanges),
		},
		applicationsWatcher: statetesting.NewMockStringsWatcher(s.applicationsChanges),
		model: mockModel{
			containerSpecWatcher: statetesting.NewMockNotifyWatcher(s.containerSpecChanges),
		},
		unit: mockUnit{
			life: state.Dying,
		},
	}
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.st.applicationsWatcher) })
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.st.application.unitsWatcher) })
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.st.model.containerSpecWatcher) })

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

func (s *CAASProvisionerSuite) TestWatchContainerSpec(c *gc.C) {
	s.containerSpecChanges <- struct{}{}

	results, err := s.facade.WatchContainerSpec(params.Entities{
		Entities: []params.Entity{
			{Tag: "unit-gitlab-0"},
			{Tag: "application-gitlab"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[1].Error, jc.DeepEquals, &params.Error{
		Message: `"application-gitlab" is not a valid unit tag`,
	})

	c.Assert(results.Results[0].NotifyWatcherId, gc.Equals, "1")
	resource := s.resources.Get("1")
	c.Assert(resource, gc.Equals, s.st.model.containerSpecWatcher)
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

func (s *CAASProvisionerSuite) TestContainerSpec(c *gc.C) {
	results, err := s.facade.ContainerSpec(params.Entities{
		Entities: []params.Entity{
			{Tag: "unit-gitlab-0"},
			{Tag: "application-gitlab"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{{
			Result: "spec(gitlab/0)",
		}, {
			Error: &params.Error{
				Message: `"application-gitlab" is not a valid unit tag`,
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

func (s *CAASProvisionerSuite) TestUpdateApplicationsUnitsNoTags(c *gc.C) {
	s.st.application.units = []caasunitprovisioner.Unit{
		&mockUnit{name: "gitlab/0", providerId: "uuid", life: state.Alive},
		&mockUnit{name: "gitlab/1", life: state.Alive},
		&mockUnit{name: "gitlab/2", providerId: "uuid2", life: state.Alive},
	}

	units := []params.ApplicationUnitParams{
		{ProviderId: "uuid", Address: "address", Ports: []string{"port"},
			Status: "running", Info: "message"},
		{ProviderId: "another-uuid", Address: "another-address", Ports: []string{"another-port"},
			Status: "running", Info: "another message"},
		{ProviderId: "last-uuid", Address: "last-address", Ports: []string{"last-port"},
			Status: "error", Info: "last message"},
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
	s.st.application.CheckCallNames(c, "AddOperation")
	s.st.application.CheckCall(c, 0, "AddOperation", state.UnitUpdateProperties{
		ProviderId: "last-uuid",
		Address:    "last-address", Ports: []string{"last-port"},
		Status: &status.StatusInfo{Status: status.Error, Message: "last message"},
	})
	s.st.application.units[0].(*mockUnit).CheckCallNames(c, "Life", "UpdateOperation")
	s.st.application.units[0].(*mockUnit).CheckCall(c, 1, "UpdateOperation", state.UnitUpdateProperties{
		ProviderId: "uuid",
		Address:    "address", Ports: []string{"port"},
	})
	s.st.application.units[1].(*mockUnit).CheckCallNames(c, "Life", "UpdateOperation")
	s.st.application.units[1].(*mockUnit).CheckCall(c, 1, "UpdateOperation", state.UnitUpdateProperties{
		ProviderId: "another-uuid",
		Address:    "another-address", Ports: []string{"another-port"},
		Status: &status.StatusInfo{Status: status.Running, Message: "another message"},
	})
	s.st.application.units[2].(*mockUnit).CheckCallNames(c, "Life")
}

func (s *CAASProvisionerSuite) TestUpdateApplicationsUnitsWithTags(c *gc.C) {
	s.st.application.units = []caasunitprovisioner.Unit{
		&mockUnit{name: "gitlab/0", life: state.Alive},
		&mockUnit{name: "gitlab/1", life: state.Alive},
		&mockUnit{name: "gitlab/2", providerId: "uuid2", life: state.Alive},
	}

	units := []params.ApplicationUnitParams{
		{ProviderId: "uuid", UnitTag: "unit-gitlab-0", Address: "address", Ports: []string{"port"},
			Status: "running", Info: "message"},
		{ProviderId: "another-uuid", UnitTag: "unit-gitlab-1", Address: "another-address", Ports: []string{"another-port"},
			Status: "error", Info: "another message"},
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
	s.st.application.units[0].(*mockUnit).CheckCallNames(c, "Life", "UpdateOperation")
	s.st.application.units[0].(*mockUnit).CheckCall(c, 1, "UpdateOperation", state.UnitUpdateProperties{
		ProviderId: "uuid",
		Address:    "address", Ports: []string{"port"},
	})
	s.st.application.units[1].(*mockUnit).CheckCallNames(c, "Life", "UpdateOperation")
	s.st.application.units[1].(*mockUnit).CheckCall(c, 1, "UpdateOperation", state.UnitUpdateProperties{
		ProviderId: "another-uuid",
		Address:    "another-address", Ports: []string{"another-port"},
		Status: &status.StatusInfo{Status: status.Error, Message: "another message"},
	})
	s.st.application.units[2].(*mockUnit).CheckCallNames(c, "Life")
}
