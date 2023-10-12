// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/juju/domain/credential"
	"github.com/juju/juju/domain/model"
)

// State is the model state required by this service.
type State interface {
	// Create creates a new model with all of its associated metadata.
	Create(context.Context, model.UUID, model.ModelCreationArgs) error

	// Delete removes a model and all of it's associated data from Juju.
	Delete(context.Context, model.UUID) error

	// UpdateCredential updates a model's cloud credential.
	UpdateCredential(context.Context, model.UUID, credential.ID) error
}

// Service defines a service for interacting with the underlying state based
// information of a model.
type Service struct {
	st State
}

// NewService returns a new Service for interacting with a models state.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// CreateModel is responsible for creating a new model from start to finish with
// its associated metadata. The function will returned the created model's uuid.
// If the ModelCreationArgs do not have a credential name set then no cloud
// credential will be associated with the model.
// The following error types can be expected to be returned:
// - modelerrors.AlreadyExists: When the model uuid is already in use or a model
// with the same name and owner already exists.
// - errors.NotFound: When the cloud, cloud region, or credential do not exist.
func (s *Service) CreateModel(
	ctx context.Context,
	args model.ModelCreationArgs,
) (model.UUID, error) {
	if err := args.Validate(); err != nil {
		return model.UUID(""), err
	}

	uuid, err := model.NewUUID()
	if err != nil {
		return model.UUID(""), fmt.Errorf("generating new model uuid: %w", err)
	}

	return uuid, s.st.Create(ctx, uuid, args)
}

// DeleteModel is responsible for removing a model from Juju and all of it's
// associated metadata.
// - errors.NotValid: When the model uuid is not valid.
// - modelerrors.ModelNotFound: When the model does not exist.
func (s *Service) DeleteModel(
	ctx context.Context,
	uuid model.UUID,
) error {
	if err := uuid.Validate(); err != nil {
		return fmt.Errorf("delete model, uuid: %w", err)
	}

	return s.st.Delete(ctx, uuid)
}

// UpdateCredential is responsible for updating the cloud credential
// associated with a model. The cloud credential must be of the same cloud type
// as that of the model.
// The following error types can be expected to be returned:
// - modelerrors.ModelNotFound: When the model does not exist.
// - errors.NotFound: When the cloud or credential cannot be found.
// - errors.NotValid: When the cloud credential is not of the same cloud as the
// model or the model uuid is not valid.
func (s *Service) UpdateCredential(
	ctx context.Context,
	uuid model.UUID,
	id credential.ID,
) error {
	if err := uuid.Validate(); err != nil {
		return fmt.Errorf("updating cloud credential model uuid: %w", err)
	}
	if err := id.Validate(); err != nil {
		return fmt.Errorf("updating cloud credential: %w", err)
	}

	return s.st.UpdateCredential(ctx, uuid, id)
}
