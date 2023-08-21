// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
)

type statusGetterSuite struct {
	statusBaseSuite
	getter *common.StatusGetter
}

var _ = gc.Suite(&statusGetterSuite{})

func (s *statusGetterSuite) SetUpTest(c *gc.C) {
	s.statusBaseSuite.SetUpTest(c)

	s.getter = common.NewStatusGetter(s.State, func() (common.AuthFunc, error) {
		return s.authFunc, nil
	})
}

func (s *statusGetterSuite) TestUnauthorized(c *gc.C) {
	tag := names.NewMachineTag("42")
	s.badTag = tag
	result, err := s.getter.Status(context.Background(),
		params.Entities{Entities: []params.Entity{{
			Tag: tag.String(),
		}}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *statusGetterSuite) TestNotATag(c *gc.C) {
	result, err := s.getter.Status(context.Background(),
		params.Entities{Entities: []params.Entity{{
			Tag: "not a tag",
		}}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, `"not a tag" is not a valid tag`)
}

func (s *statusGetterSuite) TestNotFound(c *gc.C) {
	result, err := s.getter.Status(context.Background(),
		params.Entities{Entities: []params.Entity{{
			Tag: names.NewMachineTag("42").String(),
		}}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeNotFound)
}

func (s *statusGetterSuite) TestGetMachineStatus(c *gc.C) {
	machine := s.Factory.MakeMachine(c, nil)
	result, err := s.getter.Status(context.Background(),
		params.Entities{Entities: []params.Entity{{
			Tag: machine.Tag().String(),
		}}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	machineStatus := result.Results[0]
	c.Assert(machineStatus.Error, gc.IsNil)
	c.Assert(machineStatus.Status, gc.Equals, status.Pending.String())
}

func (s *statusGetterSuite) TestGetUnitStatus(c *gc.C) {
	// The status has to be a valid workload status, because get status
	// on the unit returns the workload status not the agent status as it
	// does on a machine.
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Status: &status.StatusInfo{
		Status: status.Maintenance,
	}})
	result, err := s.getter.Status(context.Background(),
		params.Entities{Entities: []params.Entity{{
			Tag: unit.Tag().String(),
		}}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	unitStatus := result.Results[0]
	c.Assert(unitStatus.Error, gc.IsNil)
	c.Assert(unitStatus.Status, gc.Equals, status.Maintenance.String())
}

func (s *statusGetterSuite) TestGetApplicationStatus(c *gc.C) {
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{Status: &status.StatusInfo{
		Status: status.Maintenance,
	}})
	result, err := s.getter.Status(context.Background(),
		params.Entities{Entities: []params.Entity{{
			Tag: app.Tag().String(),
		}}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	appStatus := result.Results[0]
	c.Assert(appStatus.Error, gc.IsNil)
	c.Assert(appStatus.Status, gc.Equals, status.Maintenance.String())
}

func (s *statusGetterSuite) TestBulk(c *gc.C) {
	s.badTag = names.NewMachineTag("42")
	machine := s.Factory.MakeMachine(c, nil)
	result, err := s.getter.Status(context.Background(),
		params.Entities{Entities: []params.Entity{{
			Tag: s.badTag.String(),
		}, {
			Tag: machine.Tag().String(),
		}, {
			Tag: "bad-tag",
		}}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 3)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(result.Results[1].Error, gc.IsNil)
	c.Assert(result.Results[1].Status, gc.Equals, status.Pending.String())
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

type statusBaseSuite struct {
	testing.StateSuite
	leadershipChecker *fakeLeadershipChecker
	badTag            names.Tag
}

func (s *statusBaseSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	s.badTag = nil
	s.leadershipChecker = &fakeLeadershipChecker{isLeader: true}
}

func (s *statusBaseSuite) authFunc(tag names.Tag) bool {
	return tag != s.badTag
}
