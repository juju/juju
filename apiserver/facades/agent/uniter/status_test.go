// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/uniter"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
)

type statusBaseSuite struct {
	testing.StateSuite
	leadershipChecker *fakeLeadershipChecker
	badTag            names.Tag
	model             *fakeCachedModel
	api               *uniter.StatusAPI
}

func (s *statusBaseSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	s.badTag = nil
	s.leadershipChecker = &fakeLeadershipChecker{true}
	s.model = &fakeCachedModel{info: status.StatusInfo{Status: status.Unset}}
	s.api = s.newStatusAPI()
}

func (s *statusBaseSuite) authFunc(tag names.Tag) bool {
	return tag != s.badTag
}

func (s *statusBaseSuite) newStatusAPI() *uniter.StatusAPI {
	st := s.StateSuite.State
	auth := func() (common.AuthFunc, error) {
		return s.authFunc, nil
	}
	return uniter.NewStatusAPI(st, s.model, auth, s.leadershipChecker)
}

type ApplicationStatusAPISuite struct {
	statusBaseSuite
}

var _ = gc.Suite(&ApplicationStatusAPISuite{})

func (s *ApplicationStatusAPISuite) TestUnauthorized(c *gc.C) {
	tag := names.NewUnitTag("foo/0")
	s.badTag = tag
	result, err := s.api.ApplicationStatus(params.Entities{[]params.Entity{{
		tag.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *ApplicationStatusAPISuite) TestNotATag(c *gc.C) {
	result, err := s.api.ApplicationStatus(params.Entities{[]params.Entity{{
		"not a tag",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, `"not a tag" is not a valid tag`)
}

func (s *ApplicationStatusAPISuite) TestNotFound(c *gc.C) {
	result, err := s.api.ApplicationStatus(params.Entities{[]params.Entity{{
		names.NewUnitTag("foo/0").String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeNotFound)
}

func (s *ApplicationStatusAPISuite) TestGetMachineStatus(c *gc.C) {
	machine := s.Factory.MakeMachine(c, nil)
	result, err := s.api.ApplicationStatus(params.Entities{[]params.Entity{{
		machine.Tag().String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	// Can't call application status on a machine.
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *ApplicationStatusAPISuite) TestGetApplicationStatus(c *gc.C) {
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{Status: &status.StatusInfo{
		Status: status.Maintenance,
	}})
	result, err := s.api.ApplicationStatus(params.Entities{[]params.Entity{{
		app.Tag().String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	// Can't call unit status on an application.
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *ApplicationStatusAPISuite) TestGetUnitStatusNotLeader(c *gc.C) {
	// If the unit isn't the leader, it can't get it.
	s.leadershipChecker.isLeader = false
	unit := s.Factory.MakeUnit(c, nil)
	result, err := s.api.ApplicationStatus(params.Entities{[]params.Entity{{
		unit.Tag().String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	status := result.Results[0]
	c.Assert(status.Error, gc.ErrorMatches, ".* not leader .*")
}

func (s *ApplicationStatusAPISuite) TestGetUnitStatusIsLeader(c *gc.C) {
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Status: &status.StatusInfo{
		Status: status.Maintenance,
	}})
	s.model.info.Status = status.Maintenance
	// No need to claim leadership - the checker passed in in setup
	// always returns true.
	result, err := s.api.ApplicationStatus(params.Entities{[]params.Entity{{
		unit.Tag().String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	r := result.Results[0]
	c.Assert(r.Error, gc.IsNil)
	c.Assert(r.Application.Error, gc.IsNil)
	c.Assert(r.Application.Status, gc.Equals, status.Maintenance.String())
	units := r.Units
	c.Assert(units, gc.HasLen, 1)
	unitStatus, ok := units[unit.Name()]
	c.Assert(ok, jc.IsTrue)
	c.Assert(unitStatus.Error, gc.IsNil)
	c.Assert(unitStatus.Status, gc.Equals, status.Maintenance.String())
}

func (s *ApplicationStatusAPISuite) TestBulk(c *gc.C) {
	s.badTag = names.NewMachineTag("42")
	machine := s.Factory.MakeMachine(c, nil)
	result, err := s.api.ApplicationStatus(params.Entities{[]params.Entity{{
		s.badTag.String(),
	}, {
		machine.Tag().String(),
	}, {
		"bad-tag",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 3)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(result.Results[1].Error, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(result.Results[2].Error, gc.ErrorMatches, `"bad-tag" is not a valid tag`)
}

type fakeCachedModel struct {
	err  error
	info status.StatusInfo
}

func (f *fakeCachedModel) Application(name string) (uniter.CachedApplication, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f, nil
}

func (f *fakeCachedModel) Status() status.StatusInfo {
	return f.info
}
