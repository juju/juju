// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/model"
	corerelation "github.com/juju/juju/core/relation"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/status"
	statuserrors "github.com/juju/juju/domain/status/errors"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type leaderServiceSuite struct {
	modelState      *MockModelState
	controllerState *MockControllerState

	leadership    *MockEnsurer
	statusHistory *statusHistoryRecorder

	service *LeadershipService
}

func TestLeaderServiceSuite(t *testing.T) {
	tc.Run(t, &leaderServiceSuite{})
}

func (s *leaderServiceSuite) TestSetRelationStatus(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	relationUUID := corerelation.GenRelationUUID(c)
	unitName := coreunit.GenName(c, "app/0")

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
	s.modelState.EXPECT().SetRelationStatus(gomock.Any(), relationUUID, expectedStatus).Return(nil)

	// Act
	err := s.service.SetRelationStatus(c.Context(), unitName, relationUUID, sts)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *leaderServiceSuite) TestSetRelationStatusRelationNotFound(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	relationUUID := corerelation.GenRelationUUID(c)
	unitName := coreunit.GenName(c, "app/0")
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
	s.modelState.EXPECT().SetRelationStatus(gomock.Any(), relationUUID, expectedStatus).Return(statuserrors.RelationNotFound)

	// Act
	err := s.service.SetRelationStatus(c.Context(), unitName, relationUUID, sts)

	// Assert
	c.Assert(err, tc.ErrorIs, statuserrors.RelationNotFound)
}

func (s *leaderServiceSuite) TestSetApplicationStatusForUnitLeader(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	applicationUUID := coreapplication.GenID(c)
	unitName := coreunit.Name("foo/666")

	s.leadership.EXPECT().WithLeader(gomock.Any(), "foo", unitName.String(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _, _ string, fn func(context.Context) error) error {
			return fn(ctx)
		})

	s.modelState.EXPECT().GetApplicationIDAndNameByUnitName(gomock.Any(), unitName).Return(applicationUUID, "foo", nil)
	s.modelState.EXPECT().SetApplicationStatus(gomock.Any(), applicationUUID, status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "doink",
		Data:    []byte(`{"foo":"bar"}`),
		Since:   &now,
	})

	err := s.service.SetApplicationStatusForUnitLeader(c.Context(), unitName, corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *leaderServiceSuite) TestSetApplicationStatusForUnitLeaderNotLeader(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	applicationUUID := coreapplication.GenID(c)
	unitName := coreunit.Name("foo/666")

	s.leadership.EXPECT().WithLeader(gomock.Any(), "foo", unitName.String(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _, _ string, fn func(context.Context) error) error {
			return lease.ErrNotHeld
		})

	s.modelState.EXPECT().GetApplicationIDAndNameByUnitName(gomock.Any(), unitName).Return(applicationUUID, "foo", nil)

	err := s.service.SetApplicationStatusForUnitLeader(c.Context(), unitName, corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorIs, statuserrors.UnitNotLeader)
}

func (s *leaderServiceSuite) TestSetApplicationStatusForUnitLeaderInvalidUnitName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	unitName := coreunit.Name("!!!!")
	err := s.service.SetApplicationStatusForUnitLeader(c.Context(), unitName, corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *leaderServiceSuite) TestSetApplicationStatusForUnitLeaderNoUnitFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	applicationUUID := coreapplication.GenID(c)
	unitName := coreunit.Name("foo/666")

	s.modelState.EXPECT().GetApplicationIDAndNameByUnitName(gomock.Any(), unitName).
		Return(applicationUUID, "foo", statuserrors.UnitNotFound)

	err := s.service.SetApplicationStatusForUnitLeader(c.Context(), unitName, corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *leaderServiceSuite) TestGetApplicationAndUnitStatusesForUnitWithLeaderNotLeader(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	applicationUUID := coreapplication.GenID(c)

	s.modelState.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)

	s.leadership.EXPECT().WithLeader(gomock.Any(), "foo", unitName.String(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _, _ string, fn func(context.Context) error) error {
			return lease.ErrNotHeld
		})

	_, _, err := s.service.GetApplicationAndUnitStatusesForUnitWithLeader(c.Context(), unitName)
	c.Assert(err, tc.ErrorIs, statuserrors.UnitNotLeader)
}

func (s *leaderServiceSuite) TestGetApplicationAndUnitStatusesForUnitWithLeaderInvalidUnitName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, _, err := s.service.GetApplicationAndUnitStatusesForUnitWithLeader(c.Context(), coreunit.Name("!!!"))
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *leaderServiceSuite) TestGetApplicationAndUnitStatusesForUnitWithLeaderNoApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")

	s.modelState.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return("", statuserrors.ApplicationNotFound)
	_, _, err := s.service.GetApplicationAndUnitStatusesForUnitWithLeader(c.Context(), unitName)
	c.Assert(err, tc.ErrorIs, statuserrors.ApplicationNotFound)
}

func (s *leaderServiceSuite) TestGetApplicationAndUnitStatusesForUnitWithLeaderApplicationStatusSet(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	now := time.Now()
	applicationUUID := coreapplication.GenID(c)

	s.leadership.EXPECT().WithLeader(gomock.Any(), "foo", unitName.String(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _, _ string, fn func(context.Context) error) error {
			return fn(ctx)
		})

	s.modelState.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)

	s.modelState.EXPECT().GetApplicationStatus(gomock.Any(), applicationUUID).Return(
		status.StatusInfo[status.WorkloadStatusType]{
			Status:  status.WorkloadStatusActive,
			Message: "doink",
			Data:    []byte(`{"foo":"bar"}`),
			Since:   &now,
		}, nil)

	s.modelState.EXPECT().GetAllFullUnitStatusesForApplication(gomock.Any(), applicationUUID).Return(
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

	applicationStatus, unitWorkloadStatuses, err := s.service.GetApplicationAndUnitStatusesForUnitWithLeader(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(applicationStatus, tc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Check(unitWorkloadStatuses, tc.DeepEquals, map[coreunit.Name]corestatus.StatusInfo{
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

func (s *leaderServiceSuite) TestGetApplicationAndUnitStatusesForUnitWithLeaderApplicationStatusUnset(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	now := time.Now()
	applicationUUID := coreapplication.GenID(c)

	s.leadership.EXPECT().WithLeader(gomock.Any(), "foo", unitName.String(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _, _ string, fn func(context.Context) error) error {
			return fn(ctx)
		})

	s.modelState.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)

	s.modelState.EXPECT().GetApplicationStatus(gomock.Any(), applicationUUID).Return(
		status.StatusInfo[status.WorkloadStatusType]{
			Status: status.WorkloadStatusUnset,
		}, nil)

	s.modelState.EXPECT().GetAllFullUnitStatusesForApplication(gomock.Any(), applicationUUID).Return(
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

	applicationStatus, unitWorkloadStatuses, err := s.service.GetApplicationAndUnitStatusesForUnitWithLeader(c.Context(), unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(applicationStatus, tc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Blocked,
		Message: "zoink",
		Data:    map[string]interface{}{"foo": "baz"},
		Since:   &now,
	})
	c.Check(unitWorkloadStatuses, tc.DeepEquals, map[coreunit.Name]corestatus.StatusInfo{
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

func (s *leaderServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelState = NewMockModelState(ctrl)
	s.controllerState = NewMockControllerState(ctrl)
	s.leadership = NewMockEnsurer(ctrl)
	s.statusHistory = &statusHistoryRecorder{}

	s.service = NewLeadershipService(
		s.modelState,
		s.controllerState,
		s.leadership,
		model.UUID("test-model"),
		s.statusHistory,
		func() (StatusHistoryReader, error) {
			return nil, errors.Errorf("status history reader not available")
		},
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	return ctrl
}
