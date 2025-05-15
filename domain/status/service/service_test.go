// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	applicationtesting "github.com/juju/juju/core/application/testing"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	corerelation "github.com/juju/juju/core/relation"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
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
	state         *MockState
	statusHistory *statusHistoryRecorder

	service *Service
}

var _ = tc.Suite(&serviceSuite{})

// TestGetAllRelationStatuses verifies that GetAllRelationStatuses
// retrieves and returns the expected relation details without errors.
// Doesn't have logic, so the test doesn't need to be smart.
func (s *serviceSuite) TestGetAllRelationStatuses(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	relUUID := corerelationtesting.GenRelationUUID(c)
	stateRelationStatus := []status.RelationStatusInfo{{
		RelationUUID: relUUID,
		StatusInfo: status.StatusInfo[status.RelationStatusType]{
			Status: status.RelationStatusTypeBroken,
		}},
	}
	s.state.EXPECT().GetAllRelationStatuses(gomock.Any()).Return(stateRelationStatus, nil)

	// Act
	details, err := s.service.GetAllRelationStatuses(c.Context())

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
	s.state.EXPECT().GetAllRelationStatuses(gomock.Any()).Return(nil, expectedError)

	// Act
	_, err := s.service.GetAllRelationStatuses(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIs, expectedError)
}

func (s *serviceSuite) TestImportRelationStatus(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	relationID := 1
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
	s.state.EXPECT().ImportRelationStatus(gomock.Any(), relationID, expectedStatus).Return(nil)

	// Act
	err := s.service.ImportRelationStatus(c.Context(), relationID, sts)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestImportRelationServiceError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	relationID := 1
	boom := errors.New("boom")
	sts := corestatus.StatusInfo{
		Status: corestatus.Broken,
	}
	expectedStatus := status.StatusInfo[status.RelationStatusType]{
		Status: status.RelationStatusTypeBroken,
	}
	s.state.EXPECT().ImportRelationStatus(gomock.Any(), relationID, expectedStatus).Return(boom)

	// Act
	err := s.service.ImportRelationStatus(c.Context(), relationID, sts)

	// Assert
	c.Assert(err, tc.ErrorIs, boom)
}

func (s *serviceSuite) TestSetApplicationStatus(c *tc.C) {
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

	err := s.service.SetApplicationStatus(c.Context(), "gitlab", corestatus.StatusInfo{
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

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "gitlab").Return("", statuserrors.ApplicationNotFound)

	err := s.service.SetApplicationStatus(c.Context(), "gitlab", corestatus.StatusInfo{
		Status: corestatus.Active,
	})
	c.Assert(err, tc.ErrorIs, statuserrors.ApplicationNotFound)
}

func (s *serviceSuite) TestSetApplicationStatusInvalidStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.SetApplicationStatus(c.Context(), "gitlab", corestatus.StatusInfo{
		Status: corestatus.Status("invalid"),
	})
	c.Assert(err, tc.ErrorMatches, `.*unknown workload status "invalid"`)

	err = s.service.SetApplicationStatus(c.Context(), "gitlab", corestatus.StatusInfo{
		Status: corestatus.Allocating,
	})
	c.Assert(err, tc.ErrorMatches, `.*unknown workload status "allocating"`)
}

func (s *serviceSuite) TestGetApplicationDisplayStatusNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "gitlab").Return("", statuserrors.ApplicationNotFound)

	_, err := s.service.GetApplicationDisplayStatus(c.Context(), "gitlab")
	c.Assert(err, tc.ErrorIs, statuserrors.ApplicationNotFound)
}

func (s *serviceSuite) TestGetApplicationDisplayStatusApplicationStatusSet(c *tc.C) {
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

	obtained, err := s.service.GetApplicationDisplayStatus(c.Context(), "foo")
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

	applicationUUID := applicationtesting.GenApplicationUUID(c)
	s.state.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(applicationUUID, nil)
	s.state.EXPECT().GetApplicationStatus(gomock.Any(), applicationUUID).Return(
		status.StatusInfo[status.WorkloadStatusType]{
			Status: status.WorkloadStatusUnset,
		}, nil)

	s.state.EXPECT().GetAllFullUnitStatusesForApplication(gomock.Any(), applicationUUID).Return(nil, nil)

	obtained, err := s.service.GetApplicationDisplayStatus(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained, tc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Unknown,
	})
}

func (s *serviceSuite) TestGetApplicationDisplayStatusFallbackToUnitsNoContainers(c *tc.C) {
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

	obtained, err := s.service.GetApplicationDisplayStatus(c.Context(), "foo")
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

	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.state.EXPECT().SetUnitWorkloadStatus(gomock.Any(), unitUUID, status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "doink",
		Data:    []byte(`{"foo":"bar"}`),
		Since:   &now,
	})

	err := s.service.SetUnitWorkloadStatus(c.Context(), coreunit.Name("foo/666"), corestatus.StatusInfo{
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

	err := s.service.SetUnitWorkloadStatus(c.Context(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status: corestatus.Status("invalid"),
	})
	c.Assert(err, tc.ErrorMatches, `.*unknown workload status "invalid"`)

	err = s.service.SetUnitWorkloadStatus(c.Context(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status: corestatus.Allocating,
	})
	c.Assert(err, tc.ErrorMatches, `.*unknown workload status "allocating"`)
}

func (s *serviceSuite) TestGetUnitWorkloadStatusesForApplication(c *tc.C) {
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

	obtained, err := s.service.GetUnitWorkloadStatusesForApplication(c.Context(), appUUID)
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

func (s *serviceSuite) TestGetUnitDisplayAndAgentStatus(c *tc.C) {
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

	s.state.EXPECT().GetUnitK8sPodStatus(gomock.Any(), unitUUID).Return(status.StatusInfo[status.K8sPodStatusType]{
		Status: status.K8sPodStatusUnset,
	}, nil)

	agent, workload, err := s.service.GetUnitDisplayAndAgentStatus(c.Context(), coreunit.Name("foo/666"))
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

	s.state.EXPECT().GetUnitK8sPodStatus(gomock.Any(), unitUUID).Return(status.StatusInfo[status.K8sPodStatusType]{
		Status: status.K8sPodStatusUnset,
	}, nil)

	agent, workload, err := s.service.GetUnitDisplayAndAgentStatus(c.Context(), coreunit.Name("foo/666"))
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

	s.state.EXPECT().GetUnitK8sPodStatus(gomock.Any(), unitUUID).Return(status.StatusInfo[status.K8sPodStatusType]{
		Status: status.K8sPodStatusUnset,
	}, nil)

	agent, workload, err := s.service.GetUnitDisplayAndAgentStatus(c.Context(), coreunit.Name("foo/666"))
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

	obtained, err := s.service.GetUnitWorkloadStatus(c.Context(), coreunit.Name("foo/666"))
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

	_, err := s.service.GetUnitWorkloadStatus(c.Context(), coreunit.Name("!!!"))
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *serviceSuite) TestGetUnitWorkloadStatusUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, statuserrors.UnitNotFound)

	_, err := s.service.GetUnitWorkloadStatus(c.Context(), coreunit.Name("foo/666"))
	c.Assert(err, tc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *serviceSuite) TestGetUnitWorkloadStatusUnitInvalidWorkloadStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, nil)
	s.state.EXPECT().GetUnitWorkloadStatus(gomock.Any(), unitUUID).Return(status.UnitStatusInfo[status.WorkloadStatusType]{}, errors.Errorf("boom"))

	_, err := s.service.GetUnitWorkloadStatus(c.Context(), coreunit.Name("foo/666"))
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) TestSetUnitWorkloadStatus(c *tc.C) {
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

	err := s.service.SetUnitWorkloadStatus(c.Context(), coreunit.Name("foo/666"), corestatus.StatusInfo{
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

	err := s.service.SetUnitWorkloadStatus(c.Context(), coreunit.Name("!!!"), corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *serviceSuite) TestSetUnitWorkloadStatusUnitFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	now := time.Now()

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, statuserrors.UnitNotFound)

	err := s.service.SetUnitWorkloadStatus(c.Context(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *serviceSuite) TestSetUnitWorkloadStatusInvalidStatus(c *tc.C) {
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

	err := s.service.SetUnitWorkloadStatus(c.Context(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorMatches, ".*boom")
}

func (s *serviceSuite) TestSetUnitAgentStatus(c *tc.C) {
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

	err := s.service.SetUnitAgentStatus(c.Context(), coreunit.Name("foo/666"), corestatus.StatusInfo{
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

	err := s.service.SetUnitAgentStatus(c.Context(), coreunit.Name("foo/666"), corestatus.StatusInfo{
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

	err := s.service.SetUnitAgentStatus(c.Context(), coreunit.Name("foo/666"), corestatus.StatusInfo{
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

	err := s.service.SetUnitAgentStatus(c.Context(), coreunit.Name("foo/666"), corestatus.StatusInfo{
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

	err := s.service.SetUnitAgentStatus(c.Context(), coreunit.Name("!!!"), corestatus.StatusInfo{
		Status:  corestatus.Idle,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *serviceSuite) TestSetUnitAgentStatusUnitFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	now := time.Now()

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(unitUUID, statuserrors.UnitNotFound)

	err := s.service.SetUnitAgentStatus(c.Context(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status:  corestatus.Idle,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorIs, statuserrors.UnitNotFound)
}

func (s *serviceSuite) TestSetUnitAgentStatusInvalidStatus(c *tc.C) {
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

	err := s.service.SetUnitAgentStatus(c.Context(), coreunit.Name("foo/666"), corestatus.StatusInfo{
		Status:  corestatus.Idle,
		Message: "doink",
		Data:    map[string]any{"foo": "bar"},
		Since:   &now,
	})
	c.Assert(err, tc.ErrorMatches, ".*boom")
}

func (s *serviceSuite) TestSetUnitPresence(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetUnitPresence(gomock.Any(), coreunit.Name("foo/666"))

	err := s.service.SetUnitPresence(c.Context(), coreunit.Name("foo/666"))
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetUnitPresenceInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.SetUnitPresence(c.Context(), coreunit.Name("!!!"))
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *serviceSuite) TestDeleteUnitPresence(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().DeleteUnitPresence(gomock.Any(), coreunit.Name("foo/666"))

	err := s.service.DeleteUnitPresence(c.Context(), coreunit.Name("foo/666"))
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestDeleteUnitPresenceInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.DeleteUnitPresence(c.Context(), coreunit.Name("!!!"))
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *serviceSuite) TestCheckUnitStatusesReadyForMigrationEmptyModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAllUnitWorkloadAgentStatuses(gomock.Any()).Return(status.UnitWorkloadAgentStatuses{}, nil)

	err := s.service.CheckUnitStatusesReadyForMigration(c.Context())
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
	s.state.EXPECT().GetAllUnitWorkloadAgentStatuses(gomock.Any()).Return(fullStatus, nil)

	err := s.service.CheckUnitStatusesReadyForMigration(c.Context())
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
	s.state.EXPECT().GetAllUnitWorkloadAgentStatuses(gomock.Any()).Return(fullStatus, nil)

	err := s.service.CheckUnitStatusesReadyForMigration(c.Context())
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
	s.state.EXPECT().GetAllUnitWorkloadAgentStatuses(gomock.Any()).Return(fullStatus, nil)

	err := s.service.CheckUnitStatusesReadyForMigration(c.Context())
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
	s.state.EXPECT().GetAllUnitWorkloadAgentStatuses(gomock.Any()).Return(fullStatus, nil)

	err := s.service.CheckUnitStatusesReadyForMigration(c.Context())
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
	s.state.EXPECT().GetAllUnitWorkloadAgentStatuses(gomock.Any()).Return(fullStatus, nil)

	err := s.service.CheckUnitStatusesReadyForMigration(c.Context())
	c.Assert(err, tc.ErrorMatches, `(?m).*
- unit "foo/66\d" workload not active or viable
- unit "foo/66\d" workload not active or viable`)
}

func (s *serviceSuite) TestExportUnitStatusesEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAllUnitWorkloadAgentStatuses(gomock.Any()).Return(status.UnitWorkloadAgentStatuses{}, nil)

	workloadStatuses, agentStatuses, err := s.service.ExportUnitStatuses(c.Context())
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
	s.state.EXPECT().GetAllUnitWorkloadAgentStatuses(gomock.Any()).Return(fullStatus, nil)

	workloadStatuses, agentStatuses, err := s.service.ExportUnitStatuses(c.Context())
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

	s.state.EXPECT().GetAllApplicationStatuses(gomock.Any()).Return(map[string]status.StatusInfo[status.WorkloadStatusType]{}, nil)

	statuses, err := s.service.ExportApplicationStatuses(c.Context())
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
	s.state.EXPECT().GetAllApplicationStatuses(gomock.Any()).Return(statuses, nil)

	exported, err := s.service.ExportApplicationStatuses(c.Context())
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

	s.state.EXPECT().GetApplicationAndUnitStatuses(gomock.Any()).Return(
		map[string]status.Application{}, nil,
	)

	statuses, err := s.service.GetApplicationAndUnitStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statuses, tc.DeepEquals, map[string]Application{})
}

func (s *serviceSuite) TestGetApplicationAndUnitStatusesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationAndUnitStatuses(gomock.Any()).Return(
		map[string]status.Application{}, errors.Errorf("boom"),
	)

	_, err := s.service.GetApplicationAndUnitStatuses(c.Context())
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) TestGetApplicationAndUnitStatuses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	relationUUID := corerelationtesting.GenRelationUUID(c)

	s.state.EXPECT().GetApplicationAndUnitStatuses(gomock.Any()).Return(
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

	statuses, err := s.service.GetApplicationAndUnitStatuses(c.Context())
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

	s.state.EXPECT().GetApplicationAndUnitModelStatuses(gomock.Any()).Return(
		map[string]int{
			"foo": 2,
		}, nil,
	)

	statuses, err := s.service.GetApplicationAndUnitModelStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statuses, tc.DeepEquals, map[string]int{
		"foo": 2,
	})
}

func (s *serviceSuite) TestGetApplicationAndUnitStatusesInvalidLXDProfile(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetApplicationAndUnitStatuses(gomock.Any()).Return(
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

	_, err := s.service.GetApplicationAndUnitStatuses(c.Context())
	c.Assert(err, tc.ErrorMatches, `.*decoding LXD profile.*`)
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
	s.state.EXPECT().GetAllRelationStatuses(gomock.Any()).Return(stateRelationStatus, nil)

	// Act
	details, err := s.service.ExportRelationStatuses(c.Context())

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
	s.state.EXPECT().GetAllRelationStatuses(gomock.Any()).Return(nil, expectedError)

	// Act
	_, err := s.service.ExportRelationStatuses(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIs, expectedError)
}

func (s *serviceSuite) TestGetModelInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelStatusInfo := status.ModelStatusInfo{
		Type: model.IAAS,
	}

	s.state.EXPECT().GetModelStatusInfo(gomock.Any()).Return(modelStatusInfo, nil)

	modelStatusInfo, err := s.service.GetModelStatusInfo(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(modelStatusInfo, tc.DeepEquals, modelStatusInfo)
}

func (s *serviceSuite) TestGetModelInfoNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetModelStatusInfo(gomock.Any()).Return(status.ModelStatusInfo{}, modelerrors.NotFound)

	_, err := s.service.GetModelStatusInfo(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.statusHistory = &statusHistoryRecorder{}

	s.service = NewService(
		s.state,
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

func ptr[T any](v T) *T {
	return &v
}
