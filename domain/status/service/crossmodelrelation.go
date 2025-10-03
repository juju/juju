// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreremoteapplication "github.com/juju/juju/core/remoteapplication"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/trace"
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
