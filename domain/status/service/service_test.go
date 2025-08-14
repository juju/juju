// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	corerelation "github.com/juju/juju/core/relation"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/status"
	statuserrors "github.com/juju/juju/domain/status/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/statushistory"
)

type serviceSuite struct {
	controllerState *MockControllerState
	modelState      *MockModelState
	statusHistory   *statusHistoryRecorder

	modelService *Service
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

// TestGetAllRelationStatuses verifies that GetAllRelationStatuses
// retrieves and returns the expected relation details without errors.
// Doesn't have logic, so the test doesn't need to be smart.
func (s *serviceSuite) TestGetAllRelationStatuses(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	relUUID := corerelation.GenRelationUUID(c)
	stateRelationStatus := []status.RelationStatusInfo{{
		RelationUUID: relUUID,
		StatusInfo: status.StatusInfo[status.RelationStatusType]{
			Status: status.RelationStatusTypeBroken,
		}},
	}
	s.modelState.EXPECT().GetAllRelationStatuses(gomock.Any()).Return(stateRelationStatus, nil)

	// Act
	details, err := s.modelService.GetAllRelationStatuses(c.Context())

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(details, tc.DeepEquals, map[corerelation.UUID]corestatus.StatusInfo{
		relUUID: {
			Status: corestatus.Broken,
		},
	})
}

// TestGetAllRelationStatusesError verifies the behavior when GetAllRelationStatuses
// encounters an error from the state layer.
func (s *serviceSuite) TestGetAllRelationStatusesError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	expectedError := errors.New("state error")
	s.modelState.EXPECT().GetAllRelationStatuses(gomock.Any()).Return(nil, expectedError)

	// Act
	_, err := s.modelService.GetAllRelationStatuses(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIs, expectedError)
}

func (s *serviceSuite) TestImportRelationStatus(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	relationID := 1
	relationUUID := corerelation.GenRelationUUID(c)
	s.modelState.EXPECT().GetRelationUUIDByID(gomock.Any(), relationID).Return(relationUUID, nil)
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
	s.modelState.EXPECT().ImportRelationStatus(gomock.Any(), relationUUID, expectedStatus).Return(nil)

	// Act
	err := s.modelService.ImportRelationStatus(c.Context(), relationID, sts)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestImportRelationServiceError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	relationID := 1
	relationUUID := corerelation.GenRelationUUID(c)
	s.modelState.EXPECT().GetRelationUUIDByID(gomock.Any(), relationID).Return(relationUUID, nil)
	boom := errors.New("boom")
	sts := corestatus.StatusInfo{
		Status: corestatus.Broken,
	}
	expectedStatus := status.StatusInfo[status.RelationStatusType]{
		Status: status.RelationStatusTypeBroken,
	}
	s.modelState.EXPECT().ImportRelationStatus(gomock.Any(), relationUUID, expectedStatus).Return(boom)

	// Act
	err := s.modelService.ImportRelationStatus(c.Context(), relationID, sts)

	// Assert
	c.Assert(err, tc.ErrorIs, boom)
}

func (s *serviceSuite) TestSetApplicationStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	applicationUUID := coreapplication.GenID(c)
	s.modelState.EXPECT().GetApplicationIDByName(gomock.Any(), "gitlab").Return(applicationUUID, nil)
	s.modelState.EXPECT().SetApplicationStatus(gomock.Any(), applicationUUID, status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "doink",
		Data:    []byte(`{"foo":"bar"}`),
		Since:   &now,
	})

	err := s.modelService.SetApplicationStatus(c.Context(), "gitlab", corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.statusHistory.records, tc.DeepEquals, []statusHistoryRecord{{
		ns: statushistory.Namespace{Kind: corestatus.KindApplication, ID: applicationUUID.String()},
		s: corestatus.StatusInfo{
			Status:  corestatus.Active,
			Message: "doink",
			Data:    map[string]any{"foo": "bar"},
			Since:   &now,
		},
	}})
}

func (s *serviceSuite) TestSetApplicationStatusNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetApplicationIDByName(gomock.Any(), "gitlab").Return("", statuserrors.ApplicationNotFound)

	err := s.modelService.SetApplicationStatus(c.Context(), "gitlab", corestatus.StatusInfo{
		Status: corestatus.Active,
	})
	c.Assert(err, tc.ErrorIs, statuserrors.ApplicationNotFound)
}

func (s *serviceSuite) TestSetApplicationStatusInvalidStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.modelService.SetApplicationStatus(c.Context(), "gitlab", corestatus.StatusInfo{
		Status: corestatus.Status("invalid"),
	})
	c.Assert(err, tc.ErrorMatches, `.*unknown workload status "invalid"`)

	err = s.modelService.SetApplicationStatus(c.Context(), "gitlab", corestatus.StatusInfo{
		Status: corestatus.Allocating,
	})
	c.Assert(err, tc.ErrorMatches, `.*unknown workload status "allocating"`)
}

func (s *serviceSuite) TestGetApplicationDisplayStatusNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetApplicationIDByName(gomock.Any(), "gitlab").Return("", statuserrors.ApplicationNotFound)

	_, err := s.modelService.GetApplicationDisplayStatus(c.Context(), "gitlab")
	c.Assert(err, tc.ErrorIs, statuserrors.ApplicationNotFound)
}

func (s *serviceSuite) TestGetApplicationDisplayStatusApplicationStatusSet(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	applicationUUID := coreapplication.GenID(c)
	s.modelState.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)
	s.modelState.EXPECT().GetApplicationStatus(gomock.Any(), applicationUUID).Return(
		status.StatusInfo[status.WorkloadStatusType]{
			Status:  status.WorkloadStatusActive,
			Message: "doink",
			Data:    []byte(`{"foo":"bar"}`),
			Since:   &now,
		}, nil)

	obtained, err := s.modelService.GetApplicationDisplayStatus(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained, tc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
}

func (s *serviceSuite) TestGetApplicationDisplayStatusFallbackToUnitsNoUnits(c *tc.C) {
	defer s.setupMocks(c).Finish()

	applicationUUID := coreapplication.GenID(c)
	s.modelState.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)
	s.modelState.EXPECT().GetApplicationStatus(gomock.Any(), applicationUUID).Return(
		status.StatusInfo[status.WorkloadStatusType]{
			Status: status.WorkloadStatusUnset,
		}, nil)

	s.modelState.EXPECT().GetAllFullUnitStatusesForApplication(gomock.Any(), applicationUUID).Return(nil, nil)

	obtained, err := s.modelService.GetApplicationDisplayStatus(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained, tc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Unknown,
	})
}

func (s *serviceSuite) TestGetApplicationDisplayStatusFallbackToUnitsNoContainers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	applicationUUID := coreapplication.GenID(c)
	s.modelState.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)
	s.modelState.EXPECT().GetApplicationStatus(gomock.Any(), applicationUUID).Return(
		status.StatusInfo[status.WorkloadStatusType]{
			Status: status.WorkloadStatusUnset,
		}, nil)

	s.modelState.EXPECT().GetAllFullUnitStatusesForApplication(gomock.Any(), applicationUUID).Return(
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

	obtained, err := s.modelService.GetApplicationDisplayStatus(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained, tc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
}

func (s *serviceSuite) TestSetWorkloadUnitStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	unitUUID := coreunit.GenUUID(c)
	s.modelState.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.modelState.EXPECT().SetUnitWorkloadStatus(gomock.Any(), unitUUID, status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "doink",
		Data:    []byte(`{"foo":"bar"}`),
		Since:   &now,
	})

	err := s.modelService.SetUnitWorkloadStatus(c.Context(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.statusHistory.records, tc.DeepEquals, []statusHistoryRecord{{
		ns: statushistory.Namespace{Kind: corestatus.KindWorkload, ID: "foo/666"},
		s: corestatus.StatusInfo{
			Status:  corestatus.Active,
			Message: "doink",
			Data:    map[string]any{"foo": "bar"},
			Since:   &now,
		},
	}})
}

func (s *serviceSuite) TestSetWorkloadUnitStatusInvalidStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.modelService.SetUnitWorkloadStatus(c.Context(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status: corestatus.Status("invalid"),
	})
	c.Assert(err, tc.ErrorMatches, `.*unknown workload status "invalid"`)

	err = s.modelService.SetUnitWorkloadStatus(c.Context(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status: corestatus.Allocating,
	})
	c.Assert(err, tc.ErrorMatches, `.*unknown workload status "allocating"`)
}

func (s *serviceSuite) TestGetUnitWorkloadStatusesForApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	appUUID := coreapplication.GenID(c)
	s.modelState.EXPECT().GetUnitWorkloadStatusesForApplication(gomock.Any(), appUUID).Return(
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

	obtained, err := s.modelService.GetUnitWorkloadStatusesForApplication(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained, tc.DeepEquals, map[coreunit.Name]corestatus.StatusInfo{
		"unit-1": {
			Status:  corestatus.Active,
			Message: "doink",
			Data:    map[string]any{"foo": "bar"},
			Since:   &now,
		},
		"unit-2": {
			Status:  corestatus.Maintenance,
			Message: "boink",
			Data:    map[string]any{"foo": "baz"},
			Since:   &now,
		},
	})
}

func (s *serviceSuite) TestGetUnitAgentStatusesForApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	appUUID := coreapplication.GenID(c)
	s.modelState.EXPECT().GetUnitAgentStatusesForApplication(gomock.Any(), appUUID).Return(
		status.UnitAgentStatuses{
			"unit-1": status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusAllocating,
				Message: "doink",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &now,
			},
			"unit-2": status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusError,
				Message: "boink",
				Data:    []byte(`{"foo":"baz"}`),
				Since:   &now,
			},
		}, nil,
	)

	obtained, err := s.modelService.GetUnitAgentStatusesForApplication(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained, tc.DeepEquals, map[coreunit.Name]corestatus.StatusInfo{
		"unit-1": {
			Status:  corestatus.Allocating,
			Message: "doink",
			Data:    map[string]any{"foo": "bar"},
			Since:   &now,
		},
		"unit-2": {
			Status:  corestatus.Error,
			Message: "boink",
			Data:    map[string]any{"foo": "baz"},
			Since:   &now,
		},
	})
}

func (s *serviceSuite) TestGetUnitDisplayAndAgentStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	unitUUID := coreunit.GenUUID(c)
	s.modelState.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.modelState.EXPECT().GetUnitAgentStatus(gomock.Any(), unitUUID).Return(
		status.UnitStatusInfo[status.UnitAgentStatusType]{
			StatusInfo: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusAllocating,
				Message: "doink",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &now,
			},
			Present: true,
		}, nil)
	s.modelState.EXPECT().GetUnitWorkloadStatus(gomock.Any(), unitUUID).Return(
		status.UnitStatusInfo[status.WorkloadStatusType]{
			StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
				Message: "doink",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &now,
			},
			Present: true,
		}, nil)

	s.modelState.EXPECT().GetUnitK8sPodStatus(gomock.Any(), unitUUID).Return(status.StatusInfo[status.K8sPodStatusType]{
		Status: status.K8sPodStatusUnset,
	}, nil)

	agent, workload, err := s.modelService.GetUnitDisplayAndAgentStatus(c.Context(), coreunit.Name("foo/666"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(agent, tc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Allocating,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
	c.Check(workload, tc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
}

func (s *serviceSuite) TestGetUnitDisplayAndAgentStatusWithAllocatingPresence(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	unitUUID := coreunit.GenUUID(c)
	s.modelState.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.modelState.EXPECT().GetUnitAgentStatus(gomock.Any(), unitUUID).Return(
		status.UnitStatusInfo[status.UnitAgentStatusType]{
			StatusInfo: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusAllocating,
				Message: "doink",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &now,
			},
			Present: false,
		}, nil)
	s.modelState.EXPECT().GetUnitWorkloadStatus(gomock.Any(), unitUUID).Return(
		status.UnitStatusInfo[status.WorkloadStatusType]{
			StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
				Message: "doink",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &now,
			},
			Present: true,
		}, nil)

	s.modelState.EXPECT().GetUnitK8sPodStatus(gomock.Any(), unitUUID).Return(status.StatusInfo[status.K8sPodStatusType]{
		Status: status.K8sPodStatusUnset,
	}, nil)

	agent, workload, err := s.modelService.GetUnitDisplayAndAgentStatus(c.Context(), coreunit.Name("foo/666"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(agent, tc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Allocating,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
	c.Check(workload, tc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
}

func (s *serviceSuite) TestGetUnitDisplayAndAgentStatusWithNoPresence(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	unitUUID := coreunit.GenUUID(c)
	s.modelState.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.modelState.EXPECT().GetUnitAgentStatus(gomock.Any(), unitUUID).Return(
		status.UnitStatusInfo[status.UnitAgentStatusType]{
			StatusInfo: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusIdle,
				Message: "doink",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &now,
			},
			Present: false,
		}, nil)
	s.modelState.EXPECT().GetUnitWorkloadStatus(gomock.Any(), unitUUID).Return(
		status.UnitStatusInfo[status.WorkloadStatusType]{
			StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
				Message: "doink",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &now,
			},
			Present: false,
		}, nil)

	s.modelState.EXPECT().GetUnitK8sPodStatus(gomock.Any(), unitUUID).Return(status.StatusInfo[status.K8sPodStatusType]{
		Status: status.K8sPodStatusUnset,
	}, nil)

	agent, workload, err := s.modelService.GetUnitDisplayAndAgentStatus(c.Context(), coreunit.Name("foo/666"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(agent, tc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Lost,
		Message: "agent is not communicating with the server",
		Since:   &now,
	})
	c.Check(workload, tc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Unknown,
		Message: "agent lost, see `juju debug-logs` or `juju show-status-log` for more information",
		Since:   &now,
	})
}

func (s *serviceSuite) TestGetUnitWorkloadStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	unitUUID := coreunit.GenUUID(c)
	s.modelState.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.modelState.EXPECT().GetUnitWorkloadStatus(gomock.Any(), unitUUID).Return(
		status.UnitStatusInfo[status.WorkloadStatusType]{
			StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
				Message: "doink",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &now,
			},
			Present: true,
		}, nil)

	obtained, err := s.modelService.GetUnitWorkloadStatus(c.Context(), coreunit.Name("foo/666"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained, tc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
}

func (s *serviceSuite) TestGetUnitWorkloadStatusUnitInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.modelService.GetUnitWorkloadStatus(c.Context(), coreunit.Name("!!!"))
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *serviceSuite) TestGetUnitWorkloadStatusUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := coreunit.GenUUID(c)
	s.modelState.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, statuserrors.UnitNotFound)

	_, err := s.modelService.GetUnitWorkloadStatus(c.Context(), coreunit.Name("foo/666"))
	c.Assert(err, tc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *serviceSuite) TestGetUnitWorkloadStatusUnitInvalidWorkloadStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := coreunit.GenUUID(c)
	s.modelState.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.modelState.EXPECT().GetUnitWorkloadStatus(gomock.Any(), unitUUID).Return(status.UnitStatusInfo[status.WorkloadStatusType]{}, errors.Errorf("boom"))

	_, err := s.modelService.GetUnitWorkloadStatus(c.Context(), coreunit.Name("foo/666"))
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) TestSetUnitWorkloadStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := coreunit.GenUUID(c)
	now := time.Now()

	s.modelState.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.modelState.EXPECT().SetUnitWorkloadStatus(gomock.Any(), unitUUID, status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "doink",
		Data:    []byte(`{"foo":"bar"}`),
		Since:   &now,
	})

	err := s.modelService.SetUnitWorkloadStatus(c.Context(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetUnitWorkloadStatusInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	err := s.modelService.SetUnitWorkloadStatus(c.Context(), coreunit.Name("!!!"), corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *serviceSuite) TestSetUnitWorkloadStatusUnitFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := coreunit.GenUUID(c)
	now := time.Now()

	s.modelState.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, statuserrors.UnitNotFound)

	err := s.modelService.SetUnitWorkloadStatus(c.Context(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *serviceSuite) TestSetUnitWorkloadStatusInvalidStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := coreunit.GenUUID(c)
	now := time.Now()

	s.modelState.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.modelState.EXPECT().SetUnitWorkloadStatus(gomock.Any(), unitUUID, status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "doink",
		Data:    []byte(`{"foo":"bar"}`),
		Since:   &now,
	}).Return(errors.New("boom"))

	err := s.modelService.SetUnitWorkloadStatus(c.Context(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorMatches, ".*boom")
}

func (s *serviceSuite) TestSetUnitAgentStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := coreunit.GenUUID(c)
	now := time.Now()

	s.modelState.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.modelState.EXPECT().SetUnitAgentStatus(gomock.Any(), unitUUID, status.StatusInfo[status.UnitAgentStatusType]{
		Status:  status.UnitAgentStatusIdle,
		Message: "doink",
		Data:    []byte(`{"foo":"bar"}`),
		Since:   &now,
	})

	err := s.modelService.SetUnitAgentStatus(c.Context(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status:  corestatus.Idle,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetUnitAgentStatusErrorWithNoMessage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	err := s.modelService.SetUnitAgentStatus(c.Context(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status:  corestatus.Error,
		Message: "",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorMatches, `setting status "error" without message`)
}

func (s *serviceSuite) TestSetUnitAgentStatusLost(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	err := s.modelService.SetUnitAgentStatus(c.Context(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status:  corestatus.Lost,
		Message: "are you lost?",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorMatches, `setting status "lost" is not allowed`)
}

func (s *serviceSuite) TestSetUnitAgentStatusAllocating(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	err := s.modelService.SetUnitAgentStatus(c.Context(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status:  corestatus.Allocating,
		Message: "help me help you",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorMatches, `setting status "allocating" is not allowed`)
}

func (s *serviceSuite) TestSetUnitAgentStatusInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	err := s.modelService.SetUnitAgentStatus(c.Context(), coreunit.Name("!!!"), corestatus.StatusInfo{
		Status:  corestatus.Idle,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *serviceSuite) TestSetUnitAgentStatusUnitFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := coreunit.GenUUID(c)
	now := time.Now()

	s.modelState.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, statuserrors.UnitNotFound)

	err := s.modelService.SetUnitAgentStatus(c.Context(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status:  corestatus.Idle,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *serviceSuite) TestSetUnitAgentStatusInvalidStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := coreunit.GenUUID(c)
	now := time.Now()

	s.modelState.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.modelState.EXPECT().SetUnitAgentStatus(gomock.Any(), unitUUID, status.StatusInfo[status.UnitAgentStatusType]{
		Status:  status.UnitAgentStatusIdle,
		Message: "doink",
		Data:    []byte(`{"foo":"bar"}`),
		Since:   &now,
	}).Return(errors.New("boom"))

	err := s.modelService.SetUnitAgentStatus(c.Context(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status:  corestatus.Idle,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorMatches, ".*boom")
}

func (s *serviceSuite) TestSetUnitPresence(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().SetUnitPresence(gomock.Any(), coreunit.Name("foo/666"))

	err := s.modelService.SetUnitPresence(c.Context(), coreunit.Name("foo/666"))
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetUnitPresenceInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.modelService.SetUnitPresence(c.Context(), coreunit.Name("!!!"))
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *serviceSuite) TestDeleteUnitPresence(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().DeleteUnitPresence(gomock.Any(), coreunit.Name("foo/666"))

	err := s.modelService.DeleteUnitPresence(c.Context(), coreunit.Name("foo/666"))
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestDeleteUnitPresenceInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.modelService.DeleteUnitPresence(c.Context(), coreunit.Name("!!!"))
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *serviceSuite) TestCheckUnitStatusesReadyForMigrationEmptyModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetAllUnitWorkloadAgentStatuses(gomock.Any()).Return(status.UnitWorkloadAgentStatuses{}, nil)

	err := s.modelService.CheckUnitStatusesReadyForMigration(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestCheckUnitStatusesReadyForMigration(c *tc.C) {
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
	s.modelState.EXPECT().GetAllUnitWorkloadAgentStatuses(gomock.Any()).Return(fullStatus, nil)

	err := s.modelService.CheckUnitStatusesReadyForMigration(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestCheckUnitStatusesReadyForMigrationNotReadyPresence(c *tc.C) {
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
	s.modelState.EXPECT().GetAllUnitWorkloadAgentStatuses(gomock.Any()).Return(fullStatus, nil)

	err := s.modelService.CheckUnitStatusesReadyForMigration(c.Context())
	c.Assert(err, tc.ErrorMatches, `(?m).*
- unit "foo/66\d" is not logged into the controller
- unit "foo/66\d" is not logged into the controller`)
}

func (s *serviceSuite) TestCheckUnitStatusesReadyForMigrationNotReadyAgentStatus(c *tc.C) {
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
	s.modelState.EXPECT().GetAllUnitWorkloadAgentStatuses(gomock.Any()).Return(fullStatus, nil)

	err := s.modelService.CheckUnitStatusesReadyForMigration(c.Context())
	c.Assert(err, tc.ErrorMatches, `(?m).*
- unit "foo/66\d" agent not idle or executing
- unit "foo/66\d" agent not idle or executing`)
}

func (s *serviceSuite) TestCheckUnitStatusesReadyForMigrationNotReadyWorkload(c *tc.C) {
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
	s.modelState.EXPECT().GetAllUnitWorkloadAgentStatuses(gomock.Any()).Return(fullStatus, nil)

	err := s.modelService.CheckUnitStatusesReadyForMigration(c.Context())
	c.Assert(err, tc.ErrorMatches, `(?m).*
- unit "foo/66\d" workload not active or viable
- unit "foo/66\d" workload not active or viable`)
}

func (s *serviceSuite) TestCheckUnitStatusesReadyForMigrationNotReadyWorkloadMessage(c *tc.C) {
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
	s.modelState.EXPECT().GetAllUnitWorkloadAgentStatuses(gomock.Any()).Return(fullStatus, nil)

	err := s.modelService.CheckUnitStatusesReadyForMigration(c.Context())
	c.Assert(err, tc.ErrorMatches, `(?m).*
- unit "foo/66\d" workload not active or viable
- unit "foo/66\d" workload not active or viable`)
}

func (s *serviceSuite) TestExportMachineStatusesEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetAllMachineStatuses(gomock.Any()).Return(map[string]status.StatusInfo[status.MachineStatusType]{}, nil)
	s.modelState.EXPECT().GetAllInstanceStatuses(gomock.Any()).Return(map[string]status.StatusInfo[status.InstanceStatusType]{}, nil)

	machineStatuses, instanceStatuses, err := s.modelService.ExportMachineStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machineStatuses, tc.HasLen, 0)
	c.Check(instanceStatuses, tc.HasLen, 0)
}

func (s *serviceSuite) TestExportMachineStatuses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mStatuses := map[string]status.StatusInfo[status.MachineStatusType]{
		"0": {
			Status:  status.MachineStatusStarted,
			Message: "it's started",
			Data:    []byte(`{"foo":"bar"}`),
		},
		"1": {
			Status:  status.MachineStatusPending,
			Message: "it's pending",
			Data:    []byte(`{"foo":"baz"}`),
		},
	}
	iStatuses := map[string]status.StatusInfo[status.InstanceStatusType]{
		"0": {
			Status:  status.InstanceStatusRunning,
			Message: "it's running",
			Data:    []byte(`{"foo":"bar"}`),
		},
		"1": {
			Status:  status.InstanceStatusPending,
			Message: "it's pending",
			Data:    []byte(`{"foo":"baz"}`),
		},
	}
	s.modelState.EXPECT().GetAllMachineStatuses(gomock.Any()).Return(mStatuses, nil)
	s.modelState.EXPECT().GetAllInstanceStatuses(gomock.Any()).Return(iStatuses, nil)

	machineStatuses, instanceStatuses, err := s.modelService.ExportMachineStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machineStatuses, tc.DeepEquals, map[machine.Name]corestatus.StatusInfo{
		"0": {
			Status:  corestatus.Started,
			Message: "it's started",
			Data:    map[string]any{"foo": "bar"},
		},
		"1": {
			Status:  corestatus.Pending,
			Message: "it's pending",
			Data:    map[string]any{"foo": "baz"},
		},
	})
	c.Check(instanceStatuses, tc.DeepEquals, map[machine.Name]corestatus.StatusInfo{
		"0": {
			Status:  corestatus.Running,
			Message: "it's running",
			Data:    map[string]any{"foo": "bar"},
		},
		"1": {
			Status:  corestatus.Pending,
			Message: "it's pending",
			Data:    map[string]any{"foo": "baz"},
		},
	})
}

func (s *serviceSuite) TestExportUnitStatusesEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetAllUnitWorkloadAgentStatuses(gomock.Any()).Return(status.UnitWorkloadAgentStatuses{}, nil)

	workloadStatuses, agentStatuses, err := s.modelService.ExportUnitStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(workloadStatuses, tc.HasLen, 0)
	c.Check(agentStatuses, tc.HasLen, 0)
}

func (s *serviceSuite) TestExportUnitStatuses(c *tc.C) {
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
	s.modelState.EXPECT().GetAllUnitWorkloadAgentStatuses(gomock.Any()).Return(fullStatus, nil)

	workloadStatuses, agentStatuses, err := s.modelService.ExportUnitStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(workloadStatuses, tc.DeepEquals, map[coreunit.Name]corestatus.StatusInfo{
		"foo/66": {
			Status:  corestatus.Active,
			Message: "it's active",
			Data:    map[string]any{"foo": "bar"},
			Since:   &now,
		},
		"foo/67": {
			Status:  corestatus.Blocked,
			Message: "it's blocked",
			Data:    map[string]any{"baz": "foo"},
			Since:   &now,
		},
	})
	c.Check(agentStatuses, tc.DeepEquals, map[coreunit.Name]corestatus.StatusInfo{
		"foo/66": {
			Status:  corestatus.Idle,
			Message: "it's idle",
			Data:    map[string]any{"bar": "foo"},
			Since:   &now,
		},
		"foo/67": {
			Status:  corestatus.Allocating,
			Message: "it's allocating",
			Data:    map[string]any{"foo": "baz"},
			Since:   &now,
		},
	})
}

func (s *serviceSuite) TestExportApplicationStatusesEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetAllApplicationStatuses(gomock.Any()).Return(map[string]status.StatusInfo[status.WorkloadStatusType]{}, nil)

	statuses, err := s.modelService.ExportApplicationStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statuses, tc.HasLen, 0)
}

func (s *serviceSuite) TestExportApplicationStatuses(c *tc.C) {
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
	s.modelState.EXPECT().GetAllApplicationStatuses(gomock.Any()).Return(statuses, nil)

	exported, err := s.modelService.ExportApplicationStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exported, tc.DeepEquals, map[string]corestatus.StatusInfo{
		"foo": {
			Status:  corestatus.Active,
			Message: "it's active",
			Data:    map[string]any{"foo": "bar"},
			Since:   &now,
		},
		"bar": {
			Status:  corestatus.Blocked,
			Message: "it's blocked",
			Data:    map[string]any{"bar": "foo"},
			Since:   &now,
		},
	})
}

func (s *serviceSuite) TestGetApplicationAndUnitStatusesNoApps(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetApplicationAndUnitStatuses(gomock.Any()).Return(
		map[string]status.Application{}, nil,
	)

	statuses, err := s.modelService.GetApplicationAndUnitStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statuses, tc.DeepEquals, map[string]Application{})
}

func (s *serviceSuite) TestGetApplicationAndUnitStatusesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetApplicationAndUnitStatuses(gomock.Any()).Return(
		map[string]status.Application{}, errors.Errorf("boom"),
	)

	_, err := s.modelService.GetApplicationAndUnitStatuses(c.Context())
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) TestGetApplicationAndUnitStatuses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relationUUID := corerelation.GenRelationUUID(c)

	s.modelState.EXPECT().GetApplicationAndUnitStatuses(gomock.Any()).Return(
		map[string]status.Application{
			"foo": {
				ID:   "deadbeef",
				Life: life.Alive,
				Status: status.StatusInfo[status.WorkloadStatusType]{
					Status:  status.WorkloadStatusActive,
					Message: "it's active",
					Data:    []byte(`{"foo": "bar"}`),
				},
				Relations: []corerelation.UUID{
					relationUUID,
				},
				Subordinate: true,
				CharmLocator: charm.CharmLocator{
					Name:         "foo",
					Revision:     32,
					Source:       "local",
					Architecture: architecture.ARM64,
				},
				CharmVersion: "1.0.0",
				Platform: deployment.Platform{
					Channel:      "stable",
					OSType:       0,
					Architecture: architecture.ARM64,
				},
				Channel: &deployment.Channel{
					Track:  "4.0",
					Risk:   deployment.RiskCandidate,
					Branch: "test",
				},
				LXDProfile:    []byte(`{}`),
				Exposed:       true,
				Scale:         ptr(2),
				K8sProviderID: ptr("k8s-provider-id"),
				Units: map[coreunit.Name]status.Unit{
					"foo/666": {
						Life: life.Alive,
						WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
							Status:  status.WorkloadStatusActive,
							Message: "it's active",
							Data:    []byte(`{"foo": "bar"}`),
						},
						AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
							Status:  status.UnitAgentStatusIdle,
							Message: "it's idle",
							Data:    []byte(`{"foo": "bar"}`),
						},
						K8sPodStatus: status.StatusInfo[status.K8sPodStatusType]{
							Status:  status.K8sPodStatusUnset,
							Message: "it's unset",
						},
						Present: true,
						CharmLocator: charm.CharmLocator{
							Name:         "foo",
							Revision:     42,
							Source:       "local",
							Architecture: architecture.ARM64,
						},
						Subordinate: false,
						MachineName: ptr(machine.Name("0")),
						SubordinateNames: map[coreunit.Name]struct{}{
							coreunit.Name("foo/667"): {},
						},
						ApplicationName: "foo",
						PrincipalName:   ptr(coreunit.Name("foo/666")),
						AgentVersion:    "1.0.0",
						WorkloadVersion: ptr("v1.0.0"),
						K8sProviderID:   ptr("k8s-provider-id"),
					},
				},
			},
		}, nil,
	)

	statuses, err := s.modelService.GetApplicationAndUnitStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statuses, tc.DeepEquals, map[string]Application{
		"foo": {
			Life: corelife.Alive,
			Status: corestatus.StatusInfo{
				Status:  corestatus.Active,
				Message: "it's active",
				Data:    map[string]any{"foo": "bar"},
			},
			Relations: []corerelation.UUID{
				relationUUID,
			},
			Subordinate: true,
			CharmLocator: charm.CharmLocator{
				Name:         "foo",
				Revision:     32,
				Source:       "local",
				Architecture: architecture.ARM64,
			},
			CharmVersion: "1.0.0",
			Platform: deployment.Platform{
				Channel:      "stable",
				OSType:       0,
				Architecture: architecture.ARM64,
			},
			Channel: &deployment.Channel{
				Track:  "4.0",
				Risk:   deployment.RiskCandidate,
				Branch: "test",
			},
			LXDProfile:    &internalcharm.LXDProfile{},
			Exposed:       true,
			Scale:         ptr(2),
			K8sProviderID: ptr("k8s-provider-id"),
			Units: map[coreunit.Name]Unit{
				"foo/666": {
					Life: corelife.Alive,
					WorkloadStatus: corestatus.StatusInfo{
						Status:  corestatus.Active,
						Message: "it's active",
						Data:    map[string]any{"foo": "bar"},
					},
					AgentStatus: corestatus.StatusInfo{
						Status:  corestatus.Idle,
						Message: "it's idle",
						Data:    map[string]any{"foo": "bar"},
					},
					K8sPodStatus: corestatus.StatusInfo{
						Status:  corestatus.Unset,
						Message: "it's unset",
					},
					Present: true,
					CharmLocator: charm.CharmLocator{
						Name:         "foo",
						Revision:     42,
						Source:       "local",
						Architecture: architecture.ARM64,
					},
					Subordinate: false,
					MachineName: ptr(machine.Name("0")),
					SubordinateNames: []coreunit.Name{
						coreunit.Name("foo/667"),
					},
					ApplicationName: "foo",
					PrincipalName:   ptr(coreunit.Name("foo/666")),
					AgentVersion:    "1.0.0",
					WorkloadVersion: ptr("v1.0.0"),
					K8sProviderID:   ptr("k8s-provider-id"),
				},
			},
		},
	})
}

func (s *serviceSuite) TestGetApplicationAndUnitModelStatuses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetApplicationAndUnitModelStatuses(gomock.Any()).Return(
		map[string]int{
			"foo": 2,
		}, nil,
	)

	statuses, err := s.modelService.GetApplicationAndUnitModelStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statuses, tc.DeepEquals, map[string]int{
		"foo": 2,
	})
}

func (s *serviceSuite) TestGetApplicationAndUnitStatusesInvalidLXDProfile(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetApplicationAndUnitStatuses(gomock.Any()).Return(
		map[string]status.Application{
			"foo": {
				ID: "deadbeef",
				Status: status.StatusInfo[status.WorkloadStatusType]{
					Status: status.WorkloadStatusActive,
				},
				LXDProfile: []byte(`{!!!}`),
			},
		}, nil,
	)

	_, err := s.modelService.GetApplicationAndUnitStatuses(c.Context())
	c.Assert(err, tc.ErrorMatches, `.*decoding LXD profile.*`)
}

// TestGetMachineStatusSuccess asserts the happy path of the GetMachineStatus.
func (s *serviceSuite) TestGetMachineStatusSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expectedStatus := corestatus.StatusInfo{
		Status: corestatus.Started,
		Data:   map[string]interface{}{"foo": "bar"},
	}
	s.modelState.EXPECT().GetMachineStatus(gomock.Any(), "666").Return(status.StatusInfo[status.MachineStatusType]{
		Status: status.MachineStatusStarted,
		Data:   []byte(`{"foo":"bar"}`),
	}, nil)

	machineStatus, err := s.modelService.
		GetMachineStatus(c.Context(), "666")
	c.Check(err, tc.ErrorIsNil)
	c.Assert(machineStatus, tc.DeepEquals, expectedStatus)
}

// TestGetMachineStatusError asserts that an error coming from the state layer
// is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestGetMachineStatusError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.modelState.EXPECT().GetMachineStatus(gomock.Any(), "666").Return(status.StatusInfo[status.MachineStatusType]{}, rErr)

	machineStatus, err := s.modelService.
		GetMachineStatus(c.Context(), "666")
	c.Check(err, tc.ErrorIs, rErr)
	c.Check(machineStatus, tc.DeepEquals, corestatus.StatusInfo{})
}

func (s *serviceSuite) TestGetAllMachineStatuses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expectedStatuses := map[machine.Name]corestatus.StatusInfo{
		"666": {
			Status: corestatus.Started,
			Data: map[string]interface{}{
				"foo": "bar",
			},
		},
		"777": {
			Status: corestatus.Pending,
			Data: map[string]interface{}{
				"foo": "baz",
			},
		},
		"888": {
			Status: corestatus.Stopped,
			Data: map[string]interface{}{
				"foo": "qux",
			},
		},
	}
	s.modelState.EXPECT().GetAllMachineStatuses(gomock.Any()).Return(map[string]status.StatusInfo[status.MachineStatusType]{
		"666": {
			Status: status.MachineStatusStarted,
			Data:   []byte(`{"foo": "bar"}`),
		},
		"777": {
			Status: status.MachineStatusPending,
			Data:   []byte(`{"foo": "baz"}`),
		},
		"888": {
			Status: status.MachineStatusStopped,
			Data:   []byte(`{"foo": "qux"}`),
		},
	}, nil)

	statuses, err := s.modelService.GetAllMachineStatuses(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(statuses, tc.DeepEquals, expectedStatuses)
}

func (s *serviceSuite) TestGetMachineFullStatuses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expectedStatuses := map[machine.Name]Machine{
		"666": {
			Name:        "666",
			Life:        corelife.Alive,
			DNSName:     "10.51.45.181",
			IPAddresses: []string{"10.0.0.1", "10.51.45.181"},
			MachineStatus: corestatus.StatusInfo{
				Status: corestatus.Started,
				Data: map[string]interface{}{
					"foo": "bar",
				},
			},
			InstanceStatus: corestatus.StatusInfo{
				Status: corestatus.Running,
			},
		},
		"777": {
			Name:        "777",
			Life:        corelife.Dying,
			DNSName:     "10.51.45.182",
			IPAddresses: []string{"10.0.0.1", "10.51.45.181"},
			MachineStatus: corestatus.StatusInfo{
				Status: corestatus.Pending,
				Data: map[string]interface{}{
					"foo": "baz",
				},
			},
			InstanceStatus: corestatus.StatusInfo{
				Status: corestatus.Allocating,
			},
		},
		"888": {
			Name:        "888",
			Life:        corelife.Dead,
			DNSName:     "10.51.45.183",
			IPAddresses: []string{"10.0.0.1", "10.51.45.181"},
			MachineStatus: corestatus.StatusInfo{
				Status: corestatus.Stopped,
				Data: map[string]interface{}{
					"foo": "qux",
				},
			},
			InstanceStatus: corestatus.StatusInfo{
				Status: corestatus.Unset,
			},
		},
	}
	s.modelState.EXPECT().GetMachineFullStatuses(gomock.Any()).Return(map[machine.Name]status.Machine{
		"666": {
			Life:        life.Alive,
			DNSName:     "10.51.45.181",
			IPAddresses: []string{"10.0.0.1", "10.51.45.181"},
			MachineStatus: status.StatusInfo[status.MachineStatusType]{
				Status: status.MachineStatusStarted,
				Data:   []byte(`{"foo": "bar"}`),
			},
			InstanceStatus: status.StatusInfo[status.InstanceStatusType]{
				Status: status.InstanceStatusRunning,
			},
		},
		"777": {
			Life:        life.Dying,
			DNSName:     "10.51.45.182",
			IPAddresses: []string{"10.0.0.1", "10.51.45.181"},
			MachineStatus: status.StatusInfo[status.MachineStatusType]{
				Status: status.MachineStatusPending,
				Data:   []byte(`{"foo": "baz"}`),
			},
			InstanceStatus: status.StatusInfo[status.InstanceStatusType]{
				Status: status.InstanceStatusAllocating,
			},
		},
		"888": {
			Life:        life.Dead,
			DNSName:     "10.51.45.183",
			IPAddresses: []string{"10.0.0.1", "10.51.45.181"},
			MachineStatus: status.StatusInfo[status.MachineStatusType]{
				Status: status.MachineStatusStopped,
				Data:   []byte(`{"foo": "qux"}`),
			},
		},
	}, nil)

	statuses, err := s.modelService.GetMachineFullStatuses(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(statuses, tc.DeepEquals, expectedStatuses)
}

// TestSetMachineStatusSuccess asserts the happy path of the SetMachineStatus.
func (s *serviceSuite) TestSetMachineStatusSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	newStatus := corestatus.StatusInfo{Status: corestatus.Started}
	s.modelState.EXPECT().SetMachineStatus(gomock.Any(), "666", status.StatusInfo[status.MachineStatusType]{
		Status: status.MachineStatusStarted,
	}).Return(nil)

	err := s.modelService.
		SetMachineStatus(c.Context(), "666", newStatus)
	c.Check(err, tc.ErrorIsNil)

	c.Check(s.statusHistory.records, tc.DeepEquals, []statusHistoryRecord{{
		ns: status.MachineNamespace.WithID("666"),
		s:  newStatus,
	}})
}

// TestSetMachineStatusError asserts that an error coming from the state layer
// is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestSetMachineStatusError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	newStatus := corestatus.StatusInfo{Status: corestatus.Started}
	rErr := errors.New("boom")
	s.modelState.EXPECT().SetMachineStatus(gomock.Any(), "666", status.StatusInfo[status.MachineStatusType]{
		Status: status.MachineStatusStarted,
	}).Return(rErr)

	err := s.modelService.
		SetMachineStatus(c.Context(), "666", newStatus)
	c.Check(err, tc.ErrorIs, rErr)
}

// TestSetMachineStatusInvalid asserts that an invalid status is passed to the
// service will result in a InvalidStatus error.
func (s *serviceSuite) TestSetMachineStatusInvalid(c *tc.C) {
	err := s.modelService.
		SetMachineStatus(c.Context(), "666", corestatus.StatusInfo{Status: "invalid"})
	c.Check(err, tc.ErrorIs, statuserrors.InvalidStatus)
}

// TestGetInstanceStatusSuccess asserts the happy path of the GetInstanceStatus.
func (s *serviceSuite) TestGetInstanceStatusSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expectedStatus := corestatus.StatusInfo{
		Status: corestatus.Running,
		Data:   map[string]interface{}{"foo": "bar"},
	}
	s.modelState.EXPECT().GetInstanceStatus(gomock.Any(), "666").Return(status.StatusInfo[status.InstanceStatusType]{
		Status: status.InstanceStatusRunning,
		Data:   []byte(`{"foo":"bar"}`),
	}, nil)

	instanceStatus, err := s.modelService.
		GetInstanceStatus(c.Context(), "666")
	c.Check(err, tc.ErrorIsNil)
	c.Assert(instanceStatus, tc.DeepEquals, expectedStatus)
}

// TestGetInstanceStatusError asserts that an error coming from the state layer
// is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestGetInstanceStatusError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.modelState.EXPECT().GetInstanceStatus(gomock.Any(), "666").Return(status.StatusInfo[status.InstanceStatusType]{}, rErr)

	instanceStatus, err := s.modelService.
		GetInstanceStatus(c.Context(), "666")
	c.Check(err, tc.ErrorIs, rErr)
	c.Check(instanceStatus, tc.DeepEquals, corestatus.StatusInfo{})
}

// TestSetInstanceStatusSuccess asserts the happy path of the SetInstanceStatus
// service.
func (s *serviceSuite) TestSetInstanceStatusSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	newStatus := corestatus.StatusInfo{Status: corestatus.Running}
	s.modelState.EXPECT().SetInstanceStatus(gomock.Any(), "666", status.StatusInfo[status.InstanceStatusType]{
		Status: status.InstanceStatusRunning,
	}).Return(nil)

	err := s.modelService.
		SetInstanceStatus(c.Context(), "666", newStatus)
	c.Check(err, tc.ErrorIsNil)

	c.Check(s.statusHistory.records, tc.DeepEquals, []statusHistoryRecord{{
		ns: status.MachineInstanceNamespace.WithID("666"),
		s:  newStatus,
	}})
}

// TestSetInstanceStatusError asserts that an error coming from the state layer
// is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestSetInstanceStatusError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	newStatus := corestatus.StatusInfo{Status: corestatus.Running}
	s.modelState.EXPECT().SetInstanceStatus(gomock.Any(), "666", status.StatusInfo[status.InstanceStatusType]{
		Status: status.InstanceStatusRunning,
	}).Return(rErr)

	err := s.modelService.
		SetInstanceStatus(c.Context(), "666", newStatus)
	c.Check(err, tc.ErrorIs, rErr)
}

// TestSetInstanceStatusInvalid asserts that an invalid status is passed to the
// service will result in a InvalidStatus error.
func (s *serviceSuite) TestSetInstanceStatusInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.modelService.
		SetInstanceStatus(c.Context(), "666", corestatus.StatusInfo{Status: "invalid"})
	c.Check(err, tc.ErrorIs, statuserrors.InvalidStatus)
}

func (s *serviceSuite) TestCheckMachineStatusesReadyForMigrationEmptyModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetAllMachineStatuses(gomock.Any()).Return(map[string]status.StatusInfo[status.MachineStatusType]{}, nil)
	s.modelState.EXPECT().GetAllInstanceStatuses(gomock.Any()).Return(map[string]status.StatusInfo[status.InstanceStatusType]{}, nil)

	err := s.modelService.CheckMachineStatusesReadyForMigration(c.Context())
	c.Check(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestCheckMachineStatusesReadyForMigrationSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetAllMachineStatuses(gomock.Any()).Return(map[string]status.StatusInfo[status.MachineStatusType]{
		"666": {
			Status: status.MachineStatusStarted,
		},
		"777": {
			Status: status.MachineStatusStarted,
		},
	}, nil)

	s.modelState.EXPECT().GetAllInstanceStatuses(gomock.Any()).Return(map[string]status.StatusInfo[status.InstanceStatusType]{
		"666": {
			Status: status.InstanceStatusRunning,
		},
		"777": {
			Status: status.InstanceStatusRunning,
		},
	}, nil)

	err := s.modelService.CheckMachineStatusesReadyForMigration(c.Context())
	c.Check(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestCheckMachineStatusesReadyForMigrationMissingInstanceStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetAllMachineStatuses(gomock.Any()).Return(map[string]status.StatusInfo[status.MachineStatusType]{
		"666": {
			Status: status.MachineStatusStarted,
		},
		"777": {
			Status: status.MachineStatusStarted,
		},
	}, nil)

	s.modelState.EXPECT().GetAllInstanceStatuses(gomock.Any()).Return(map[string]status.StatusInfo[status.InstanceStatusType]{
		"666": {
			Status: status.InstanceStatusRunning,
		},
	}, nil)

	err := s.modelService.CheckMachineStatusesReadyForMigration(c.Context())
	c.Check(err, tc.ErrorMatches, "some machines have unset statuses")
}

func (s *serviceSuite) TestCheckMachineStatusesReadyForMigrationStatusMismatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetAllMachineStatuses(gomock.Any()).Return(map[string]status.StatusInfo[status.MachineStatusType]{
		"666": {
			Status: status.MachineStatusStarted,
		},
		"777": {
			Status: status.MachineStatusStarted,
		},
	}, nil)

	s.modelState.EXPECT().GetAllInstanceStatuses(gomock.Any()).Return(map[string]status.StatusInfo[status.InstanceStatusType]{
		"666": {
			Status: status.InstanceStatusRunning,
		},
		"888": {
			Status: status.InstanceStatusRunning,
		},
	}, nil)

	err := s.modelService.CheckMachineStatusesReadyForMigration(c.Context())
	c.Check(err, tc.ErrorMatches, "some machines have unset statuses")
}

func (s *serviceSuite) TestCheckMachineStatusesReadyForMigrationBadMachineStatuses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetAllMachineStatuses(gomock.Any()).Return(map[string]status.StatusInfo[status.MachineStatusType]{
		"650": {
			Status: status.MachineStatusStarted,
		},
		"667": {
			Status: status.MachineStatusError,
		},
		"668": {
			Status: status.MachineStatusPending,
		},
	}, nil)
	s.modelState.EXPECT().GetAllInstanceStatuses(gomock.Any()).Return(map[string]status.StatusInfo[status.InstanceStatusType]{
		"650": {
			Status: status.InstanceStatusRunning,
		},
		"667": {
			Status: status.InstanceStatusAllocating,
		},
		"668": {
			Status: status.InstanceStatusProvisioningError,
		},
	}, nil)

	err := s.modelService.CheckMachineStatusesReadyForMigration(c.Context())
	c.Check(err, tc.ErrorMatches, `(?m).*
- machine "66\d" status is not started
- machine "66\d" instance status is not running
- machine "66\d" status is not started
- machine "66\d" instance status is not running`)
}

func (s *serviceSuite) TestExportRelationStatuses(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	stateRelationStatus := []status.RelationStatusInfo{{
		RelationID: 1,
		StatusInfo: status.StatusInfo[status.RelationStatusType]{
			Status: status.RelationStatusTypeBroken,
		}},
	}
	s.modelState.EXPECT().GetAllRelationStatuses(gomock.Any()).Return(stateRelationStatus, nil)

	// Act
	details, err := s.modelService.ExportRelationStatuses(c.Context())

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(details, tc.DeepEquals, map[int]corestatus.StatusInfo{
		1: {
			Status: corestatus.Broken,
		},
	})
}

// TestExportRelationStatusesError verifies the behavior when ExportRelationStatuses
// encounters an error from the state layer.
func (s *serviceSuite) TestExportRelationStatusesError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	expectedError := errors.New("state error")
	s.modelState.EXPECT().GetAllRelationStatuses(gomock.Any()).Return(nil, expectedError)

	// Act
	_, err := s.modelService.ExportRelationStatuses(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIs, expectedError)
}

func (s *serviceSuite) TestGetModelInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelStatusInfo := status.ModelStatusInfo{
		Type: model.IAAS,
	}

	s.modelState.EXPECT().GetModelStatusInfo(gomock.Any()).Return(modelStatusInfo, nil)

	modelStatusInfo, err := s.modelService.GetModelStatusInfo(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(modelStatusInfo, tc.DeepEquals, modelStatusInfo)
}

func (s *serviceSuite) TestGetModelInfoNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetModelStatusInfo(gomock.Any()).Return(status.ModelStatusInfo{}, modelerrors.NotFound)

	_, err := s.modelService.GetModelStatusInfo(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *serviceSuite) TestGetStatusSuspended(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelStatusContext := status.ModelStatusContext{
		IsDestroying:                 false,
		IsMigrating:                  false,
		HasInvalidCloudCredential:    true,
		InvalidCloudCredentialReason: "invalid cloud credential reason",
	}

	s.controllerState.EXPECT().GetModelStatusContext(gomock.Any()).Return(modelStatusContext, nil)

	modelStatus, err := s.modelService.GetModelStatus(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelStatus.Status, tc.Equals, corestatus.Suspended)
	c.Assert(modelStatus.Message, tc.Equals, "suspended since cloud credential is not valid")
	c.Assert(modelStatus.Data, tc.DeepEquals, map[string]interface{}{"reason": modelStatusContext.InvalidCloudCredentialReason})

}

func (s *serviceSuite) TestGetStatusDestroying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelStatusContext := status.ModelStatusContext{
		IsDestroying:                 true,
		IsMigrating:                  false,
		HasInvalidCloudCredential:    false,
		InvalidCloudCredentialReason: "",
	}

	s.controllerState.EXPECT().GetModelStatusContext(gomock.Any()).Return(modelStatusContext, nil)

	modelStatus, err := s.modelService.GetModelStatus(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelStatus.Status, tc.Equals, corestatus.Destroying)
	c.Assert(modelStatus.Message, tc.Equals, "")
	c.Assert(modelStatus.Data, tc.IsNil)
}

func (s *serviceSuite) TestGetStatusMigrating(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelStatusContext := status.ModelStatusContext{
		IsDestroying:                 false,
		IsMigrating:                  true,
		HasInvalidCloudCredential:    false,
		InvalidCloudCredentialReason: "",
	}

	s.controllerState.EXPECT().GetModelStatusContext(gomock.Any()).Return(modelStatusContext, nil)

	modelStatus, err := s.modelService.GetModelStatus(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelStatus.Status, tc.Equals, corestatus.Busy)
	c.Assert(modelStatus.Message, tc.Equals, "the model is being migrated")
	c.Assert(modelStatus.Data, tc.IsNil)
}

func (s *serviceSuite) TestGetStatusAvailable(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelStatusContext := status.ModelStatusContext{
		IsDestroying:                 false,
		IsMigrating:                  false,
		HasInvalidCloudCredential:    false,
		InvalidCloudCredentialReason: "",
	}

	s.controllerState.EXPECT().GetModelStatusContext(gomock.Any()).Return(modelStatusContext, nil)

	modelStatus, err := s.modelService.GetModelStatus(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelStatus.Status, tc.Equals, corestatus.Available)
	c.Assert(modelStatus.Message, tc.Equals, "")
	c.Assert(modelStatus.Data, tc.IsNil)
}
func (s *serviceSuite) TestGetStatusNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerState.EXPECT().GetModelStatusContext(gomock.Any()).Return(status.ModelStatusContext{}, modelerrors.NotFound)

	_, err := s.modelService.GetModelStatus(c.Context())
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerState = NewMockControllerState(ctrl)
	s.modelState = NewMockModelState(ctrl)
	s.statusHistory = &statusHistoryRecorder{}

	s.modelService = NewService(
		s.modelState,
		s.controllerState,
		s.statusHistory,
		func() (StatusHistoryReader, error) {
			return nil, errors.Errorf("status history reader not available")
		},
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	c.Cleanup(func() {
		s.modelState = nil
		s.statusHistory = nil
		s.modelService = nil
	})

	return ctrl
}

func ptr[T any](v T) *T {
	return &v
}
