// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/clock"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
)

// State represents a type for interacting with the underlying state for
// cross model relations.
type State struct {
	*domain.StateBase
	modelUUID string
	clock     clock.Clock
	logger    logger.Logger
}

// NewState returns a new state reference.
func NewState(factory database.TxnRunnerFactory, modelUUID model.UUID, clock clock.Clock, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		modelUUID: modelUUID.String(),
		clock:     clock,
		logger:    logger,
	}
}

func ptr[T any](v T) *T {
	return &v
}
