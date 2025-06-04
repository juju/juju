// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/status"
)

// ControllerState represents a type for interacting with the underlying model state.
type ControllerState struct {
	*domain.StateBase

	modelUUID coremodel.UUID
}

// NewControllerState returns a new ControllerState for interacting with the underlying model state.
func NewControllerState(
	factory database.TxnRunnerFactory,
	modelUUID coremodel.UUID,
) *ControllerState {
	return &ControllerState{
		StateBase: domain.NewStateBase(factory),
		modelUUID: modelUUID,
	}
}

// GetModelStatusContext is responsible for returning a set of boolean indicators for
// key aspects about a model so that a model's status can be derived from this
// information. If no model exists for the provided UUID then an error
// satisfying [modelerrors.NotFound] will be returned.
func (s *ControllerState) GetModelStatusContext(ctx context.Context) (status.ModelStatusContext, error) {
	// TODO: Implement this method to return the model status context from DB.
	return status.ModelStatusContext{}, nil
}
