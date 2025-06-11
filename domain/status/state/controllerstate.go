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

// ControllerState represents the state of a single model within the controller's context.
// It provides access to information specific to the model, scoped by its UUID, within the controller database.
type ControllerState struct {
	*domain.StateBase

	// modelUUID is the uuid of the model that the controller state is scoped to.
	// It ensures that only data related to the specified model is accessible from this state.
	modelUUID coremodel.UUID
}

// NewControllerState returns a new [ControllerState] for interacting with the underlying controller state.
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
// key aspects about the current model so that the model's status can be derived from this
// information. If the model no longer exists for the provided UUID then an error
// satisfying [modelerrors.NotFound] will be returned.
func (s *ControllerState) GetModelStatusContext(ctx context.Context) (status.ModelStatusContext, error) {
	// TODO: Implement this method to return the model status context from DB.
	return status.ModelStatusContext{}, nil
}
