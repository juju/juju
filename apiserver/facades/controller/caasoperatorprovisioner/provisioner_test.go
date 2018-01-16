// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner_test

import (
	"github.com/juju/juju/status"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/caasoperatorprovisioner"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&CAASProvisionerSuite{})

type CAASProvisionerSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	api        *caasoperatorprovisioner.API
	st         *mockState
}

func (s *CAASProvisionerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}

	s.st = newMockState()
	api, err := caasoperatorprovisioner.NewCAASOperatorProvisionerAPI(s.resources, s.authorizer, s.st)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
}

func (s *CAASProvisionerSuite) TestPermission(c *gc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	_, err := caasoperatorprovisioner.NewCAASOperatorProvisionerAPI(s.resources, s.authorizer, s.st)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *CAASProvisionerSuite) TestWatchApplications(c *gc.C) {
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

func (s *CAASProvisionerSuite) TestSetPasswords(c *gc.C) {
	s.st.app = &mockApplication{
		tag: names.NewApplicationTag("app"),
	}

	args := params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: "application-app", Password: "xxx-12345678901234567890"},
			{Tag: "application-another", Password: "yyy-12345678901234567890"},
		},
	}
	results, err := s.api.SetPasswords(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{&params.Error{Message: "entity application-another not found", Code: "not found"}},
		},
	})
	c.Assert(s.st.app.password, gc.Equals, "xxx-12345678901234567890")
}

func (s *CAASProvisionerSuite) TestUpdateApplicationsUnits(c *gc.C) {
	s.st.app = &mockApplication{
		tag: names.NewApplicationTag("app"),
		units: []caasoperatorprovisioner.Unit{
			&mockUnit{name: "app/0", providerId: "uuid"},
			&mockUnit{name: "app/1"},
			&mockUnit{name: "app/2", providerId: "uuid2"},
		},
	}

	units := []params.ApplicationUnitParams{
		{Id: "uuid", Address: "address", Ports: []string{"port"},
			Status: "running", Info: "message"},
		{Id: "another-uuid", Address: "another-address", Ports: []string{"another-port"},
			Status: "running", Info: "another message"},
		{Id: "last-uuid", Address: "last-address", Ports: []string{"last-port"},
			Status: "running", Info: "last message"},
	}
	args := params.UpdateApplicationUnitArgs{
		Args: []params.UpdateApplicationUnits{
			{ApplicationTag: "application-app", Units: units},
			{ApplicationTag: "application-another", Units: []params.ApplicationUnitParams{}},
		},
	}
	results, err := s.api.UpdateApplicationsUnits(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{&params.Error{Message: "application another not found", Code: "not found"}},
		},
	})
	s.st.app.CheckCallNames(c, "AddOperation")
	s.st.app.CheckCall(c, 0, "AddOperation", state.UnitUpdateProperties{
		ProviderId: "last-uuid",
		// TODO(caas)
		//Address: "last-address", Ports: []string{"last-port"},
		Status: &status.StatusInfo{Status: status.Running, Message: "last message"},
	})
	s.st.app.units[0].(*mockUnit).CheckCallNames(c, "UpdateOperation")
	s.st.app.units[0].(*mockUnit).CheckCall(c, 0, "UpdateOperation", state.UnitUpdateProperties{
		ProviderId: "uuid",
		// TODO(caas)
		// Address: "address", Ports: []string{"port"},
		Status: &status.StatusInfo{Status: status.Running, Message: "message"},
	})
	s.st.app.units[1].(*mockUnit).CheckCallNames(c, "UpdateOperation")
	s.st.app.units[1].(*mockUnit).CheckCall(c, 0, "UpdateOperation", state.UnitUpdateProperties{
		ProviderId: "another-uuid",
		// TODO(caas)
		//Address: "another-address", Ports: []string{"another-port"},
		Status: &status.StatusInfo{Status: status.Running, Message: "another message"},
	})
	s.st.app.units[2].(*mockUnit).CheckCallNames(c, "DestroyOperation")
}
