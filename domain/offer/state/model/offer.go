// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/offer"
	"github.com/juju/juju/internal/uuid"
)

// State represents a type for interacting with the underlying state for offers.
type State struct {
	*domain.StateBase
	logger logger.Logger
}

// NewState returns a new state reference.
func NewState(factory database.TxnRunnerFactory, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// CreateOffer creates an offer.
func (st *State) CreateOffer(
	context.Context,
	offer.ApplicationOfferArgs,
) (uuid.UUID, error) {
	return uuid.UUID{}, coreerrors.NotImplemented
}

// DeleteOffer deletes the provided offer.
func (st *State) DeleteOffer(
	context.Context,
	uuid.UUID,
) error {
	return coreerrors.NotImplemented
}

// UpdateOffer updates the endpoints of the given offer.
func (st *State) UpdateOffer(
	context.Context,
	offer.ApplicationOfferArgs,
) error {
	return coreerrors.NotImplemented
}
