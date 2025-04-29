// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	applicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/lease"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/status"
	statuserrors "github.com/juju/juju/domain/status/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type leaderServiceSuite struct {
	state         *MockState
	leadership    *MockEnsurer
	statusHistory *statusHistoryRecorder

	service *LeadershipService
}

var _ = gc.Suite(&leaderServiceSuite{})

func (s *leaderServiceSuite) TestSetRelationStatus(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := unittesting.GenNewName(c, "app/0")

	sts := corestatus.StatusInfo{
		Status:  corestatus.Broken,
		Message: "message",
		Since:   ptr(time.Now()),
	}

	expectedStatus := status.StatusInfo[status.RelationStatusType]{
		Status:  status.RelationStatusTypeBroken,
		Message: sts.Message,
		Since:   sts.Since,
	}
	s.leadership.EXPECT().WithLeader(gomock.Any(), unitName.Application(), unitName.String(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _, _ string, fn func(context.Context) error) error {
			return fn(ctx)
		},
	)
	s.state.EXPECT().SetRelationStatus(gomock.Any(), relationUUID, expectedStatus).Return(nil)

	// Act
	err := s.service.SetRelationStatus(context.Background(), unitName, relationUUID, sts)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
}

func (s *leaderServiceSuite) TestSetRelationStatusRelationNotFound(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := unittesting.GenNewName(c, "app/0")
	sts := corestatus.StatusInfo{
		Status: corestatus.Broken,
		Since:  ptr(time.Now()),
	}
	expectedStatus := status.StatusInfo[status.RelationStatusType]{
		Status: status.RelationStatusTypeBroken,
		Since:  sts.Since,
	}
	s.leadership.EXPECT().WithLeader(gomock.Any(), unitName.Application(), unitName.String(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _, _ string, fn func(context.Context) error) error {
			return fn(ctx)
		},
	)
	s.state.EXPECT().SetRelationStatus(gomock.Any(), relationUUID, expectedStatus).Return(statuserrors.RelationNotFound)

	// Act
	err := s.service.SetRelationStatus(context.Background(), unitName, relationUUID, sts)

	// Assert
	c.Assert(err, jc.ErrorIs, statuserrors.RelationNotFound)
}

func (s *leaderServiceSuite) TestSetApplicationStatusForUnitLeader(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	applicationUUID := applicationtesting.GenApplicationUUID(c)
	unitName := coreunit.Name("foo/666")

	s.leadership.EXPECT().WithLeader(gomock.Any(), "foo", unitName.String(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _, _ string, fn func(context.Context) error) error {
			return fn(ctx)
		})

	s.state.EXPECT().GetApplicationIDAndNameByUnitName(gomock.Any(), unitName).Return(applicationUUID, "foo", nil)
	s.state.EXPECT().SetApplicationStatus(gomock.Any(), applicationUUID, status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "doink",
		Data:    []byte(`{"foo":"bar"}`),
		Since:   &now,
	})

	err := s.service.SetApplicationStatusForUnitLeader(context.Background(), unitName, corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *leaderServiceSuite) TestSetApplicationStatusForUnitLeaderNotLeader(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	applicationUUID := applicationtesting.GenApplicationUUID(c)
	unitName := coreunit.Name("foo/666")

	s.leadership.EXPECT().WithLeader(gomock.Any(), "foo", unitName.String(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _, _ string, fn func(context.Context) error) error {
			return lease.ErrNotHeld
		})

	s.state.EXPECT().GetApplicationIDAndNameByUnitName(gomock.Any(), unitName).Return(applicationUUID, "foo", nil)

	err := s.service.SetApplicationStatusForUnitLeader(context.Background(), unitName, corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIs, statuserrors.UnitNotLeader)
}

func (s *leaderServiceSuite) TestSetApplicationStatusForUnitLeaderInvalidUnitName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	unitName := coreunit.Name("!!!!")
	err := s.service.SetApplicationStatusForUnitLeader(context.Background(), unitName, corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *leaderServiceSuite) TestSetApplicationStatusForUnitLeaderNoUnitFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	applicationUUID := applicationtesting.GenApplicationUUID(c)
	unitName := coreunit.Name("foo/666")

	s.state.EXPECT().GetApplicationIDAndNameByUnitName(gomock.Any(), unitName).
		Return(applicationUUID, "foo", statuserrors.UnitNotFound)

	err := s.service.SetApplicationStatusForUnitLeader(context.Background(), unitName, corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *leaderServiceSuite) TestGetApplicationAndUnitStatusesForUnitWithLeaderNotLeader(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	applicationUUID := applicationtesting.GenApplicationUUID(c)

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)

	s.leadership.EXPECT().WithLeader(gomock.Any(), "foo", unitName.String(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _, _ string, fn func(context.Context) error) error {
			return lease.ErrNotHeld
		})

	_, _, err := s.service.GetApplicationAndUnitStatusesForUnitWithLeader(context.Background(), unitName)
	c.Assert(err, jc.ErrorIs, statuserrors.UnitNotLeader)
}

func (s *leaderServiceSuite) TestGetApplicationAndUnitStatusesForUnitWithLeaderInvalidUnitName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, _, err := s.service.GetApplicationAndUnitStatusesForUnitWithLeader(context.Background(), coreunit.Name("!!!"))
	c.Assert(err, jc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *leaderServiceSuite) TestGetApplicationAndUnitStatusesForUnitWithLeaderNoApplication(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return("", statuserrors.ApplicationNotFound)
	_, _, err := s.service.GetApplicationAndUnitStatusesForUnitWithLeader(context.Background(), unitName)
	c.Assert(err, jc.ErrorIs, statuserrors.ApplicationNotFound)
}

func (s *leaderServiceSuite) TestGetApplicationAndUnitStatusesForUnitWithLeaderApplicationStatusSet(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	now := time.Now()
	applicationUUID := applicationtesting.GenApplicationUUID(c)

	s.leadership.EXPECT().WithLeader(gomock.Any(), "foo", unitName.String(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _, _ string, fn func(context.Context) error) error {
			return fn(ctx)
		})

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)

	s.state.EXPECT().GetApplicationStatus(gomock.Any(), applicationUUID).Return(
		status.StatusInfo[status.WorkloadStatusType]{
			Status:  status.WorkloadStatusActive,
			Message: "doink",
			Data:    []byte(`{"foo":"bar"}`),
			Since:   &now,
		}, nil)

	s.state.EXPECT().GetAllFullUnitStatusesForApplication(gomock.Any(), applicationUUID).Return(
		status.FullUnitStatuses{
			"foo/0": {
				WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
					Status:  status.WorkloadStatusActive,
					Message: "boink",
					Data:    []byte(`{"foo":"baz"}`),
					Since:   &now,
				},
				AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusIdle,
				},
				Present: true,
			},
			"foo/1": {
				WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
					Status:  status.WorkloadStatusBlocked,
					Message: "poink",
					Data:    []byte(`{"foo":"bat"}`),
					Since:   &now,
				},
				AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusIdle,
				},
				Present: true,
			},
		}, nil)

	applicationStatus, unitWorkloadStatuses, err := s.service.GetApplicationAndUnitStatusesForUnitWithLeader(context.Background(), unitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(applicationStatus, jc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Check(unitWorkloadStatuses, jc.DeepEquals, map[coreunit.Name]corestatus.StatusInfo{
		"foo/0": {
			Status:  corestatus.Active,
			Message: "boink",
			Data:    map[string]interface{}{"foo": "baz"},
			Since:   &now,
		},
		"foo/1": {
			Status:  corestatus.Blocked,
			Message: "poink",
			Data:    map[string]interface{}{"foo": "bat"},
			Since:   &now,
		},
	})
}

func (s *leaderServiceSuite) TestGetApplicationAndUnitStatusesForUnitWithLeaderApplicationStatusUnset(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	now := time.Now()
	applicationUUID := applicationtesting.GenApplicationUUID(c)

	s.leadership.EXPECT().WithLeader(gomock.Any(), "foo", unitName.String(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _, _ string, fn func(context.Context) error) error {
			return fn(ctx)
		})

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)

	s.state.EXPECT().GetApplicationStatus(gomock.Any(), applicationUUID).Return(
		status.StatusInfo[status.WorkloadStatusType]{
			Status: status.WorkloadStatusUnset,
		}, nil)

	s.state.EXPECT().GetAllFullUnitStatusesForApplication(gomock.Any(), applicationUUID).Return(
		status.FullUnitStatuses{
			"foo/0": {
				WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
					Status:  status.WorkloadStatusActive,
					Message: "boink",
					Data:    []byte(`{"foo":"baz"}`),
					Since:   &now,
				},
				AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusIdle,
				},
				K8sPodStatus: status.StatusInfo[status.K8sPodStatusType]{
					Status:  status.K8sPodStatusBlocked,
					Message: "zoink",
					Data:    []byte(`{"foo":"baz"}`),
					Since:   &now,
				},
				Present: true,
			},
			"foo/1": {
				WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
					Status:  status.WorkloadStatusActive,
					Message: "poink",
					Data:    []byte(`{"foo":"bat"}`),
					Since:   &now,
				},
				AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusIdle,
				},
				K8sPodStatus: status.StatusInfo[status.K8sPodStatusType]{
					Status:  status.K8sPodStatusRunning,
					Message: "yoink",
					Data:    []byte(`{"foo":"bat"}`),
					Since:   &now,
				},
				Present: true,
			},
		}, nil)

	applicationStatus, unitWorkloadStatuses, err := s.service.GetApplicationAndUnitStatusesForUnitWithLeader(context.Background(), unitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(applicationStatus, jc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Blocked,
		Message: "zoink",
		Data:    map[string]interface{}{"foo": "baz"},
		Since:   &now,
	})
	c.Check(unitWorkloadStatuses, jc.DeepEquals, map[coreunit.Name]corestatus.StatusInfo{
		"foo/0": {
			Status:  corestatus.Active,
			Message: "boink",
			Data:    map[string]interface{}{"foo": "baz"},
			Since:   &now,
		},
		"foo/1": {
			Status:  corestatus.Active,
			Message: "poink",
			Data:    map[string]interface{}{"foo": "bat"},
			Since:   &now,
		},
	})
}

func (s *leaderServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.leadership = NewMockEnsurer(ctrl)
	s.statusHistory = &statusHistoryRecorder{}

	s.service = NewLeadershipService(
		s.state,
		s.leadership,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
		s.statusHistory,
	)

	return ctrl
}
