// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"

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

type statusGetterSuite struct {
	entityFinder *mocks.MockEntityFinder
	getter       *common.StatusGetter

	badTag names.Tag
}

var _ = gc.Suite(&statusGetterSuite{})

func (s *statusGetterSuite) SetUpTest(c *gc.C) {
	s.badTag = nil
}

func (s *statusGetterSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.entityFinder = mocks.NewMockEntityFinder(ctrl)
	s.getter = common.NewStatusGetter(s.entityFinder, func(ctx context.Context) (common.AuthFunc, error) {
		return s.authFunc, nil
	})
	return ctrl
}

func (s *statusGetterSuite) TestUnauthorized(c *gc.C) {
	defer s.setupMocks(c).Finish()

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
	defer s.setupMocks(c).Finish()

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
	defer s.setupMocks(c).Finish()

	tag := names.NewMachineTag("42")
	s.entityFinder.EXPECT().FindEntity(tag).Return(nil, errors.NotFoundf("machine 42"))

	result, err := s.getter.Status(context.Background(),
		params.Entities{Entities: []params.Entity{{
			Tag: tag.String(),
		}}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeNotFound)
}

func (s *statusGetterSuite) TestGetMachineStatus(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	entity := newMockStatusGetterEntity(ctrl)
	entity.MockStatusGetter.EXPECT().Status().Return(status.StatusInfo{
		Status: status.Pending,
	}, nil)

	tag := names.NewMachineTag("42")
	s.entityFinder.EXPECT().FindEntity(tag).Return(entity, nil)

	result, err := s.getter.Status(context.Background(),
		params.Entities{Entities: []params.Entity{{
			Tag: tag.String(),
		}}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	machineStatus := result.Results[0]
	c.Assert(machineStatus.Error, gc.IsNil)
	c.Assert(machineStatus.Status, gc.Equals, status.Pending.String())
}

func (s *statusGetterSuite) TestGetUnitStatus(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	entity := &state.Unit{}

	tag := names.NewUnitTag("wordpress/1")
	s.entityFinder.EXPECT().FindEntity(tag).Return(entity, nil)

	result, err := s.getter.Status(context.Background(),
		params.Entities{Entities: []params.Entity{{
			Tag: tag.String(),
		}}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *statusGetterSuite) TestGetApplicationStatus(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	entity := newMockStatusGetterEntity(ctrl)
	entity.MockStatusGetter.EXPECT().Status().Return(status.StatusInfo{
		Status: status.Maintenance,
	}, nil)

	tag := names.NewApplicationTag("wordpress")
	s.entityFinder.EXPECT().FindEntity(tag).Return(entity, nil)

	result, err := s.getter.Status(context.Background(),
		params.Entities{Entities: []params.Entity{{
			Tag: tag.String(),
		}}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	appStatus := result.Results[0]
	c.Assert(appStatus.Error, gc.IsNil)
	c.Assert(appStatus.Status, gc.Equals, status.Maintenance.String())
}

func (s *statusGetterSuite) TestBulk(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.badTag = names.NewMachineTag("42")

	entity := newMockStatusGetterEntity(ctrl)
	entity.MockStatusGetter.EXPECT().Status().Return(status.StatusInfo{
		Status: status.Pending,
	}, nil)

	tag := names.NewMachineTag("43")
	s.entityFinder.EXPECT().FindEntity(tag).Return(entity, nil)

	result, err := s.getter.Status(context.Background(),
		params.Entities{Entities: []params.Entity{{
			Tag: s.badTag.String(),
		}, {
			Tag: tag.String(),
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

func (s *statusGetterSuite) authFunc(tag names.Tag) bool {
	return tag != s.badTag
}

type mockStatusGetterEntity struct {
	*mocks.MockStatusGetter
	*mocks.MockEntity
}

func newMockStatusGetterEntity(ctrl *gomock.Controller) *mockStatusGetterEntity {
	return &mockStatusGetterEntity{
		MockStatusGetter: mocks.NewMockStatusGetter(ctrl),
		MockEntity:       mocks.NewMockEntity(ctrl),
	}
}
