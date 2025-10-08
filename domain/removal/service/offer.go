// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/offer"
)

// RemoveOffer removes the offer from the model.
func (s *Service) RemoveOffer(ctx context.Context, offerUUID offer.UUID, force bool) error {
	// TODO:
	// Remove offer with cascade
	// Remove permissions on offer
	return coreerrors.NotImplemented
}
