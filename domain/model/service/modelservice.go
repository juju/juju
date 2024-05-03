// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/version/v2"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/model"
	"github.com/juju/juju/internal/uuid"
)

// ModelState is the model state required by this service. This is the model
// database state, not the controller state.
type ModelState interface {
	// AgentVersion is responsible for reporting the currently stored target
	// agent version for the model.
	AgentVersion(context.Context) (version.Number, error)

	// Create creates a new model with all of its associated metadata.
	Create(context.Context, model.ReadOnlyModelCreationArgs) error

	// Delete deletes a model.
	Delete(ctx context.Context, uuid coremodel.UUID) error

	// Model returns a read-only model for the given uuid.
	Model(ctx context.Context) (coremodel.ReadOnlyModel, error)
}

// ModelGetterState represents the state required for reading all model information.
type ModelGetterState interface {
	Get(context.Context, coremodel.UUID) (coremodel.Model, error)
}

// ModelService defines a service for interacting with the underlying model
// state, as opposed to the controller state.
type ModelService struct {
	modelGetterSt ModelGetterState
	st            ModelState
}

// NewModelService returns a new Service for interacting with a models state.
func NewModelService(modelGetterSt ModelGetterState, st ModelState) *ModelService {
	return &ModelService{
		modelGetterSt: modelGetterSt,
		st:            st,
	}
}

// AgentVersion returns the target agent version currently set for the
// model. If no agent version happens to be set for the model an error
// satisfying [errors.NotFound] will be returned.
func (s *ModelService) AgentVersion(ctx context.Context) (version.Number, error) {
	return s.st.AgentVersion(ctx)
}

// GetModelInfo returns the readonly model information for the model in
// question.
func (s *ModelService) GetModelInfo(ctx context.Context) (coremodel.ReadOnlyModel, error) {
	return s.st.Model(ctx)
}

// CreateModel is responsible for creating a new model within the model
// database.
//
// The following error types can be expected to be returned:
// - [modelerrors.AlreadyExists]: When the model uuid is already in use.
func (s *ModelService) CreateModel(
	ctx context.Context,
	id coremodel.UUID,
	controllerUUID uuid.UUID,
) error {
	if err := id.Validate(); err != nil {
		return err
	}

	m, err := s.modelGetterSt.Get(ctx, id)
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
		CloudRegion:     m.CloudRegion,
		CredentialOwner: m.Credential.Owner,
		CredentialName:  m.Credential.Name,
	}

	return s.st.Create(ctx, args)
}

// DeleteModel is responsible for removing a model from the system.
//
// The following error types can be expected to be returned:
// - [modelerrors.NotFound]: When the model does not exist.
func (s *ModelService) DeleteModel(
	ctx context.Context,
	uuid coremodel.UUID,
) error {
	return s.st.Delete(ctx, uuid)
}
