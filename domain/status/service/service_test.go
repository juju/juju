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
	corerelation "github.com/juju/juju/core/relation"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/status"
	statuserrors "github.com/juju/juju/domain/status/errors"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/statushistory"
)

type serviceSuite struct {
	state         *MockState
	leadership    *MockEnsurer
	statusHistory *statusHistoryRecorder

	service *LeadershipService
}

var _ = gc.Suite(&serviceSuite{})

// TestGetAllRelationStatuses verifies that GetAllRelationStatuses
// retrieves and returns the expected relation details without errors.
// Doesn't have logic, so the test doesn't need to be smart.
func (s *serviceSuite) TestGetAllRelationStatuses(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	relUUID := corerelationtesting.GenRelationUUID(c)
	stateRelationStatus := map[corerelation.UUID]status.StatusInfo[status.RelationStatusType]{
		relUUID: {
			Status: status.RelationStatusTypeBroken,
		},
	}
	s.state.EXPECT().GetAllRelationStatuses(gomock.Any()).Return(stateRelationStatus, nil)

	// Act
	details, err := s.service.GetAllRelationStatuses(context.Background())

	// Assert
	c.Assert(err, gc.IsNil)
	c.Assert(details, gc.DeepEquals, map[corerelation.UUID]corestatus.StatusInfo{
		relUUID: {
			Status: corestatus.Broken,
		},
	})
}

// TestGetAllRelationStatusesError verifies the behavior when GetAllRelationStatuses
// encounters an error from the state layer.
func (s *serviceSuite) TestGetAllRelationStatusesError(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	expectedError := errors.New("state error")
	s.state.EXPECT().GetAllRelationStatuses(gomock.Any()).Return(nil, expectedError)

	// Act
	_, err := s.service.GetAllRelationStatuses(context.Background())

	// Assert
	c.Assert(err, jc.ErrorIs, expectedError)
}

func (s *serviceSuite) TestSetRelationStatus(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := unittesting.GenNewName(c, "app/0")
	appName := "app-name"
	currentStatus := status.StatusInfo[status.RelationStatusType]{
		Status: status.RelationStatusTypeBroken,
	}
	s.state.EXPECT().GetRelationStatus(gomock.Any(), relationUUID).Return(currentStatus, nil)

	s.state.EXPECT().GetApplicationIDAndNameByUnitName(gomock.Any(), unitName).Return("", appName, nil)

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
	s.leadership.EXPECT().WithLeader(gomock.Any(), appName, unitName.String(), gomock.Any()).DoAndReturn(
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

func (s *serviceSuite) TestSetRelationStatusRelationNotFound(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := unittesting.GenNewName(c, "app/0")
	sts := corestatus.StatusInfo{
		Status: corestatus.Broken,
	}
	s.state.EXPECT().GetRelationStatus(gomock.Any(), relationUUID).Return(status.StatusInfo[status.RelationStatusType]{}, statuserrors.RelationNotFound)

	// Act
	err := s.service.SetRelationStatus(context.Background(), unitName, relationUUID, sts)

	// Assert
	c.Assert(err, jc.ErrorIs, statuserrors.RelationNotFound)
}

func (s *serviceSuite) TestSetRelationStatusUnitNotFound(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitName := unittesting.GenNewName(c, "app/0")
	appName := "app-name"
	sts := corestatus.StatusInfo{
		Status: corestatus.Broken,
	}
	currentStatus := status.StatusInfo[status.RelationStatusType]{
		Status: status.RelationStatusTypeBroken,
	}
	s.state.EXPECT().GetRelationStatus(gomock.Any(), relationUUID).Return(currentStatus, nil)
	s.state.EXPECT().GetApplicationIDAndNameByUnitName(gomock.Any(), unitName).Return("", appName, statuserrors.UnitNotFound)

	// Act
	err := s.service.SetRelationStatus(context.Background(), unitName, relationUUID, sts)

	// Assert
	c.Assert(err, jc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *serviceSuite) TestSetApplicationStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	applicationUUID := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "gitlab").Return(applicationUUID, nil)
	s.state.EXPECT().SetApplicationStatus(gomock.Any(), applicationUUID, status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "doink",
		Data:    []byte(`{"foo":"bar"}`),
		Since:   &now,
	})

	err := s.service.SetApplicationStatus(context.Background(), "gitlab", corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.statusHistory.records, jc.DeepEquals, []statusHistoryRecord{{
		ns: statushistory.Namespace{Name: "application", ID: applicationUUID.String()},
		s: corestatus.StatusInfo{
			Status:  corestatus.Active,
			Message: "doink",
			Data:    map[string]interface{}{"foo": "bar"},
			Since:   &now,
		},
	}})
}

func (s *serviceSuite) TestSetApplicationStatusNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "gitlab").Return("", statuserrors.ApplicationNotFound)

	err := s.service.SetApplicationStatus(context.Background(), "gitlab", corestatus.StatusInfo{
		Status: corestatus.Active,
	})
	c.Assert(err, jc.ErrorIs, statuserrors.ApplicationNotFound)
}

func (s *serviceSuite) TestSetApplicationStatusInvalidStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.SetApplicationStatus(context.Background(), "gitlab", corestatus.StatusInfo{
		Status: corestatus.Status("invalid"),
	})
	c.Assert(err, gc.ErrorMatches, `.*unknown workload status "invalid"`)

	err = s.service.SetApplicationStatus(context.Background(), "gitlab", corestatus.StatusInfo{
		Status: corestatus.Allocating,
	})
	c.Assert(err, gc.ErrorMatches, `.*unknown workload status "allocating"`)
}

func (s *serviceSuite) TestSetApplicationStatusForUnitLeader(c *gc.C) {
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

func (s *serviceSuite) TestSetApplicationStatusForUnitLeaderNotLeader(c *gc.C) {
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

func (s *serviceSuite) TestSetApplicationStatusForUnitLeaderInvalidUnitName(c *gc.C) {
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

func (s *serviceSuite) TestSetApplicationStatusForUnitLeaderNoUnitFound(c *gc.C) {
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

func (s *serviceSuite) TestGetApplicationDisplayStatusNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "gitlab").Return("", statuserrors.ApplicationNotFound)

	_, err := s.service.GetApplicationDisplayStatus(context.Background(), "gitlab")
	c.Assert(err, jc.ErrorIs, statuserrors.ApplicationNotFound)
}

func (s *serviceSuite) TestGetApplicationDisplayStatusApplicationStatusSet(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	applicationUUID := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)
	s.state.EXPECT().GetApplicationStatus(gomock.Any(), applicationUUID).Return(
		status.StatusInfo[status.WorkloadStatusType]{
			Status:  status.WorkloadStatusActive,
			Message: "doink",
			Data:    []byte(`{"foo":"bar"}`),
			Since:   &now,
		}, nil)

	obtained, err := s.service.GetApplicationDisplayStatus(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained, jc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
}

func (s *serviceSuite) TestGetApplicationDisplayStatusFallbackToUnitsNoUnits(c *gc.C) {
	defer s.setupMocks(c).Finish()

	applicationUUID := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)
	s.state.EXPECT().GetApplicationStatus(gomock.Any(), applicationUUID).Return(
		status.StatusInfo[status.WorkloadStatusType]{
			Status: status.WorkloadStatusUnset,
		}, nil)

	s.state.EXPECT().GetAllFullUnitStatusesForApplication(gomock.Any(), applicationUUID).Return(nil, nil)

	obtained, err := s.service.GetApplicationDisplayStatus(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained, jc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Unknown,
	})
}

func (s *serviceSuite) TestGetApplicationDisplayStatusFallbackToUnitsNoContainers(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	applicationUUID := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)
	s.state.EXPECT().GetApplicationStatus(gomock.Any(), applicationUUID).Return(
		status.StatusInfo[status.WorkloadStatusType]{
			Status: status.WorkloadStatusUnset,
		}, nil)

	s.state.EXPECT().GetAllFullUnitStatusesForApplication(gomock.Any(), applicationUUID).Return(
		status.FullUnitStatuses{
			"unit-1": {
				WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
					Status:  status.WorkloadStatusActive,
					Message: "doink",
					Data:    []byte(`{"foo":"bar"}`),
					Since:   &now,
				},
				AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusIdle,
				},
				Present: true,
			},
			"unit-2": {
				WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
					Status:  status.WorkloadStatusActive,
					Message: "doink",
					Data:    []byte(`{"foo":"bar"}`),
					Since:   &now,
				},
				AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusIdle,
				},
				Present: true,
			},
		},
		nil)

	obtained, err := s.service.GetApplicationDisplayStatus(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained, jc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
}

func (s *serviceSuite) TestGetApplicationAndUnitStatusesForUnitWithLeaderNotLeader(c *gc.C) {
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

func (s *serviceSuite) TestGetApplicationAndUnitStatusesForUnitWithLeaderInvalidUnitName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, _, err := s.service.GetApplicationAndUnitStatusesForUnitWithLeader(context.Background(), coreunit.Name("!!!"))
	c.Assert(err, jc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *serviceSuite) TestGetApplicationAndUnitStatusesForUnitWithLeaderNoApplication(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return("", statuserrors.ApplicationNotFound)
	_, _, err := s.service.GetApplicationAndUnitStatusesForUnitWithLeader(context.Background(), unitName)
	c.Assert(err, jc.ErrorIs, statuserrors.ApplicationNotFound)
}

func (s *serviceSuite) TestGetApplicationAndUnitStatusesForUnitWithLeaderApplicationStatusSet(c *gc.C) {
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

func (s *serviceSuite) TestGetApplicationAndUnitStatusesForUnitWithLeaderApplicationStatusUnset(c *gc.C) {
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
				ContainerStatus: status.StatusInfo[status.CloudContainerStatusType]{
					Status:  status.CloudContainerStatusBlocked,
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
				ContainerStatus: status.StatusInfo[status.CloudContainerStatusType]{
					Status:  status.CloudContainerStatusRunning,
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

func (s *serviceSuite) TestSetWorkloadUnitStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.state.EXPECT().SetUnitWorkloadStatus(gomock.Any(), unitUUID, status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "doink",
		Data:    []byte(`{"foo":"bar"}`),
		Since:   &now,
	})

	err := s.service.SetUnitWorkloadStatus(context.Background(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.statusHistory.records, jc.DeepEquals, []statusHistoryRecord{{
		ns: statushistory.Namespace{Name: "unit-workload", ID: "foo/666"},
		s: corestatus.StatusInfo{
			Status:  corestatus.Active,
			Message: "doink",
			Data:    map[string]interface{}{"foo": "bar"},
			Since:   &now,
		},
	}})
}

func (s *serviceSuite) TestSetWorkloadUnitStatusInvalidStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.SetUnitWorkloadStatus(context.Background(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status: corestatus.Status("invalid"),
	})
	c.Assert(err, gc.ErrorMatches, `.*unknown workload status "invalid"`)

	err = s.service.SetUnitWorkloadStatus(context.Background(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status: corestatus.Allocating,
	})
	c.Assert(err, gc.ErrorMatches, `.*unknown workload status "allocating"`)
}

func (s *serviceSuite) TestGetUnitWorkloadStatusesForApplication(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	appUUID := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetUnitWorkloadStatusesForApplication(gomock.Any(), appUUID).Return(
		map[coreunit.Name]status.UnitStatusInfo[status.WorkloadStatusType]{
			"unit-1": {
				StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
					Status:  status.WorkloadStatusActive,
					Message: "doink",
					Data:    []byte(`{"foo":"bar"}`),
					Since:   &now,
				},
				Present: true,
			},
			"unit-2": {
				StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
					Status:  status.WorkloadStatusMaintenance,
					Message: "boink",
					Data:    []byte(`{"foo":"baz"}`),
					Since:   &now,
				},
				Present: true,
			},
		}, nil,
	)

	obtained, err := s.service.GetUnitWorkloadStatusesForApplication(context.Background(), appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained, jc.DeepEquals, map[coreunit.Name]corestatus.StatusInfo{
		"unit-1": {
			Status:  corestatus.Active,
			Message: "doink",
			Data:    map[string]interface{}{"foo": "bar"},
			Since:   &now,
		},
		"unit-2": {
			Status:  corestatus.Maintenance,
			Message: "boink",
			Data:    map[string]interface{}{"foo": "baz"},
			Since:   &now,
		},
	})
}

func (s *serviceSuite) TestGetUnitDisplayAndAgentStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.state.EXPECT().GetUnitAgentStatus(gomock.Any(), unitUUID).Return(
		status.UnitStatusInfo[status.UnitAgentStatusType]{
			StatusInfo: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusAllocating,
				Message: "doink",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &now,
			},
			Present: true,
		}, nil)
	s.state.EXPECT().GetUnitWorkloadStatus(gomock.Any(), unitUUID).Return(
		status.UnitStatusInfo[status.WorkloadStatusType]{
			StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
				Message: "doink",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &now,
			},
			Present: true,
		}, nil)

	s.state.EXPECT().GetUnitCloudContainerStatus(gomock.Any(), unitUUID).Return(status.StatusInfo[status.CloudContainerStatusType]{
		Status: status.CloudContainerStatusUnset,
	}, nil)

	agent, workload, err := s.service.GetUnitDisplayAndAgentStatus(context.Background(), coreunit.Name("foo/666"))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(agent, jc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Allocating,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Check(workload, jc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
}

func (s *serviceSuite) TestGetUnitDisplayAndAgentStatusWithAllocatingPresence(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.state.EXPECT().GetUnitAgentStatus(gomock.Any(), unitUUID).Return(
		status.UnitStatusInfo[status.UnitAgentStatusType]{
			StatusInfo: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusAllocating,
				Message: "doink",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &now,
			},
			Present: false,
		}, nil)
	s.state.EXPECT().GetUnitWorkloadStatus(gomock.Any(), unitUUID).Return(
		status.UnitStatusInfo[status.WorkloadStatusType]{
			StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
				Message: "doink",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &now,
			},
			Present: true,
		}, nil)

	s.state.EXPECT().GetUnitCloudContainerStatus(gomock.Any(), unitUUID).Return(status.StatusInfo[status.CloudContainerStatusType]{
		Status: status.CloudContainerStatusUnset,
	}, nil)

	agent, workload, err := s.service.GetUnitDisplayAndAgentStatus(context.Background(), coreunit.Name("foo/666"))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(agent, jc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Allocating,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Check(workload, jc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
}

func (s *serviceSuite) TestGetUnitDisplayAndAgentStatusWithNoPresence(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.state.EXPECT().GetUnitAgentStatus(gomock.Any(), unitUUID).Return(
		status.UnitStatusInfo[status.UnitAgentStatusType]{
			StatusInfo: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusIdle,
				Message: "doink",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &now,
			},
			Present: false,
		}, nil)
	s.state.EXPECT().GetUnitWorkloadStatus(gomock.Any(), unitUUID).Return(
		status.UnitStatusInfo[status.WorkloadStatusType]{
			StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
				Message: "doink",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &now,
			},
			Present: false,
		}, nil)

	s.state.EXPECT().GetUnitCloudContainerStatus(gomock.Any(), unitUUID).Return(status.StatusInfo[status.CloudContainerStatusType]{
		Status: status.CloudContainerStatusUnset,
	}, nil)

	agent, workload, err := s.service.GetUnitDisplayAndAgentStatus(context.Background(), coreunit.Name("foo/666"))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(agent, jc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Lost,
		Message: "agent is not communicating with the server",
		Since:   &now,
	})
	c.Check(workload, jc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Unknown,
		Message: "agent lost, see `juju debug-logs` or `juju show-status-log` for more information",
		Since:   &now,
	})
}

func (s *serviceSuite) TestGetUnitWorkloadStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.state.EXPECT().GetUnitWorkloadStatus(gomock.Any(), unitUUID).Return(
		status.UnitStatusInfo[status.WorkloadStatusType]{
			StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
				Message: "doink",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &now,
			},
			Present: true,
		}, nil)

	obtained, err := s.service.GetUnitWorkloadStatus(context.Background(), coreunit.Name("foo/666"))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(obtained, jc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
}

func (s *serviceSuite) TestGetUnitWorkloadStatusUnitInvalidName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.GetUnitWorkloadStatus(context.Background(), coreunit.Name("!!!"))
	c.Assert(err, jc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *serviceSuite) TestGetUnitWorkloadStatusUnitNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, statuserrors.UnitNotFound)

	_, err := s.service.GetUnitWorkloadStatus(context.Background(), coreunit.Name("foo/666"))
	c.Assert(err, jc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *serviceSuite) TestGetUnitWorkloadStatusUnitInvalidWorkloadStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.state.EXPECT().GetUnitWorkloadStatus(gomock.Any(), unitUUID).Return(status.UnitStatusInfo[status.WorkloadStatusType]{}, errors.Errorf("boom"))

	_, err := s.service.GetUnitWorkloadStatus(context.Background(), coreunit.Name("foo/666"))
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *serviceSuite) TestSetUnitWorkloadStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	now := time.Now()

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.state.EXPECT().SetUnitWorkloadStatus(gomock.Any(), unitUUID, status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "doink",
		Data:    []byte(`{"foo":"bar"}`),
		Since:   &now,
	})

	err := s.service.SetUnitWorkloadStatus(context.Background(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestSetUnitWorkloadStatusInvalidName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	err := s.service.SetUnitWorkloadStatus(context.Background(), coreunit.Name("!!!"), corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *serviceSuite) TestSetUnitWorkloadStatusUnitFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	now := time.Now()

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, statuserrors.UnitNotFound)

	err := s.service.SetUnitWorkloadStatus(context.Background(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *serviceSuite) TestSetUnitWorkloadStatusInvalidStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	now := time.Now()

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.state.EXPECT().SetUnitWorkloadStatus(gomock.Any(), unitUUID, status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "doink",
		Data:    []byte(`{"foo":"bar"}`),
		Since:   &now,
	}).Return(errors.New("boom"))

	err := s.service.SetUnitWorkloadStatus(context.Background(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, gc.ErrorMatches, ".*boom")
}

func (s *serviceSuite) TestSetUnitAgentStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	now := time.Now()

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.state.EXPECT().SetUnitAgentStatus(gomock.Any(), unitUUID, status.StatusInfo[status.UnitAgentStatusType]{
		Status:  status.UnitAgentStatusIdle,
		Message: "doink",
		Data:    []byte(`{"foo":"bar"}`),
		Since:   &now,
	})

	err := s.service.SetUnitAgentStatus(context.Background(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status:  corestatus.Idle,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestSetUnitAgentStatusErrorWithNoMessage(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.SetUnitAgentStatus(context.Background(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status:  corestatus.Error,
		Message: "",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, gc.ErrorMatches, `setting status "error" without message`)
}

func (s *serviceSuite) TestSetUnitAgentStatusLost(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.SetUnitAgentStatus(context.Background(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status:  corestatus.Lost,
		Message: "are you lost?",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, gc.ErrorMatches, `setting status "lost" is not allowed`)
}

func (s *serviceSuite) TestSetUnitAgentStatusAllocating(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.SetUnitAgentStatus(context.Background(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status:  corestatus.Allocating,
		Message: "help me help you",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, gc.ErrorMatches, `setting status "allocating" is not allowed`)
}

func (s *serviceSuite) TestSetUnitAgentStatusInvalidName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	err := s.service.SetUnitAgentStatus(context.Background(), coreunit.Name("!!!"), corestatus.StatusInfo{
		Status:  corestatus.Idle,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *serviceSuite) TestSetUnitAgentStatusUnitFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	now := time.Now()

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, statuserrors.UnitNotFound)

	err := s.service.SetUnitAgentStatus(context.Background(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status:  corestatus.Idle,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *serviceSuite) TestSetUnitAgentStatusInvalidStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	now := time.Now()

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.state.EXPECT().SetUnitAgentStatus(gomock.Any(), unitUUID, status.StatusInfo[status.UnitAgentStatusType]{
		Status:  status.UnitAgentStatusIdle,
		Message: "doink",
		Data:    []byte(`{"foo":"bar"}`),
		Since:   &now,
	}).Return(errors.New("boom"))

	err := s.service.SetUnitAgentStatus(context.Background(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status:  corestatus.Idle,
		Message: "doink",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, gc.ErrorMatches, ".*boom")
}

func (s *serviceSuite) TestSetUnitPresence(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetUnitPresence(gomock.Any(), coreunit.Name("foo/666"))

	err := s.service.SetUnitPresence(context.Background(), coreunit.Name("foo/666"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestSetUnitPresenceInvalidName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.SetUnitPresence(context.Background(), coreunit.Name("!!!"))
	c.Assert(err, jc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *serviceSuite) TestDeleteUnitPresence(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().DeleteUnitPresence(gomock.Any(), coreunit.Name("foo/666"))

	err := s.service.DeleteUnitPresence(context.Background(), coreunit.Name("foo/666"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestDeleteUnitPresenceInvalidName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.DeleteUnitPresence(context.Background(), coreunit.Name("!!!"))
	c.Assert(err, jc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *serviceSuite) TestCheckUnitStatusesReadyForMigrationEmptyModel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAllUnitWorkloadAgentStatuses(gomock.Any()).Return(status.UnitWorkloadAgentStatuses{}, nil)

	err := s.service.CheckUnitStatusesReadyForMigration(context.Background())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCheckUnitStatusesReadyForMigration(c *gc.C) {
	defer s.setupMocks(c).Finish()

	fullStatus := status.UnitWorkloadAgentStatuses{
		"foo/650": {
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusIdle,
				Message: "boink",
			},
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
				Message: "doink",
			},
			Present: true,
		},
		"foo/667": {
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusExecuting,
				Message: "boink",
			},
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusMaintenance,
				Message: "doink",
			},
			Present: true,
		},
		"foo/668": {
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusExecuting,
				Message: "boink",
			},
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusWaiting,
				Message: "doink",
			},
			Present: true,
		},
	}
	s.state.EXPECT().GetAllUnitWorkloadAgentStatuses(gomock.Any()).Return(fullStatus, nil)

	err := s.service.CheckUnitStatusesReadyForMigration(context.Background())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCheckUnitStatusesReadyForMigrationNotReadyPresence(c *gc.C) {
	defer s.setupMocks(c).Finish()

	fullStatus := status.UnitWorkloadAgentStatuses{
		"foo/650": {
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusIdle,
				Message: "boink",
			},
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
				Message: "doink",
			},
			Present: true,
		},
		"foo/667": {
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusIdle,
				Message: "boink",
			},
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
				Message: "doink",
			},
		},
		"foo/668": {
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusIdle,
				Message: "boink",
			},
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
				Message: "doink",
			},
		},
	}
	s.state.EXPECT().GetAllUnitWorkloadAgentStatuses(gomock.Any()).Return(fullStatus, nil)

	err := s.service.CheckUnitStatusesReadyForMigration(context.Background())
	c.Assert(err, gc.ErrorMatches, `(?m).*
- unit "foo/66\d" is not logged into the controller
- unit "foo/66\d" is not logged into the controller`)
}

func (s *serviceSuite) TestCheckUnitStatusesReadyForMigrationNotReadyAgentStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	fullStatus := status.UnitWorkloadAgentStatuses{
		"foo/650": {
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusIdle,
				Message: "boink",
			},
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
				Message: "doink",
			},
			Present: true,
		},
		"foo/667": {
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusAllocating,
				Message: "boink",
			},
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
				Message: "doink",
			},
			Present: true,
		},
		"foo/668": {
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusRebooting,
				Message: "boink",
			},
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
				Message: "doink",
			},
			Present: true,
		},
	}
	s.state.EXPECT().GetAllUnitWorkloadAgentStatuses(gomock.Any()).Return(fullStatus, nil)

	err := s.service.CheckUnitStatusesReadyForMigration(context.Background())
	c.Assert(err, gc.ErrorMatches, `(?m).*
- unit "foo/66\d" agent not idle or executing
- unit "foo/66\d" agent not idle or executing`)
}

func (s *serviceSuite) TestCheckUnitStatusesReadyForMigrationNotReadyWorkload(c *gc.C) {
	defer s.setupMocks(c).Finish()

	fullStatus := status.UnitWorkloadAgentStatuses{
		"foo/650": {
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusIdle,
				Message: "boink",
			},
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
				Message: "doink",
			},
			Present: true,
		},
		"foo/667": {
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusIdle,
				Message: "boink",
			},
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusBlocked,
				Message: "doink",
			},
			Present: true,
		},
		"foo/668": {
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusIdle,
				Message: "boink",
			},
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusError,
				Message: "doink",
			},
			Present: true,
		},
	}
	s.state.EXPECT().GetAllUnitWorkloadAgentStatuses(gomock.Any()).Return(fullStatus, nil)

	err := s.service.CheckUnitStatusesReadyForMigration(context.Background())
	c.Assert(err, gc.ErrorMatches, `(?m).*
- unit "foo/66\d" workload not active or viable
- unit "foo/66\d" workload not active or viable`)
}

func (s *serviceSuite) TestCheckUnitStatusesReadyForMigrationNotReadyWorkloadMessage(c *gc.C) {
	defer s.setupMocks(c).Finish()

	fullStatus := status.UnitWorkloadAgentStatuses{
		"foo/650": {
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusIdle,
				Message: "boink",
			},
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusWaiting,
				Message: "doink",
			},
			Present: true,
		},
		"foo/651": {
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusIdle,
				Message: "boink",
			},
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusMaintenance,
				Message: "doink",
			},
			Present: true,
		},
		"foo/666": {
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusIdle,
				Message: "boink",
			},
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusWaiting,
				Message: corestatus.MessageInstallingAgent,
			},
			Present: true,
		},
		"foo/667": {
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusIdle,
				Message: "boink",
			},
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusMaintenance,
				Message: corestatus.MessageInstallingCharm,
			},
			Present: true,
		},
	}
	s.state.EXPECT().GetAllUnitWorkloadAgentStatuses(gomock.Any()).Return(fullStatus, nil)

	err := s.service.CheckUnitStatusesReadyForMigration(context.Background())
	c.Assert(err, gc.ErrorMatches, `(?m).*
- unit "foo/66\d" workload not active or viable
- unit "foo/66\d" workload not active or viable`)
}

func (s *serviceSuite) TestExportUnitStatusesEmpty(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAllUnitWorkloadAgentStatuses(gomock.Any()).Return(status.UnitWorkloadAgentStatuses{}, nil)

	workloadStatuses, agentStatuses, err := s.service.ExportUnitStatuses(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(workloadStatuses, gc.HasLen, 0)
	c.Check(agentStatuses, gc.HasLen, 0)
}

func (s *serviceSuite) TestExportUnitStatuses(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	fullStatus := status.UnitWorkloadAgentStatuses{
		"foo/66": {
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusIdle,
				Message: "it's idle",
				Data:    []byte(`{"bar":"foo"}`),
				Since:   &now,
			},
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
				Message: "it's active",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &now,
			},
			Present: true,
		},
		"foo/67": {
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusAllocating,
				Message: "it's allocating",
				Data:    []byte(`{"foo":"baz"}`),
				Since:   &now,
			},
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusBlocked,
				Message: "it's blocked",
				Data:    []byte(`{"baz":"foo"}`),
				Since:   &now,
			},
		},
	}
	s.state.EXPECT().GetAllUnitWorkloadAgentStatuses(gomock.Any()).Return(fullStatus, nil)

	workloadStatuses, agentStatuses, err := s.service.ExportUnitStatuses(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(workloadStatuses, gc.DeepEquals, map[coreunit.Name]corestatus.StatusInfo{
		"foo/66": {
			Status:  corestatus.Active,
			Message: "it's active",
			Data:    map[string]interface{}{"foo": "bar"},
			Since:   &now,
		},
		"foo/67": {
			Status:  corestatus.Blocked,
			Message: "it's blocked",
			Data:    map[string]interface{}{"baz": "foo"},
			Since:   &now,
		},
	})
	c.Check(agentStatuses, gc.DeepEquals, map[coreunit.Name]corestatus.StatusInfo{
		"foo/66": {
			Status:  corestatus.Idle,
			Message: "it's idle",
			Data:    map[string]interface{}{"bar": "foo"},
			Since:   &now,
		},
		"foo/67": {
			Status:  corestatus.Allocating,
			Message: "it's allocating",
			Data:    map[string]interface{}{"foo": "baz"},
			Since:   &now,
		},
	})
}

func (s *serviceSuite) TestExportApplicationStatusesEmpty(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAllApplicationStatuses(gomock.Any()).Return(map[string]status.StatusInfo[status.WorkloadStatusType]{}, nil)

	statuses, err := s.service.ExportApplicationStatuses(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(statuses, gc.HasLen, 0)
}

func (s *serviceSuite) TestExportApplicationStatuses(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	statuses := map[string]status.StatusInfo[status.WorkloadStatusType]{
		"foo": {
			Status:  status.WorkloadStatusActive,
			Message: "it's active",
			Data:    []byte(`{"foo": "bar"}`),
			Since:   &now,
		},
		"bar": {
			Status:  status.WorkloadStatusBlocked,
			Message: "it's blocked",
			Data:    []byte(`{"bar": "foo"}`),
			Since:   &now,
		},
	}
	s.state.EXPECT().GetAllApplicationStatuses(gomock.Any()).Return(statuses, nil)

	exported, err := s.service.ExportApplicationStatuses(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(exported, gc.DeepEquals, map[string]corestatus.StatusInfo{
		"foo": {
			Status:  corestatus.Active,
			Message: "it's active",
			Data:    map[string]interface{}{"foo": "bar"},
			Since:   &now,
		},
		"bar": {
			Status:  corestatus.Blocked,
			Message: "it's blocked",
			Data:    map[string]interface{}{"bar": "foo"},
			Since:   &now,
		},
	})
}

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
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

type statusHistoryRecord struct {
	ns statushistory.Namespace
	s  corestatus.StatusInfo
}

type statusHistoryRecorder struct {
	records []statusHistoryRecord
}

// RecordStatus records the given status information.
// If the status data cannot be marshalled, it will not be recorded, instead
// the error will be logged under the data_error key.
func (r *statusHistoryRecorder) RecordStatus(ctx context.Context, ns statushistory.Namespace, s corestatus.StatusInfo) error {
	r.records = append(r.records, statusHistoryRecord{ns: ns, s: s})
	return nil
}
