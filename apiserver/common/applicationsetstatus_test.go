// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state/testing"
)

type serviceStatusSetterSuite struct {
	testing.StateSuite
	leadershipChecker *fakeLeadershipChecker
	badTag            names.Tag
	setter            *common.ApplicationStatusSetter
}

var _ = gc.Suite(&serviceStatusSetterSuite{})

func (s *serviceStatusSetterSuite) SetUpTest(c *gc.C) {
	c.Skip("skipping factory based tests. TODO: Re-write without factories")
	s.badTag = nil
	s.leadershipChecker = &fakeLeadershipChecker{isLeader: true}
}

func (s *serviceStatusSetterSuite) TestUnauthorized(c *gc.C) {
	// Machines are unauthorized since they are not units
	tag := names.NewUnitTag("foo/0")
	s.badTag = tag
	result, err := s.setter.SetStatus(context.Background(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    tag.String(),
		Status: status.Active.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *serviceStatusSetterSuite) TestNotATag(c *gc.C) {
	result, err := s.setter.SetStatus(context.Background(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    "not a tag",
		Status: status.Active.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, `"not a tag" is not a valid tag`)
}

func (s *serviceStatusSetterSuite) TestNotFound(c *gc.C) {
	result, err := s.setter.SetStatus(context.Background(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    names.NewUnitTag("foo/0").String(),
		Status: status.Active.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeNotFound)
}

func (s *serviceStatusSetterSuite) TestSetMachineStatus(c *gc.C) {
	machine := s.Factory.MakeMachine(c, nil)
	result, err := s.setter.SetStatus(context.Background(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    machine.Tag().String(),
		Status: status.Active.String(),
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
	service := s.Factory.MakeApplication(c, &factory.ApplicationParams{Status: &status.StatusInfo{
		Status: status.Maintenance,
	}})
	result, err := s.setter.SetStatus(context.Background(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    service.Tag().String(),
		Status: status.Active.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	// Can't call set service status on a service. Weird I know, but the only
	// way is to go through the unit leader.
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *serviceStatusSetterSuite) TestSetUnitStatusNotLeader(c *gc.C) {
	// If the unit isn't the leader, it can't set it.
	s.leadershipChecker.isLeader = false
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Status: &status.StatusInfo{
		Status: status.Maintenance,
	}})
	result, err := s.setter.SetStatus(context.Background(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    unit.Tag().String(),
		Status: status.Active.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	status := result.Results[0]
	c.Assert(status.Error, gc.ErrorMatches, "not leader")
}

func (s *serviceStatusSetterSuite) TestSetUnitStatusIsLeader(c *gc.C) {
	service := s.Factory.MakeApplication(c, &factory.ApplicationParams{Status: &status.StatusInfo{
		Status: status.Maintenance,
	}})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: service,
		Status: &status.StatusInfo{
			Status: status.Maintenance,
		}})
	// No need to claim leadership - the checker passed in in setup
	// always returns true.
	result, err := s.setter.SetStatus(context.Background(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    unit.Tag().String(),
		Status: status.Active.String(),
	}}})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	err = service.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	unitStatus, err := service.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitStatus.Status, gc.Equals, status.Active)
}

func (s *serviceStatusSetterSuite) TestBulk(c *gc.C) {
	s.badTag = names.NewMachineTag("42")
	machine := s.Factory.MakeMachine(c, nil)
	result, err := s.setter.SetStatus(context.Background(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    s.badTag.String(),
		Status: status.Active.String(),
	}, {
		Tag:    machine.Tag().String(),
		Status: status.Active.String(),
	}, {
		Tag:    "bad-tag",
		Status: status.Active.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 3)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(result.Results[1].Error, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(result.Results[2].Error, gc.ErrorMatches, `"bad-tag" is not a valid tag`)
}

type fakeLeadershipChecker struct {
	isLeader bool
}

type token struct {
	isLeader bool
}

func (t *token) Check() error {
	if !t.isLeader {
		return errors.New("not leader")
	}
	return nil
}

func (f *fakeLeadershipChecker) LeadershipCheck(applicationName, unitName string) leadership.Token {
	return &token{isLeader: f.isLeader}
}
