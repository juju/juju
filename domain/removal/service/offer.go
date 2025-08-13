// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/uuid"
)

// RemoveOffer removes the offer from the model.
func (s *Service) RemoveOffer(ctx context.Context, offerUUID uuid.UUID, force bool) error {
	// TODO:
	// Remove offer with cascade
	// Remove permissions on offer
	return coreerrors.NotImplemented
}
