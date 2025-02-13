// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type statusSetterSuite struct {
	entityFinder *mocks.MockEntityFinder
	setter       *common.StatusSetter
	now          time.Time

	badTag names.Tag
}

var _ = gc.Suite(&statusSetterSuite{})

func (s *statusSetterSuite) SetUpTest(c *gc.C) {
	s.badTag = nil
}

func (s *statusSetterSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.entityFinder = mocks.NewMockEntityFinder(ctrl)

	s.now = time.Now()
	clock := mocks.NewMockClock(ctrl)
	clock.EXPECT().Now().Return(s.now).AnyTimes()

	s.setter = common.NewStatusSetter(s.entityFinder, func() (common.AuthFunc, error) {
		return s.authFunc, nil
	}, clock)
	return ctrl
}

func (s *statusSetterSuite) TestUnauthorized(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewMachineTag("42")
	s.badTag = tag
	result, err := s.setter.SetStatus(context.Background(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    tag.String(),
		Status: status.Executing.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *statusSetterSuite) TestNotATag(c *gc.C) {
	defer s.setupMocks(c).Finish()

	result, err := s.setter.SetStatus(context.Background(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    "not a tag",
		Status: status.Executing.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, `"not a tag" is not a valid tag`)
}

func (s *statusSetterSuite) TestNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewMachineTag("42")
	s.entityFinder.EXPECT().FindEntity(tag).Return(nil, errors.NotFoundf("machine 42"))

	result, err := s.setter.SetStatus(context.Background(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    tag.String(),
		Status: status.Down.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeNotFound)
}

func (s *statusSetterSuite) TestSetMachineStatus(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	entity := newMockStatusSetterEntity(ctrl)
	entity.MockStatusSetter.EXPECT().SetStatus(status.StatusInfo{
		Status: status.Started,
		Since:  &s.now,
	}).Return(nil)

	tag := names.NewMachineTag("42")
	s.entityFinder.EXPECT().FindEntity(tag).Return(entity, nil)

	result, err := s.setter.SetStatus(context.Background(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    tag.String(),
		Status: status.Started.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
}

// The status has to be a valid workload status, because get status
// on the unit returns the workload status not the agent status as it
// does on a machine.
func (s *statusSetterSuite) TestSetUnitStatus(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	entity := newMockStatusSetterEntity(ctrl)
	entity.MockStatusSetter.EXPECT().SetStatus(status.StatusInfo{
		Status: status.Active,
		Since:  &s.now,
	}).Return(nil)

	tag := names.NewUnitTag("wordpress/1")
	s.entityFinder.EXPECT().FindEntity(tag).Return(entity, nil)

	result, err := s.setter.SetStatus(context.Background(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    tag.String(),
		Status: status.Active.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
}

func (s *statusSetterSuite) TestSetServiceStatus(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Calls to set the status of a application should be going through the
	// ApplicationStatusSetter that checks for leadership, so permission denied
	// here.
	entity := &state.Application{}

	tag := names.NewUnitTag("wordpress/1")
	s.entityFinder.EXPECT().FindEntity(tag).Return(entity, nil)

	result, err := s.setter.SetStatus(context.Background(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    tag.String(),
		Status: status.Active.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *statusSetterSuite) TestBulk(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.badTag = names.NewMachineTag("42")
	result, err := s.setter.SetStatus(context.Background(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    s.badTag.String(),
		Status: status.Active.String(),
	}, {
		Tag:    "bad-tag",
		Status: status.Active.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 2)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(result.Results[1].Error, gc.ErrorMatches, `"bad-tag" is not a valid tag`)
}

func (s *statusSetterSuite) authFunc(tag names.Tag) bool {
	return tag != s.badTag
}

type mockStatusSetterEntity struct {
	*mocks.MockStatusSetter
	*mocks.MockEntity
}

func newMockStatusSetterEntity(ctrl *gomock.Controller) *mockStatusSetterEntity {
	return &mockStatusSetterEntity{
		MockStatusSetter: mocks.NewMockStatusSetter(ctrl),
		MockEntity:       mocks.NewMockEntity(ctrl),
	}
}

type unitAgentFinderSuite struct{}

var _ = gc.Suite(&unitAgentFinderSuite{})

func (unitAgentFinderSuite) TestFindEntity(c *gc.C) {
	f := fakeEntityFinder{
		unit: fakeUnit{
			agent: &state.UnitAgent{},
		},
	}
	ua := &common.UnitAgentFinder{EntityFinder: f}
	entity, err := ua.FindEntity(names.NewUnitTag("unit/0"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity, gc.DeepEquals, f.unit.agent)
}

func (unitAgentFinderSuite) TestFindEntityBadTag(c *gc.C) {
	ua := &common.UnitAgentFinder{EntityFinder: fakeEntityFinder{}}
	_, err := ua.FindEntity(names.NewApplicationTag("foo"))
	c.Assert(err, gc.ErrorMatches, "unsupported tag.*")
}

func (unitAgentFinderSuite) TestFindEntityErr(c *gc.C) {
	f := fakeEntityFinder{err: errors.Errorf("boo")}
	ua := &common.UnitAgentFinder{EntityFinder: f}
	_, err := ua.FindEntity(names.NewUnitTag("unit/0"))
	c.Assert(errors.Cause(err), gc.Equals, f.err)
}

type fakeEntityFinder struct {
	unit fakeUnit
	err  error
}

func (f fakeEntityFinder) FindEntity(tag names.Tag) (state.Entity, error) {
	return f.unit, f.err
}

type fakeUnit struct {
	state.Entity
	agent *state.UnitAgent
}

func (f fakeUnit) Agent() *state.UnitAgent {
	return f.agent
}
