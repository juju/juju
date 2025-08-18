// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/crossmodel"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/domain/crossmodelrelation"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/domain/crossmodelrelation/internal"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// GetOfferUUID returns the uuid for the provided offer URL.
func (s *Service) GetOfferUUID(ctx context.Context, offerURL *crossmodel.OfferURL) (uuid.UUID, error) {
	return uuid.UUID{}, coreerrors.NotImplemented
}

// Offer updates an existing offer, or creates a new offer if it does not exist.
// Permissions are created for a new offer only.
func (s *Service) Offer(
	ctx context.Context,
	args crossmodelrelation.ApplicationOfferArgs,
) error {
	if err := args.Validate(); err != nil {
		return errors.Capture(err)
	}

	if args.OfferName == "" {
		args.OfferName = args.ApplicationName
	}

	offerUUID, err := uuid.NewUUID()
	if err != nil {
		return errors.Capture(err)
	}
	permissionUUID, err := uuid.NewUUID()
	if err != nil {
		return errors.Capture(err)
	}
	createArgs := internal.MakeCreateOfferArgs(args, offerUUID)

	// Attempt to update the offer, return if successful or an error other than
	// OfferNotFound is received.
	err = s.modelState.UpdateOffer(ctx, createArgs.OfferName, createArgs.Endpoints)
	if err == nil {
		return nil
	} else if !errors.Is(err, crossmodelrelationerrors.OfferNotFound) {
		return errors.Errorf("update offer: %w", err)
	}

	// Verify the owner exists, has not been removed, and
	// is not disabled before creating. Other users can
	// update an offer, such an admin.
	ownerUUID, err := s.controllerState.GetUserUUIDByName(ctx, args.OwnerName)
	if err != nil {
		return errors.Errorf("create offer: %w", err)
	}

	// The offer does not exist, create it.
	err = s.modelState.CreateOffer(ctx, createArgs)
	if err != nil {
		return errors.Errorf("create offer: %w", err)
	}

	err = s.controllerState.CreateOfferAccess(ctx, permissionUUID, offerUUID, ownerUUID)
	if err == nil {
		return nil
	}

	// If we fail to create offer access rows, delete the offer.
	deleteErr := s.modelState.DeleteFailedOffer(ctx, offerUUID)
	if deleteErr != nil {
		err = errors.Join(err, deleteErr)
	}
	err = errors.Errorf("creating access for offer %q: %w", args.OfferName, err)
	return errors.Capture(err)
}

// GetOffers returns offer details for all offers satisfying any of the
// provided filters.
func (s *Service) GetOffers(ctx context.Context, filters []crossmodelrelation.OfferFilter) ([]*crossmodelrelation.OfferDetails, error) {
	return nil, coreerrors.NotImplemented
}
