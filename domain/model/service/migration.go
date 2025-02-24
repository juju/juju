// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/clock"

	"github.com/juju/juju/core/constraints"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// MigrationService defines a service for model migration operations.
type MigrationService struct {
	clock                 clock.Clock
	modelID               coremodel.UUID
	controllerSt          ControllerState
	modelSt               ModelState
	environProviderGetter EnvironVersionProviderFunc
}

// NewMigrationService creates a new instance of MigrationService.
func NewMigrationService(
	modelID coremodel.UUID,
	controllerSt ControllerState,
	modelSt ModelState,
	environProviderGetter EnvironVersionProviderFunc,
) *MigrationService {
	return &MigrationService{
		modelID:               modelID,
		controllerSt:          controllerSt,
		modelSt:               modelSt,
		clock:                 clock.WallClock,
		environProviderGetter: environProviderGetter,
	}
}

// GetModelConstraints returns the current model constraints.
// It returns an error satisfying [modelerrors.NotFound] if the model does not
// exist.
// It returns an empty Value if the model does not have any constraints
// configured.
func (s *MigrationService) GetModelConstraints(ctx context.Context) (constraints.Value, error) {
	cons, err := s.modelSt.GetModelConstraints(ctx)
	// If no constraints have been set for the model we return a zero value of
	// constraints. This is done so the state layer isn't making decisions on
	// what the caller of this service requires.
	if errors.Is(err, modelerrors.ConstraintsNotFound) {
		return constraints.Value{}, nil
	} else if err != nil {
		return constraints.Value{}, err
	}

	return model.ToCoreConstraints(cons), nil
}

// SetModelConstraints sets the model constraints to the new values removing
// any previously set constraints.
//
// The following error types can be expected:
// - [modelerrors.NotFound]: when the model does not exist
// - [github.com/juju/juju/domain/network/errors.SpaceNotFound]: when the space
// being set in the model constraint doesn't exist.
// - [github.com/juju/juju/domain/machine/errors.InvalidContainerType]: when
// the container type being set in the model constraint isn't valid.
func (s *MigrationService) SetModelConstraints(ctx context.Context, cons constraints.Value) error {
	modelCons := model.FromCoreConstraints(cons)
	return s.modelSt.SetModelConstraints(ctx, modelCons)
}

// CreateModel is responsible for creating a new model within the model
// database.
//
// The following error types can be expected to be returned:
// - [modelerrors.AlreadyExists]: When the model uuid is already in use.
func (s *MigrationService) CreateModel(
	ctx context.Context,
	controllerUUID uuid.UUID,
) error {
	m, err := s.controllerSt.GetModel(ctx, s.modelID)
	if err != nil {
		return err
	}

	args := model.ModelDetailArgs{
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
func (s *MigrationService) DeleteModel(
	ctx context.Context,
) error {
	return s.modelSt.Delete(ctx, s.modelID)
}

// GetEnvironVersion retrieves the version of the environment provider associated with the model.
//
// The following error types can be expected:
// - [modelerrors.NotFound]: Returned if the model does not exist.
func (s *MigrationService) GetEnvironVersion(ctx context.Context) (int, error) {
	modelCloudType, err := s.modelSt.GetModelCloudType(ctx)
	if err != nil {
		return 0, errors.Errorf(
			"getting model cloud type from state: %w", err,
		)
	}

	envProvider, err := s.environProviderGetter(modelCloudType)
	if err != nil {
		return 0, errors.Errorf(
			"getting environment provider for cloud type %q: %w", modelCloudType, err,
		)
	}

	return envProvider.Version(), nil
}
