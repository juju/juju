// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/offer"
	"github.com/juju/juju/core/trace"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/internal/errors"
)

// OfferState describes retrieval and persistence methods for offer removal.
type OfferState interface {
	// OfferExists returns true if an offer exists with the input offer UUID.
	OfferExists(ctx context.Context, offerUUID string) (bool, error)

	// DeleteOffer removes an offer from the database completely.
	DeleteOffer(ctx context.Context, offerUUID string, force bool) error
}

type ControllerOfferState interface {
	// DeleteOfferAccess removes the access permissions for an offer.
	DeleteOfferAccess(ctx context.Context, offerUUID string) error
}

// RemoveOffer removes the offer from the model.
//
// NOTE: This method is different from the other removal methods, because offers
// do not have life, and their removal only cascade removes synthetic entities
// whose life does not matter either. This means we can simply attempt to remove
// the offer directly in this call.
func (s *Service) RemoveOffer(ctx context.Context, offerUUID offer.UUID, force bool) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	exists, err := s.modelState.OfferExists(ctx, offerUUID.String())
	if err != nil {
		return errors.Errorf("checking if offer %q exists: %w", offerUUID, err)
	} else if !exists {
		return errors.Errorf("offer %q does not exist", offerUUID).Add(crossmodelrelationerrors.OfferNotFound)
	}

	if err := s.modelState.DeleteOffer(ctx, offerUUID.String(), force); err != nil {
		return errors.Errorf("deleting offer %q: %w", offerUUID, err)
	}

	if err := s.controllerState.DeleteOfferAccess(ctx, offerUUID.String()); err != nil {
		// This call shouldn't fail, but if it does it is not fatal. Permissions
		// are indexed by offer UUID, so there is no risk leaving some orphaned
		s.logger.Warningf(ctx, "deleting offer access for offer %q: %v", offerUUID, err)
	}

	return nil
}
