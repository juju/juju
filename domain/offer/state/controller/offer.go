// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/internal/uuid"
)

// State represents a type for interacting with the underlying state for offer access.
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

// CreateOfferAccess give the offer owner AdminAccess and EveryoneUserName
// ReadAccess for the provided offer.
func (st *State) CreateOfferAccess(context.Context, uuid.UUID, user.Name) error {
	return coreerrors.NotImplemented
}
