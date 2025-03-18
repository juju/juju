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

func (s *serviceStatusSetterSuite) TestStub(c *gc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- Veryfying that setting application status when identifying as an application fails.
- Veryfying that setting application status as leader unit works and affects the unit's reported status.
- Verifying that setting application status as a non-leader fails.
`)
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
