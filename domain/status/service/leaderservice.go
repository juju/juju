// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/clock"

	"github.com/juju/juju/core/leadership"
	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	corerelation "github.com/juju/juju/core/relation"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	domainstatus "github.com/juju/juju/domain/status"
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
	controllerState ControllerState,
	leaderEnsurer leadership.Ensurer,
	modelUUID model.UUID,
	statusHistory StatusHistory,
	statusHistoryReaderFn StatusHistoryReaderFunc,
	clock clock.Clock,
	logger logger.Logger,
) *LeadershipService {
	return &LeadershipService{
		Service: NewService(
			st,
			controllerState,
			modelUUID,
			statusHistory,
			statusHistoryReaderFn,
			clock,
			logger,
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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return corestatus.StatusInfo{}, nil, errors.Errorf("unit name: %w", err)
	}

	appName := unitName.Application()
	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return corestatus.StatusInfo{}, nil, errors.Errorf("getting application id: %w", err)
	}

	var applicationStatus domainstatus.StatusInfo[domainstatus.WorkloadStatusType]
	var fullUnitStatuses domainstatus.FullUnitStatuses
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

	if applicationStatus.Status == domainstatus.WorkloadStatusUnset {
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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// Check that the time has been provided
	if info.Since == nil || info.Since.IsZero() {
		return errors.Errorf("invalid time: %v", info.Since)
	}

	// Status can only be set by the leader unit.
	if err := s.leaderEnsurer.WithLeader(ctx, unitName.Application(), unitName.String(), func(ctx context.Context) error {
		// Encode status.
		relationStatus, err := encodeRelationStatus(info)
		if err != nil {
			return errors.Errorf("encoding relation status: %w", err)
		}

		return s.st.SetRelationStatus(ctx, relationUUID, relationStatus)
	}); err != nil {
		return errors.Capture(err)
	}

	return nil
}

// GetModelStatus returns the current status of the model.
//
// The following error types can be expected to be returned:
// - [modelerrors.NotFound]: When the model does not exist.
func (s *Service) GetModelStatus(ctx context.Context) (domainstatus.ModelStatus, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	modelState, err := s.controllerState.GetModelState(ctx, s.modelUUID)
	if err != nil {
		return domainstatus.ModelStatus{}, errors.Capture(err)
	}
	return s.statusFromModelState(ctx, modelState), nil
}

// statusFromModelState is responsible for converting the a [model.ModelState]
// into a model status representation.
func (s *Service) statusFromModelState(
	ctx context.Context,
	statusState domainstatus.ModelState,
) domainstatus.ModelStatus {
	now := s.clock.Now()
	if statusState.HasInvalidCloudCredential {
		return domainstatus.ModelStatus{
			Status:  corestatus.Suspended,
			Message: "suspended since cloud credential is not valid",
			Reason:  statusState.InvalidCloudCredentialReason,
			Since:   now,
		}
	}
	if statusState.Destroying {
		return domainstatus.ModelStatus{
			Status:  corestatus.Destroying,
			Message: "the model is being destroyed",
			Since:   now,
		}
	}
	if statusState.Migrating {
		return domainstatus.ModelStatus{
			Status:  corestatus.Busy,
			Message: "the model is being migrated",
			Since:   now,
		}
	}

	return domainstatus.ModelStatus{
		Status: corestatus.Available,
		Since:  now,
	}
}
