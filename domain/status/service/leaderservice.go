// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/clock"

	"github.com/juju/juju/core/leadership"
	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/logger"
	corerelation "github.com/juju/juju/core/relation"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/status"
	statuserrors "github.com/juju/juju/domain/status/errors"
	"github.com/juju/juju/internal/errors"
)

// LeadershipService provides the API for working with the statuses of applications
// and units, including the API handlers that require leadership checks.
type LeadershipService struct {
	*Service
	leaderEnsurer leadership.Ensurer
}

// NewLeadershipService returns a new leadership service reference wrapping the
// input state.
func NewLeadershipService(
	st State,
	leaderEnsurer leadership.Ensurer,
	clock clock.Clock,
	logger logger.Logger,
	statusHistory StatusHistory,
) *LeadershipService {
	return &LeadershipService{
		Service: NewService(
			st,
			clock,
			logger,
			statusHistory,
		),
		leaderEnsurer: leaderEnsurer,
	}
}

// SetApplicationStatusForUnitLeader sets the application status using the
// leader unit of the application. If the specified unit is not the leader of
// it's application and error satisfying [statuserrors.UnitNotLeader] is
// returned. If the unit is not found, an error satisfying
// [statuserrors.UnitNotFound] is returned.
func (s *LeadershipService) SetApplicationStatusForUnitLeader(
	ctx context.Context,
	unitName coreunit.Name,
	status corestatus.StatusInfo,
) error {
	if err := unitName.Validate(); err != nil {
		return errors.Errorf("unit name: %w", err)
	}

	// This will implicitly verify that the status is valid.
	encodedStatus, err := encodeWorkloadStatus(status)
	if err != nil {
		return errors.Errorf("encoding workload status: %w", err)
	}

	// This returns the UnitNotFound if we can't find the application. This
	// is because we're doing a reverse lookup from the unit to the application.
	// We can't return the application not found, as we're not looking up the
	// application directly.
	appID, appName, err := s.st.GetApplicationIDAndNameByUnitName(ctx, unitName)
	if err != nil {
		return errors.Capture(err)
	}

	err = s.leaderEnsurer.WithLeader(ctx, appName, unitName.String(), func(ctx context.Context) error {
		return s.st.SetApplicationStatus(ctx, appID, encodedStatus)
	})
	if errors.Is(err, corelease.ErrNotHeld) {
		return statuserrors.UnitNotLeader
	} else if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// GetApplicationAndUnitStatusesForUnitWithLeader returns the display status
// of the application the specified unit belongs to, and the workload statuses
// of all the units that belong to that application, indexed by unit name.
// If the specified unit is not the leader of it's application and error satisfying
// [statuserrors.UnitNotLeader] is returned. If no application is found for the
// unit name, an error satisfying [statuserrors.ApplicationNotFound] is returned.
func (s *LeadershipService) GetApplicationAndUnitStatusesForUnitWithLeader(
	ctx context.Context,
	unitName coreunit.Name,
) (
	applicationDisplayStatus corestatus.StatusInfo,
	unitWorkloadStatuses map[coreunit.Name]corestatus.StatusInfo,
	err error,
) {
	if err := unitName.Validate(); err != nil {
		return corestatus.StatusInfo{}, nil, errors.Errorf("unit name: %w", err)
	}

	appName := unitName.Application()
	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return corestatus.StatusInfo{}, nil, errors.Errorf("getting application id: %w", err)
	}

	var applicationStatus status.StatusInfo[status.WorkloadStatusType]
	var fullUnitStatuses status.FullUnitStatuses
	err = s.leaderEnsurer.WithLeader(ctx, appName, unitName.String(), func(ctx context.Context) error {
		var err error
		applicationStatus, err = s.st.GetApplicationStatus(ctx, appID)
		if err != nil {
			return errors.Errorf("getting application status: %w", err)
		}
		fullUnitStatuses, err = s.st.GetAllFullUnitStatusesForApplication(ctx, appID)
		if err != nil {
			return errors.Errorf("getting unit workload and container statuses")
		}
		return nil
	})
	if errors.Is(err, corelease.ErrNotHeld) {
		return corestatus.StatusInfo{}, nil, statuserrors.UnitNotLeader
	} else if err != nil {
		return corestatus.StatusInfo{}, nil, errors.Capture(err)
	}

	unitWorkloadStatuses = make(map[coreunit.Name]corestatus.StatusInfo, len(fullUnitStatuses))
	for unitName, fullStatus := range fullUnitStatuses {
		workloadStatus, err := decodeUnitWorkloadStatus(fullStatus.WorkloadStatus, fullStatus.Present)
		if err != nil {
			return corestatus.StatusInfo{}, nil, errors.Capture(err)
		}
		unitWorkloadStatuses[unitName] = workloadStatus
	}

	if applicationStatus.Status == status.WorkloadStatusUnset {
		applicationDisplayStatus, err = applicationDisplayStatusFromUnits(fullUnitStatuses)
		if err != nil {
			return corestatus.StatusInfo{}, nil, errors.Capture(err)
		}
	} else {
		applicationDisplayStatus, err = decodeApplicationStatus(applicationStatus)
		if err != nil {
			return corestatus.StatusInfo{}, nil, errors.Errorf("decoding application workload status: %w", err)
		}
	}

	return applicationDisplayStatus, unitWorkloadStatuses, nil
}

// SetRelationStatus sets the status of the relation to the status provided.
// Status may only be set by the application leader.
// It can return the following errors:
//   - [statuserrors.RelationNotFound] if the relation doesn't exist.
//   - [statuserrors.RelationStatusTransitionNotValid] if the current relation
//     status cannot transition to the new relation status. the relation does
//     not exist.
func (s *LeadershipService) SetRelationStatus(
	ctx context.Context,
	unitName coreunit.Name,
	relationUUID corerelation.UUID,
	info corestatus.StatusInfo,
) error {
	// Check that the time has been provided
	if info.Since == nil || info.Since.IsZero() {
		return errors.Errorf("invalid time: %v", info.Since)
	}

	// Get application name for leadership check.
	_, applicationName, err := s.st.GetApplicationIDAndNameByUnitName(ctx, unitName)
	if err != nil {
		return errors.Capture(err)
	}

	// Status can only be set by the leader unit.
	err = s.leaderEnsurer.WithLeader(ctx, applicationName, unitName.String(), func(ctx context.Context) error {
		// Encode status.
		relationStatus, err := encodeRelationStatus(info)
		if err != nil {
			return errors.Errorf("encoding relation status: %w", err)
		}

		return s.st.SetRelationStatus(ctx, relationUUID, relationStatus)
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}
