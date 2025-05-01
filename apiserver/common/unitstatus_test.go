// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"time"

	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	statuserrors "github.com/juju/juju/domain/status/errors"
	"github.com/juju/juju/rpc/params"
)

type unitStatusSuite struct {
	statusService *mocks.MockStatusService
	now           time.Time
	clock         *mocks.MockClock

	badTag names.Tag
}

func (s *unitStatusSuite) SetUpTest(c *gc.C) {
	s.badTag = nil
}

func (s *unitStatusSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.statusService = mocks.NewMockStatusService(ctrl)

	s.now = time.Now()
	s.clock = mocks.NewMockClock(ctrl)
	s.clock.EXPECT().Now().Return(s.now).AnyTimes()

	return ctrl
}

func (s *unitStatusSuite) authFunc(tag names.Tag) bool {
	return tag != s.badTag
}

type unitSetStatusSuite struct {
	unitStatusSuite
}

var _ = gc.Suite(&unitSetStatusSuite{})

func (s *unitSetStatusSuite) TestSetStatusUnauthorised(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewUnitTag("ubuntu/42")
	s.badTag = tag

	setter := common.NewUnitStatusSetter(s.statusService, s.clock, func(ctx context.Context) (common.AuthFunc, error) {
		return s.authFunc, nil
	})
	result, err := setter.SetStatus(context.Background(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    tag.String(),
		Status: status.Executing.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *unitSetStatusSuite) TestSetStatusNotATag(c *gc.C) {
	defer s.setupMocks(c).Finish()

	setter := common.NewUnitStatusSetter(s.statusService, s.clock, func(ctx context.Context) (common.AuthFunc, error) {
		return s.authFunc, nil
	})
	result, err := setter.SetStatus(context.Background(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    "not a tag",
		Status: status.Executing.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, `"not a tag" is not a valid tag`)
}

func (s *unitSetStatusSuite) TestSetStatusNotAUnitTag(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewMachineTag("42")

	setter := common.NewUnitStatusSetter(s.statusService, s.clock, func(ctx context.Context) (common.AuthFunc, error) {
		return s.authFunc, nil
	})
	result, err := setter.SetStatus(context.Background(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    tag.String(),
		Status: status.Executing.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, `"machine-42" is not a valid unit tag`)
}

func (s *unitSetStatusSuite) TestSetStatusUnitNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewUnitTag("ubuntu/42")

	s.statusService.EXPECT().SetUnitWorkloadStatus(gomock.Any(), unit.Name("ubuntu/42"), gomock.Any()).Return(statuserrors.UnitNotFound)

	setter := common.NewUnitStatusSetter(s.statusService, s.clock, func(ctx context.Context) (common.AuthFunc, error) {
		return s.authFunc, nil
	})
	result, err := setter.SetStatus(context.Background(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    tag.String(),
		Status: status.Executing.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeNotFound)
}

func (s *unitSetStatusSuite) TestSetStatus(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	tag := names.NewUnitTag("ubuntu/42")

	sInfo := status.StatusInfo{
		Status:  status.Active,
		Message: "msg",
		Data: map[string]interface{}{
			"key": "value",
		},
		Since: &s.now,
	}

	s.statusService.EXPECT().SetUnitWorkloadStatus(gomock.Any(), unit.Name("ubuntu/42"), sInfo).Return(nil)

	setter := common.NewUnitStatusSetter(s.statusService, s.clock, func(ctx context.Context) (common.AuthFunc, error) {
		return s.authFunc, nil
	})

	result, err := setter.SetStatus(context.Background(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    tag.String(),
		Status: status.Active.String(),
		Info:   "msg",
		Data: map[string]interface{}{
			"key": "value",
		},
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
}

type unitGetStatusSuite struct {
	unitStatusSuite
}

var _ = gc.Suite(&unitGetStatusSuite{})

func (s *unitGetStatusSuite) TestStatusUnauthorised(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewUnitTag("ubuntu/42")
	s.badTag = tag

	getter := common.NewUnitStatusGetter(s.statusService, s.clock, func(ctx context.Context) (common.AuthFunc, error) {
		return s.authFunc, nil
	})
	result, err := getter.Status(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: tag.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *unitGetStatusSuite) TestStatusNotATag(c *gc.C) {
	defer s.setupMocks(c).Finish()

	getter := common.NewUnitStatusGetter(s.statusService, s.clock, func(ctx context.Context) (common.AuthFunc, error) {
		return s.authFunc, nil
	})
	result, err := getter.Status(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: "not a tag",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, `"not a tag" is not a valid tag`)
}

func (s *unitGetStatusSuite) TestStatusNotAUnitTag(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewMachineTag("42")

	getter := common.NewUnitStatusGetter(s.statusService, s.clock, func(ctx context.Context) (common.AuthFunc, error) {
		return s.authFunc, nil
	})
	result, err := getter.Status(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: tag.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, `"machine-42" is not a valid unit tag`)
}

func (s *unitGetStatusSuite) TestStatusUnitNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewUnitTag("ubuntu/42")

	s.statusService.EXPECT().GetUnitWorkloadStatus(gomock.Any(), unit.Name("ubuntu/42")).Return(status.StatusInfo{}, statuserrors.UnitNotFound)

	getter := common.NewUnitStatusGetter(s.statusService, s.clock, func(ctx context.Context) (common.AuthFunc, error) {
		return s.authFunc, nil
	})
	result, err := getter.Status(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: tag.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, jc.Satisfies, params.IsCodeNotFound)
}

func (s *unitGetStatusSuite) TestStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewUnitTag("ubuntu/42")

	s.statusService.EXPECT().GetUnitWorkloadStatus(gomock.Any(), unit.Name("ubuntu/42")).Return(status.StatusInfo{
		Status:  status.Active,
		Message: "msg",
		Data: map[string]interface{}{
			"key": "value",
		},
		Since: &s.now,
	}, nil)

	getter := common.NewUnitStatusGetter(s.statusService, s.clock, func(ctx context.Context) (common.AuthFunc, error) {
		return s.authFunc, nil
	})
	result, err := getter.Status(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: tag.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0], gc.DeepEquals, params.StatusResult{
		Status: status.Active.String(),
		Info:   "msg",
		Data: map[string]interface{}{
			"key": "value",
		},
		Since: &s.now,
	})
}
