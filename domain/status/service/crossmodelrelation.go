// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/offer"
	coreremoteapplication "github.com/juju/juju/core/remoteapplication"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/trace"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
)

// CrossModelRelationState provides access to CMR related state methods.
type CrossModelRelationState interface {
	// GetRemoteApplicationOffererUUIDByName returns the UUID for the named for the remote
	// application wrapping the named application
	GetRemoteApplicationOffererUUIDByName(ctx context.Context, name string) (coreremoteapplication.UUID, error)

	// SetRemoteApplicationOffererStatus sets the status of the specified remote
	// application in the local model.
	SetRemoteApplicationOffererStatus(
		ctx context.Context,
		remoteApplicationUUID string,
		statusInfo status.StatusInfo[status.WorkloadStatusType],
	) error
}

// GetOfferStatus returns the status of the specified offer. This status shadows
// the status of the application that the offer belongs to, except in the case
// where the application or offer has been removed. Then a Terminated status is
// returned.
func (s *Service) GetOfferStatus(ctx context.Context, offerUUID offer.UUID) (corestatus.StatusInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := offerUUID.Validate(); err != nil {
		return corestatus.StatusInfo{}, errors.Capture(err)
	}

	now := s.clock.Now()

	uuid, err := s.modelState.GetApplicationUUIDForOffer(ctx, offerUUID.String())
	if errors.Is(err, crossmodelrelationerrors.OfferNotFound) {
		return corestatus.StatusInfo{
			Status:  corestatus.Terminated,
			Message: "offer has been removed",
			Since:   &now,
		}, nil
	} else if err != nil {
		return corestatus.StatusInfo{}, errors.Capture(err)
	}
	appUUID, err := coreapplication.ParseID(uuid)
	if err != nil {
		return corestatus.StatusInfo{}, errors.Capture(err)
	}

	appStatus, err := s.getApplicationDisplayStatus(ctx, appUUID)
	// It's possible between two state calls that the application is removed.
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return corestatus.StatusInfo{
			Status:  corestatus.Terminated,
			Message: "offer has been removed",
			Since:   &now,
		}, nil
	} else if err != nil {
		return corestatus.StatusInfo{}, errors.Capture(err)
	}

	return appStatus, nil
}

// SetRemoteApplicationOffererStatus sets the status of the specified remote
// application in the local model.
func (s *Service) SetRemoteApplicationOffererStatus(
	ctx context.Context,
	appName string,
	statusInfo corestatus.StatusInfo,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// This will also verify that the status is valid.
	encodedStatus, err := encodeWorkloadStatus(statusInfo)
	if err != nil {
		return errors.Errorf("encoding workload status: %w", err)
	}

	remoteAppUUID, err := s.modelState.GetRemoteApplicationOffererUUIDByName(ctx, appName)
	if err != nil {
		return errors.Capture(err)
	}

	if err := s.modelState.SetRemoteApplicationOffererStatus(ctx, remoteAppUUID.String(), encodedStatus); err != nil {
		return errors.Capture(err)
	}

	if err := s.statusHistory.RecordStatus(ctx, status.RemoteApplication.WithID(appName), statusInfo); err != nil {
		s.logger.Warningf(ctx, "recording setting remote application status history: %v", err)
	}

	return nil
}
