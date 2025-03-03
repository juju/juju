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
	applicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/rpc/params"
)

type statusBaseSuite struct {
	applicationService *uniter.MockApplicationService
	leadershipChecker  *fakeLeadershipChecker
	now                time.Time
	badTag             names.Tag
	api                *uniter.StatusAPI
}

func (s *statusBaseSuite) SetUpTest(c *gc.C) {
	s.badTag = nil
	s.leadershipChecker = &fakeLeadershipChecker{true}
}

func (s *statusBaseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.applicationService = uniter.NewMockApplicationService(ctrl)

	s.now = time.Now()
	clock := uniter.NewMockClock(ctrl)
	clock.EXPECT().Now().Return(s.now).AnyTimes()

	auth := func() (common.AuthFunc, error) {
		return s.authFunc, nil
	}
	s.api = uniter.NewStatusAPI(nil, s.applicationService, auth, s.leadershipChecker, clock)

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

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return("", applicationerrors.ApplicationNotFound)

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
	c.Check(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
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
	c.Check(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *ApplicationStatusAPISuite) TestGetUnitStatusNotLeader(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// If the unit isn't the leader, it can't get it.
	s.leadershipChecker.isLeader = false
	unitTag := names.NewUnitTag("foo/0")
	result, err := s.api.ApplicationStatus(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: unitTag.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results, gc.HasLen, 1)
	status := result.Results[0]
	c.Check(status.Error, gc.ErrorMatches, ".* not leader .*")
}

func (s *ApplicationStatusAPISuite) TestGetUnitStatusIsLeader(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appID := applicationtesting.GenApplicationUUID(c)
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(appID, nil)
	s.applicationService.EXPECT().GetApplicationDisplayStatus(gomock.Any(), appID).Return(&status.StatusInfo{
		Status: status.Maintenance,
	}, nil)
	s.applicationService.EXPECT().GetUnitWorkloadStatusesForApplication(gomock.Any(), appID).Return(map[coreunit.Name]status.StatusInfo{
		"foo/0": {
			Status: status.Maintenance,
		},
	}, nil)

	unitTag := names.NewUnitTag("foo/3")
	// No need to claim leadership - the checker passed in in setup
	// always returns true.
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

	s.badTag = names.NewMachineTag("42")
	machineTag := names.NewMachineTag("42")
	result, err := s.api.ApplicationStatus(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: s.badTag.String(),
	}, {
		Tag: machineTag.String(),
	}, {
		Tag: "bad-tag",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results, gc.HasLen, 3)
	c.Check(result.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
	c.Check(result.Results[1].Error, jc.Satisfies, params.IsCodeUnauthorized)
	c.Check(result.Results[2].Error, gc.ErrorMatches, `"bad-tag" is not a valid tag`)
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

	s.applicationService.EXPECT().SetUnitWorkloadStatus(gomock.Any(), coreunit.Name("ubuntu/42"), gomock.Any()).Return(applicationerrors.UnitNotFound)

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

	s.applicationService.EXPECT().SetUnitWorkloadStatus(gomock.Any(), coreunit.Name("ubuntu/42"), &sInfo).Return(nil)

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

	s.applicationService.EXPECT().GetUnitWorkloadStatus(gomock.Any(), coreunit.Name("ubuntu/42")).Return(nil, applicationerrors.UnitNotFound)

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

	s.applicationService.EXPECT().GetUnitWorkloadStatus(gomock.Any(), coreunit.Name("ubuntu/42")).Return(&sInfo, nil)

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
