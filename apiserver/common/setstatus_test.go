// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type statusSetterSuite struct {
	statusBaseSuite
	setter *common.StatusSetter
}

var _ = gc.Suite(&statusSetterSuite{})

func (s *statusSetterSuite) SetUpTest(c *gc.C) {
	s.statusBaseSuite.SetUpTest(c)

	s.setter = common.NewStatusSetter(s.State, func() (common.AuthFunc, error) {
		return s.authFunc, nil
	})
}

func (s *statusSetterSuite) TestUnauthorized(c *gc.C) {
	tag := names.NewMachineTag("42")
	s.badTag = tag
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    tag.String(),
		Status: params.StatusExecuting,
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *statusSetterSuite) TestNotATag(c *gc.C) {
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    "not a tag",
		Status: params.StatusExecuting,
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, `"not a tag" is not a valid tag`)
}

func (s *statusSetterSuite) TestNotFound(c *gc.C) {
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    names.NewMachineTag("42").String(),
		Status: params.StatusDown,
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeNotFound)
}

func (s *statusSetterSuite) TestSetMachineStatus(c *gc.C) {
	machine := s.Factory.MakeMachine(c, nil)
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    machine.Tag().String(),
		Status: params.StatusStarted,
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	err = machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	status, err := machine.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Status, gc.Equals, state.StatusStarted)
}

func (s *statusSetterSuite) TestSetUnitStatus(c *gc.C) {
	// The status has to be a valid workload status, because get status
	// on the unit returns the workload status not the agent status as it
	// does on a machine.
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Status: &state.StatusInfo{
		Status: state.StatusMaintenance,
	}})
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    unit.Tag().String(),
		Status: params.StatusActive,
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	err = unit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	status, err := unit.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Status, gc.Equals, state.StatusActive)
}

func (s *statusSetterSuite) TestSetServiceStatus(c *gc.C) {
	// Calls to set the status of a service should be going through the
	// ServiceStatusSetter that checks for leadership, so permission denied
	// here.
	service := s.Factory.MakeService(c, &factory.ServiceParams{Status: &state.StatusInfo{
		Status: state.StatusMaintenance,
	}})
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    service.Tag().String(),
		Status: params.StatusActive,
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)

	err = service.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	status, err := service.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Status, gc.Equals, state.StatusMaintenance)
}

func (s *statusSetterSuite) TestBulk(c *gc.C) {
	s.badTag = names.NewMachineTag("42")
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    s.badTag.String(),
		Status: params.StatusActive,
	}, {
		Tag:    "bad-tag",
		Status: params.StatusActive,
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 2)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(result.Results[1].Error, gc.ErrorMatches, `"bad-tag" is not a valid tag`)
}

type serviceStatusSetterSuite struct {
	statusBaseSuite
	setter *common.ServiceStatusSetter
}

var _ = gc.Suite(&serviceStatusSetterSuite{})

func (s *serviceStatusSetterSuite) SetUpTest(c *gc.C) {
	s.statusBaseSuite.SetUpTest(c)

	s.setter = common.NewServiceStatusSetter(s.State, func() (common.AuthFunc, error) {
		return s.authFunc, nil
	})
}

func (s *serviceStatusSetterSuite) TestUnauthorized(c *gc.C) {
	// Machines are unauthorized since they are not units
	tag := names.NewUnitTag("foo/0")
	s.badTag = tag
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    tag.String(),
		Status: params.StatusActive,
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *serviceStatusSetterSuite) TestNotATag(c *gc.C) {
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    "not a tag",
		Status: params.StatusActive,
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, `"not a tag" is not a valid tag`)
}

func (s *serviceStatusSetterSuite) TestNotFound(c *gc.C) {
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    names.NewUnitTag("foo/0").String(),
		Status: params.StatusActive,
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeNotFound)
}

func (s *serviceStatusSetterSuite) TestSetMachineStatus(c *gc.C) {
	machine := s.Factory.MakeMachine(c, nil)
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    machine.Tag().String(),
		Status: params.StatusActive,
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	// Can't call set service status on a machine.
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *serviceStatusSetterSuite) TestSetServiceStatus(c *gc.C) {
	// TODO: the correct way to fix this is to have the authorizer on the
	// simple status setter to check to see if the unit (authTag) is a leader
	// and able to set the service status. However, that is for another day.
	service := s.Factory.MakeService(c, &factory.ServiceParams{Status: &state.StatusInfo{
		Status: state.StatusMaintenance,
	}})
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    service.Tag().String(),
		Status: params.StatusActive,
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	// Can't call set service status on a service. Weird I know, but the only
	// way is to go through the unit leader.
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *serviceStatusSetterSuite) TestSetUnitStatusNotLeader(c *gc.C) {
	// If the unit isn't the leader, it can't set it.
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Status: &state.StatusInfo{
		Status: state.StatusMaintenance,
	}})
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    unit.Tag().String(),
		Status: params.StatusActive,
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	status := result.Results[0]
	c.Assert(status.Error, gc.ErrorMatches, ".* is not leader of .*")
}

func (s *serviceStatusSetterSuite) TestSetUnitStatusIsLeader(c *gc.C) {
	service := s.Factory.MakeService(c, &factory.ServiceParams{Status: &state.StatusInfo{
		Status: state.StatusMaintenance,
	}})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{
		Service: service,
		Status: &state.StatusInfo{
			Status: state.StatusMaintenance,
		}})
	s.State.LeadershipClaimer().ClaimLeadership(
		service.Name(),
		unit.Name(),
		time.Minute)
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    unit.Tag().String(),
		Status: params.StatusActive,
	}}})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	err = service.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	status, err := service.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Status, gc.Equals, state.StatusActive)
}

func (s *serviceStatusSetterSuite) TestBulk(c *gc.C) {
	s.badTag = names.NewMachineTag("42")
	machine := s.Factory.MakeMachine(c, nil)
	result, err := s.setter.SetStatus(params.SetStatus{[]params.EntityStatusArgs{{
		Tag:    s.badTag.String(),
		Status: params.StatusActive,
	}, {
		Tag:    machine.Tag().String(),
		Status: params.StatusActive,
	}, {
		Tag:    "bad-tag",
		Status: params.StatusActive,
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 3)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(result.Results[1].Error, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(result.Results[2].Error, gc.ErrorMatches, `"bad-tag" is not a valid tag`)
}
