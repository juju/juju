// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain"
)

// State represents a type for interacting with the underlying state.
// Composes both application and charm state, so we can interact with both
// from the single state, whilst also keeping the concerns separate.
type State struct {
	*domain.StateBase
	*ApplicationState
	*CharmState
}

// NewState returns a new State for interacting with the underlying state.
func NewState(factory database.TxnRunnerFactory, logger logger.Logger) *State {
	base := domain.NewStateBase(factory)
	return &State{
		ApplicationState: NewApplicationState(base, logger),
		CharmState:       NewCharmState(base),
	}
}
