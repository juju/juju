// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"testing"
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

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

func (s *unitStatusSuite) SetUpTest(c *tc.C) {
	s.badTag = nil
}

func (s *unitStatusSuite) setupMocks(c *tc.C) *gomock.Controller {
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

func TestUnitSetStatusSuite(t *testing.T) {
	tc.Run(t, &unitSetStatusSuite{})
}

func (s *unitSetStatusSuite) TestSetStatusUnauthorised(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewUnitTag("ubuntu/42")
	s.badTag = tag

	setter := common.NewUnitStatusSetter(s.statusService, s.clock, func(ctx context.Context) (common.AuthFunc, error) {
		return s.authFunc, nil
	})
	result, err := setter.SetStatus(c.Context(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    tag.String(),
		Status: status.Executing.String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *unitSetStatusSuite) TestSetStatusNotATag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	setter := common.NewUnitStatusSetter(s.statusService, s.clock, func(ctx context.Context) (common.AuthFunc, error) {
		return s.authFunc, nil
	})
	result, err := setter.SetStatus(c.Context(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    "not a tag",
		Status: status.Executing.String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.ErrorMatches, `"not a tag" is not a valid tag`)
}

func (s *unitSetStatusSuite) TestSetStatusNotAUnitTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewMachineTag("42")

	setter := common.NewUnitStatusSetter(s.statusService, s.clock, func(ctx context.Context) (common.AuthFunc, error) {
		return s.authFunc, nil
	})
	result, err := setter.SetStatus(c.Context(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    tag.String(),
		Status: status.Executing.String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.ErrorMatches, `"machine-42" is not a valid unit tag`)
}

func (s *unitSetStatusSuite) TestSetStatusUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewUnitTag("ubuntu/42")

	s.statusService.EXPECT().SetUnitWorkloadStatus(gomock.Any(), unit.Name("ubuntu/42"), gomock.Any()).Return(statuserrors.UnitNotFound)

	setter := common.NewUnitStatusSetter(s.statusService, s.clock, func(ctx context.Context) (common.AuthFunc, error) {
		return s.authFunc, nil
	})
	result, err := setter.SetStatus(c.Context(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    tag.String(),
		Status: status.Executing.String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *unitSetStatusSuite) TestSetStatus(c *tc.C) {
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

	result, err := setter.SetStatus(c.Context(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    tag.String(),
		Status: status.Active.String(),
		Info:   "msg",
		Data: map[string]interface{}{
			"key": "value",
		},
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)
}

type unitGetStatusSuite struct {
	unitStatusSuite
}

func TestUnitGetStatusSuite(t *testing.T) {
	tc.Run(t, &unitGetStatusSuite{})
}

func (s *unitGetStatusSuite) TestStatusUnauthorised(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewUnitTag("ubuntu/42")
	s.badTag = tag

	getter := common.NewUnitStatusGetter(s.statusService, s.clock, func(ctx context.Context) (common.AuthFunc, error) {
		return s.authFunc, nil
	})
	result, err := getter.Status(c.Context(), params.Entities{Entities: []params.Entity{{
		Tag: tag.String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *unitGetStatusSuite) TestStatusNotATag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	getter := common.NewUnitStatusGetter(s.statusService, s.clock, func(ctx context.Context) (common.AuthFunc, error) {
		return s.authFunc, nil
	})
	result, err := getter.Status(c.Context(), params.Entities{Entities: []params.Entity{{
		Tag: "not a tag",
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.ErrorMatches, `"not a tag" is not a valid tag`)
}

func (s *unitGetStatusSuite) TestStatusNotAUnitTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewMachineTag("42")

	getter := common.NewUnitStatusGetter(s.statusService, s.clock, func(ctx context.Context) (common.AuthFunc, error) {
		return s.authFunc, nil
	})
	result, err := getter.Status(c.Context(), params.Entities{Entities: []params.Entity{{
		Tag: tag.String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.ErrorMatches, `"machine-42" is not a valid unit tag`)
}

func (s *unitGetStatusSuite) TestStatusUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewUnitTag("ubuntu/42")

	s.statusService.EXPECT().GetUnitWorkloadStatus(gomock.Any(), unit.Name("ubuntu/42")).Return(status.StatusInfo{}, statuserrors.UnitNotFound)

	getter := common.NewUnitStatusGetter(s.statusService, s.clock, func(ctx context.Context) (common.AuthFunc, error) {
		return s.authFunc, nil
	})
	result, err := getter.Status(c.Context(), params.Entities{Entities: []params.Entity{{
		Tag: tag.String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *unitGetStatusSuite) TestStatus(c *tc.C) {
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
	result, err := getter.Status(c.Context(), params.Entities{Entities: []params.Entity{{
		Tag: tag.String(),
	}}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0], tc.DeepEquals, params.StatusResult{
		Status: status.Active.String(),
		Info:   "msg",
		Data: map[string]interface{}{
			"key": "value",
		},
		Since: &s.now,
	})
}
