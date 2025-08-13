// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/crossmodel"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/offer"
	offererrors "github.com/juju/juju/domain/offer/errors"
	"github.com/juju/juju/domain/offer/internal"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// ModelDBState describes retrieval and persistence methods for offers
// in the model database..
type ModelDBState interface {
	// CreateOffer creates an offer and links the endpoints to it.
	CreateOffer(
		context.Context,
		internal.CreateOfferArgs,
	) error

	// DeleteFailedOffer deletes the provided offer, used after adding
	// permissions failed. Assumes that the offer is never used, no
	// checking of relations is required.
	DeleteFailedOffer(
		context.Context,
		uuid.UUID,
	) error

	// UpdateOffer updates the endpoints of the given offer.
	UpdateOffer(
		ctx context.Context,
		offerName string,
		offerEndpoints []string,
	) error
}

// ControllerDBState describes retrieval and persistence methods for offer
// access in the controller database.
type ControllerDBState interface {
	// CreateOfferAccess give the offer owner AdminAccess and EveryoneUserName
	// ReadAccess for the provided offer.
	CreateOfferAccess(
		ctx context.Context,
		permissionUUID, offerUUID, ownerUUID uuid.UUID,
	) error

	// GetUserUUIDByName returns the UUID of the user provided exists, has not
	// been removed and is not disabled.
	GetUserUUIDByName(ctx context.Context, name user.Name) (uuid.UUID, error)
}

// Service provides the API for working with offers.
type Service struct {
	controllerState ControllerDBState
	modelState      ModelDBState
	logger          logger.Logger
}

// NewService returns a new service reference wrapping the input state.
func NewService(
	controllerState ControllerDBState,
	modelState ModelDBState,
	logger logger.Logger,
) *Service {
	return &Service{
		controllerState: controllerState,
		modelState:      modelState,
		logger:          logger,
	}
}

// GetOfferUUID returns the uuid for the provided offer URL.
func (s *Service) GetOfferUUID(ctx context.Context, offerURL *crossmodel.OfferURL) (uuid.UUID, error) {
	return uuid.UUID{}, coreerrors.NotImplemented
}

// Offer updates an existing offer, or creates a new offer if it does not exist.
// Permissions are created for a new offer only.
func (s *Service) Offer(
	ctx context.Context,
	args offer.ApplicationOfferArgs,
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
	} else if !errors.Is(err, offererrors.OfferNotFound) {
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
func (s *Service) GetOffers(ctx context.Context, filters []offer.OfferFilter) ([]*offer.OfferDetails, error) {
	return nil, coreerrors.NotImplemented
}
