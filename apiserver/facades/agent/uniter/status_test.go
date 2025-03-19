// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"
	"time"

	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/uniter"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	statuserrors "github.com/juju/juju/domain/status/errors"
	"github.com/juju/juju/rpc/params"
)

type statusBaseSuite struct {
	statusService *uniter.MockStatusService
	now           time.Time
	badTag        names.Tag
	api           *uniter.StatusAPI
}

func (s *statusBaseSuite) SetUpTest(c *gc.C) {
	s.badTag = nil
}

func (s *statusBaseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.statusService = uniter.NewMockStatusService(ctrl)

	s.now = time.Now()
	clock := uniter.NewMockClock(ctrl)
	clock.EXPECT().Now().Return(s.now).AnyTimes()

	auth := func() (common.AuthFunc, error) {
		return s.authFunc, nil
	}
	s.api = uniter.NewStatusAPI(nil, s.statusService, auth, nil, clock)

	return ctrl
}

func (s *statusBaseSuite) authFunc(tag names.Tag) bool {
	return tag != s.badTag
}

type ApplicationStatusAPISuite struct {
	statusBaseSuite
}

var _ = gc.Suite(&ApplicationStatusAPISuite{})

func (s *ApplicationStatusAPISuite) TestUnauthorized(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewUnitTag("foo/0")
	s.badTag = tag
	result, err := s.api.ApplicationStatus(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: tag.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results, gc.HasLen, 1)
	c.Check(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *ApplicationStatusAPISuite) TestNotATag(c *gc.C) {
	defer s.setupMocks(c).Finish()

	result, err := s.api.ApplicationStatus(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: "not a tag",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results, gc.HasLen, 1)
	c.Check(result.Results[0].Error, gc.ErrorMatches, `"not a tag" is not a valid tag`)
}

func (s *ApplicationStatusAPISuite) TestNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.statusService.EXPECT().GetApplicationAndUnitStatusesForUnitWithLeader(gomock.Any(), coreunit.Name("foo/0")).Return(nil, nil, statuserrors.ApplicationNotFound)

	result, err := s.api.ApplicationStatus(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: names.NewUnitTag("foo/0").String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results, gc.HasLen, 1)
	c.Check(result.Results[0].Error, jc.Satisfies, params.IsCodeNotFound)
}

func (s *ApplicationStatusAPISuite) TestGetMachineStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machineTag := names.NewMachineTag("42")

	result, err := s.api.ApplicationStatus(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: machineTag.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results, gc.HasLen, 1)
	// Can't call application status on a machine.
	c.Check(result.Results[0].Error, gc.ErrorMatches, ".*is not a valid unit tag.*")
}

func (s *ApplicationStatusAPISuite) TestGetApplicationStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appTag := names.NewApplicationTag("foo")

	result, err := s.api.ApplicationStatus(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: appTag.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results, gc.HasLen, 1)
	// Can't call unit status on an application.
	c.Check(result.Results[0].Error, gc.ErrorMatches, ".*is not a valid unit tag.*")
}

func (s *ApplicationStatusAPISuite) TestGetUnitStatusNotLeader(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitTag := names.NewUnitTag("foo/0")

	s.statusService.EXPECT().GetApplicationAndUnitStatusesForUnitWithLeader(gomock.Any(), coreunit.Name("foo/0")).Return(nil, nil, statuserrors.UnitNotLeader)

	result, err := s.api.ApplicationStatus(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: unitTag.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results, gc.HasLen, 1)
	c.Check(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *ApplicationStatusAPISuite) TestGetUnitStatusIsLeader(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.statusService.EXPECT().GetApplicationAndUnitStatusesForUnitWithLeader(gomock.Any(), coreunit.Name("foo/3")).Return(
		&status.StatusInfo{
			Status: status.Maintenance,
		},
		map[coreunit.Name]status.StatusInfo{
			"foo/0": {
				Status: status.Maintenance,
			},
		}, nil)

	unitTag := names.NewUnitTag("foo/3")
	result, err := s.api.ApplicationStatus(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: unitTag.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results, gc.HasLen, 1)
	r := result.Results[0]
	c.Check(r.Error, gc.IsNil)
	c.Check(r.Application.Error, gc.IsNil)
	c.Check(r.Application.Status, gc.Equals, status.Maintenance.String())
	units := r.Units
	c.Check(units, gc.HasLen, 1)
	unitStatus, ok := units["foo/0"]
	c.Check(ok, jc.IsTrue)
	c.Check(unitStatus.Error, gc.IsNil)
	c.Check(unitStatus.Status, gc.Equals, status.Maintenance.String())
}

func (s *ApplicationStatusAPISuite) TestBulk(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.badTag = names.NewUnitTag("foo/42")
	result, err := s.api.ApplicationStatus(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: s.badTag.String(),
	}, {
		Tag: "bad-tag",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results, gc.HasLen, 2)
	c.Check(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
	c.Check(result.Results[1].Error, gc.ErrorMatches, `"bad-tag" is not a valid tag`)
}

type UnitStatusAPISuite struct {
	statusBaseSuite
}

var _ = gc.Suite(&UnitStatusAPISuite{})

func (s *UnitStatusAPISuite) TestSetUnitStatusUnauthorized(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewUnitTag("foo/0")
	s.badTag = tag
	result, err := s.api.SetUnitStatus(context.Background(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    tag.String(),
		Status: status.Active.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results, gc.HasLen, 1)
	c.Check(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *UnitStatusAPISuite) TestSetUnitStatusNotATag(c *gc.C) {
	defer s.setupMocks(c).Finish()

	result, err := s.api.SetUnitStatus(context.Background(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    "not a tag",
		Status: status.Active.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results, gc.HasLen, 1)
	c.Check(result.Results[0].Error, gc.ErrorMatches, `"not a tag" is not a valid tag`)
}

func (s *UnitStatusAPISuite) TestSetUnitStatusNotAUnitTag(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewMachineTag("42")

	result, err := s.api.SetUnitStatus(context.Background(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    tag.String(),
		Status: status.Active.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results, gc.HasLen, 1)
	c.Check(result.Results[0].Error, gc.ErrorMatches, `"machine-42" is not a valid unit tag`)
}

func (s *UnitStatusAPISuite) TestSetUnitStatusUnitNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewUnitTag("ubuntu/42")

	s.statusService.EXPECT().SetUnitWorkloadStatus(gomock.Any(), coreunit.Name("ubuntu/42"), gomock.Any()).Return(statuserrors.UnitNotFound)

	result, err := s.api.SetUnitStatus(context.Background(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    tag.String(),
		Status: status.Active.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results, gc.HasLen, 1)
	c.Check(result.Results[0].Error, jc.Satisfies, params.IsCodeNotFound)
}

func (s *UnitStatusAPISuite) TestSetUnitStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewUnitTag("ubuntu/42")

	sInfo := status.StatusInfo{
		Status:  status.Active,
		Message: "msg",
		Data: map[string]interface{}{
			"key": "value",
		},
		Since: &s.now,
	}

	s.statusService.EXPECT().SetUnitWorkloadStatus(gomock.Any(), coreunit.Name("ubuntu/42"), &sInfo).Return(nil)

	result, err := s.api.SetUnitStatus(context.Background(), params.SetStatus{Entities: []params.EntityStatusArgs{{
		Tag:    tag.String(),
		Status: status.Active.String(),
		Info:   "msg",
		Data: map[string]interface{}{
			"key": "value",
		},
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results, gc.HasLen, 1)
	c.Check(result.Results[0].Error, gc.IsNil)
}

func (s *UnitStatusAPISuite) TestUnitStatusUnauthorized(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewUnitTag("foo/0")
	s.badTag = tag
	result, err := s.api.UnitStatus(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: tag.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results, gc.HasLen, 1)
	c.Check(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *UnitStatusAPISuite) TestUnitStatusNotATag(c *gc.C) {
	defer s.setupMocks(c).Finish()

	result, err := s.api.UnitStatus(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: "not a tag",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results, gc.HasLen, 1)
	c.Check(result.Results[0].Error, gc.ErrorMatches, `"not a tag" is not a valid tag`)
}

func (s *UnitStatusAPISuite) TestUnitStatusNotAUnitTag(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewMachineTag("42")

	result, err := s.api.UnitStatus(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: tag.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results, gc.HasLen, 1)
	c.Check(result.Results[0].Error, gc.ErrorMatches, `"machine-42" is not a valid unit tag`)
}

func (s *UnitStatusAPISuite) TestUnitStatusUnitNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewUnitTag("ubuntu/42")

	s.statusService.EXPECT().GetUnitWorkloadStatus(gomock.Any(), coreunit.Name("ubuntu/42")).Return(nil, statuserrors.UnitNotFound)

	result, err := s.api.UnitStatus(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: tag.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results, gc.HasLen, 1)
	c.Check(result.Results[0].Error, jc.Satisfies, params.IsCodeNotFound)
}

func (s *UnitStatusAPISuite) TestUnitStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewUnitTag("ubuntu/42")

	sInfo := status.StatusInfo{
		Status:  status.Active,
		Message: "msg",
		Data: map[string]interface{}{
			"key": "value",
		},
		Since: &s.now,
	}

	s.statusService.EXPECT().GetUnitWorkloadStatus(gomock.Any(), coreunit.Name("ubuntu/42")).Return(&sInfo, nil)

	result, err := s.api.UnitStatus(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: tag.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results, gc.HasLen, 1)
	c.Check(result.Results[0].Error, gc.IsNil)
	c.Check(result.Results[0].Status, gc.Equals, status.Active.String())
	c.Check(result.Results[0].Info, gc.Equals, "msg")
	c.Check(result.Results[0].Data, gc.DeepEquals, map[string]interface{}{
		"key": "value",
	})
	c.Check(result.Results[0].Since, gc.DeepEquals, &s.now)
}
