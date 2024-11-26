// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

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

	// Model returns the read only model information set in the database.
	Model(context.Context) (coremodel.ReadOnlyModel, error)
}

// ControllerState is the controller state required by this service. This is the
// controller database, not the model state.
type ControllerState interface {
	// GetModel returns the model with the given UUID.
	GetModel(context.Context, coremodel.UUID) (coremodel.Model, error)

	// GetModelState returns the model state for the given model.
	GetModelState(context.Context, coremodel.UUID) (model.ModelState, error)
}

// ModelService defines a service for interacting with the underlying model
// state, as opposed to the controller state.
type ModelService struct {
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
	}
}

// GetModelInfo returns the readonly model information for the model in
// question.
func (s *ModelService) GetModelInfo(ctx context.Context) (coremodel.ReadOnlyModel, error) {
	return s.modelSt.Model(ctx)
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

// Status returns the current status of the model.
//
// The following error types can be expected to be returned:
// - [modelerrors.NotFound]: When the model does not exist.
func (s *ModelService) Status(ctx context.Context) (model.StatusInfo, error) {
	modelState, err := s.controllerSt.GetModelState(ctx, s.modelID)
	if err != nil {
		return model.StatusInfo{}, errors.Capture(err)
	}

	if modelState.HasInvalidCloudCredential {
		return model.StatusInfo{
			Status:  corestatus.Suspended,
			Message: "suspended since cloud credential is not valid",
			Reason:  modelState.InvalidCloudCredentialReason,
		}, nil
	}
	if modelState.Destroying {
		return model.StatusInfo{
			Status:  corestatus.Destroying,
			Message: "the model is being destroyed",
		}, nil
	}
	if modelState.Migrating {
		return model.StatusInfo{
			Status:  corestatus.Busy,
			Message: "the model is being migrated",
		}, nil
	}

	return model.StatusInfo{
		Status: corestatus.Available,
	}, nil
}
