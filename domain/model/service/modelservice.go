// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/clock"

	"github.com/juju/juju/core/constraints"
	coremodel "github.com/juju/juju/core/model"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/model"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// ModelState is the model state required by this service. This is the model
// database state, not the controller state.
type ModelState interface {
	// Create creates a new model with all of its associated metadata.
	Create(context.Context, model.ReadOnlyModelCreationArgs) error

	// Delete deletes a model.
	Delete(context.Context, coremodel.UUID) error

	// GetModel returns the read only model information set in the database.
	GetModel(context.Context) (coremodel.ReadOnlyModel, error)

	// GetModelMetrics returns the model metrics information set in the
	// database.
	GetModelMetrics(context.Context) (coremodel.ModelMetrics, error)

	// ModelConstraints returns the current model constraints.
	ModelConstraints(context.Context) (constraints.Value, error)

	// SetModelConstraints replaces the current model constraints.
	SetModelConstraints(ctx context.Context, cons constraints.Value) error
}

// ControllerState is the controller state required by this service. This is the
// controller database, not the model state.
type ControllerState interface {
	// GetModel returns the model with the given UUID.
	GetModel(context.Context, coremodel.UUID) (coremodel.Model, error)

	// GetModelState returns the model state for the given model.
	// It returns [modelerrors.NotFound] if the model does not exist for the given UUID.
	GetModelState(context.Context, coremodel.UUID) (model.ModelState, error)
}

// ModelService defines a service for interacting with the underlying model
// state, as opposed to the controller state.
type ModelService struct {
	clock        clock.Clock
	modelID      coremodel.UUID
	controllerSt ControllerState
	modelSt      ModelState
}

// NewModelService returns a new Service for interacting with a models state.
func NewModelService(
	modelID coremodel.UUID,
	controllerSt ControllerState,
	modelSt ModelState,
) *ModelService {
	return &ModelService{
		modelID:      modelID,
		controllerSt: controllerSt,
		modelSt:      modelSt,
		clock:        clock.WallClock,
	}
}

// ModelConstraints returns the current model constraints.
func (s *ModelService) ModelConstraints(ctx context.Context) (constraints.Value, error) {
	return s.modelSt.ModelConstraints(ctx)
}

// SetModelConstraints replaces the current model constraints.
func (s *ModelService) SetModelConstraints(ctx context.Context, cons constraints.Value) error {
	return s.modelSt.SetModelConstraints(ctx, cons)
}

// GetModelInfo returns the readonly model information for the model in
// question.
func (s *ModelService) GetModelInfo(ctx context.Context) (coremodel.ReadOnlyModel, error) {
	return s.modelSt.GetModel(ctx)
}

// GetModelMetrics returns the model metrics information set in the
// database.
func (s *ModelService) GetModelMetrics(ctx context.Context) (coremodel.ModelMetrics, error) {
	return s.modelSt.GetModelMetrics(ctx)
}

// CreateModel is responsible for creating a new model within the model
// database.
//
// The following error types can be expected to be returned:
// - [modelerrors.AlreadyExists]: When the model uuid is already in use.
func (s *ModelService) CreateModel(
	ctx context.Context,
	controllerUUID uuid.UUID,
) error {
	m, err := s.controllerSt.GetModel(ctx, s.modelID)
	if err != nil {
		return err
	}

	args := model.ReadOnlyModelCreationArgs{
		UUID:            m.UUID,
		AgentVersion:    m.AgentVersion,
		ControllerUUID:  controllerUUID,
		Name:            m.Name,
		Type:            m.ModelType,
		Cloud:           m.Cloud,
		CloudType:       m.CloudType,
		CloudRegion:     m.CloudRegion,
		CredentialOwner: m.Credential.Owner,
		CredentialName:  m.Credential.Name,
	}

	return s.modelSt.Create(ctx, args)
}

// DeleteModel is responsible for removing a model from the system.
//
// The following error types can be expected to be returned:
// - [modelerrors.NotFound]: When the model does not exist.
func (s *ModelService) DeleteModel(
	ctx context.Context,
) error {
	return s.modelSt.Delete(ctx, s.modelID)
}

// GetStatus returns the current status of the model.
//
// The following error types can be expected to be returned:
// - [modelerrors.NotFound]: When the model does not exist.
func (s *ModelService) GetStatus(ctx context.Context) (model.StatusInfo, error) {
	modelState, err := s.controllerSt.GetModelState(ctx, s.modelID)
	if err != nil {
		return model.StatusInfo{}, errors.Capture(err)
	}

	now := s.clock.Now()
	if modelState.HasInvalidCloudCredential {
		return model.StatusInfo{
			Status:  corestatus.Suspended,
			Message: "suspended since cloud credential is not valid",
			Reason:  modelState.InvalidCloudCredentialReason,
			Since:   now,
		}, nil
	}
	if modelState.Destroying {
		return model.StatusInfo{
			Status:  corestatus.Destroying,
			Message: "the model is being destroyed",
			Since:   now,
		}, nil
	}
	if modelState.Migrating {
		return model.StatusInfo{
			Status:  corestatus.Busy,
			Message: "the model is being migrated",
			Since:   now,
		}, nil
	}

	return model.StatusInfo{
		Status: corestatus.Available,
		Since:  now,
	}, nil
}
