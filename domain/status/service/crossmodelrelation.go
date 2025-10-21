// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/transform"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/offer"
	corerelation "github.com/juju/juju/core/relation"
	coreremoteapplication "github.com/juju/juju/core/remoteapplication"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/trace"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/domain/status"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

// CrossModelRelationState provides access to CMR related state methods.
type CrossModelRelationState interface {
	// GetRemoteApplicationOffererUUIDByName returns the UUID for the named for the remote
	// application wrapping the named application
	GetRemoteApplicationOffererUUIDByName(ctx context.Context, name string) (coreremoteapplication.UUID, error)

	// GetRemoteApplicationOffererStatuses returns the statuses of all remote
	// application offerers in the model, indexed by application name.
	GetRemoteApplicationOffererStatuses(ctx context.Context) (map[string]status.RemoteApplicationOfferer, error)

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

	now := s.clock.Now().UTC()

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
	appUUID, err := coreapplication.ParseUUID(uuid)
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

// GetRemoteApplicationOffererStatuses returns the statuses of all remote
// application offerers in the model, indexed by application name.
func (s *Service) GetRemoteApplicationOffererStatuses(ctx context.Context) (map[string]RemoteApplicationOfferer, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	res, err := s.modelState.GetRemoteApplicationOffererStatuses(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return decodeRemoteApplicationOffererStatuses(res)
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

func decodeRemoteApplicationOffererStatuses(statuses map[string]status.RemoteApplicationOfferer) (map[string]RemoteApplicationOfferer, error) {
	res := make(map[string]RemoteApplicationOfferer, len(statuses))
	for name, sts := range statuses {
		life, err := sts.Life.Value()
		if err != nil {
			return nil, errors.Errorf("decoding remote application offerer life: %w", err)
		}

		eps, err := transform.SliceOrErr(sts.Endpoints, func(ep status.Endpoint) (Endpoint, error) {
			role, err := decodeRole(ep.Role)
			if err != nil {
				return Endpoint{}, errors.Errorf("decoding remote application offerer role: %w", err)
			}
			return Endpoint{
				Name:      ep.Name,
				Role:      role,
				Interface: ep.Interface,
				Limit:     ep.Limit,
			}, nil
		})
		if err != nil {
			return nil, errors.Errorf("decoding remote application offerer endpoints: %w", err)
		}

		rels, err := transform.SliceOrErr(sts.Relations, corerelation.ParseUUID)
		if err != nil {
			return nil, errors.Errorf("decoding remote application offerer relations: %w", err)
		}

		appStatus, err := decodeApplicationStatus(sts.Status)
		if err != nil {
			return nil, errors.Errorf("decoding remote application offerer status: %w", err)
		}

		offerURL, err := crossmodel.ParseOfferURL(sts.OfferURL)
		if err != nil {
			return nil, errors.Errorf("parsing remote application offerer offer URL: %w", err)
		}

		res[name] = RemoteApplicationOfferer{
			Status:    appStatus,
			OfferURL:  offerURL,
			Life:      life,
			Endpoints: eps,
			Relations: rels,
		}
	}
	return res, nil
}

func decodeRole(role string) (internalcharm.RelationRole, error) {
	switch role {
	case "provider":
		return internalcharm.RoleProvider, nil
	case "requirer":
		return internalcharm.RoleRequirer, nil
	case "peer":
		return internalcharm.RolePeer, nil
	default:
		return "", errors.Errorf("unknown role %q", role)
	}
}
