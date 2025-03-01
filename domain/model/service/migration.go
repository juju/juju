// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/version/v2"

	coreconstraints "github.com/juju/juju/core/constraints"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/constraints"
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
	agentBinaryFinder     AgentBinaryFinder
}

// NewMigrationService creates a new instance of MigrationService.
func NewMigrationService(
	modelID coremodel.UUID,
	controllerSt ControllerState,
	modelSt ModelState,
	environProviderGetter EnvironVersionProviderFunc,
	agentBinaryFinder AgentBinaryFinder,
) *MigrationService {
	return &MigrationService{
		modelID:               modelID,
		controllerSt:          controllerSt,
		modelSt:               modelSt,
		clock:                 clock.WallClock,
		environProviderGetter: environProviderGetter,
		agentBinaryFinder:     agentBinaryFinder,
	}
}

// GetModelConstraints returns the current model constraints.
// It returns an error satisfying [modelerrors.NotFound] if the model does not
// exist.
// It returns an empty Value if the model does not have any constraints
// configured.
func (s *MigrationService) GetModelConstraints(ctx context.Context) (coreconstraints.Value, error) {
	cons, err := s.modelSt.GetModelConstraints(ctx)
	// If no constraints have been set for the model we return a zero value of
	// constraints. This is done so the state layer isn't making decisions on
	// what the caller of this service requires.
	if errors.Is(err, modelerrors.ConstraintsNotFound) {
		return coreconstraints.Value{}, nil
	} else if err != nil {
		return coreconstraints.Value{}, err
	}

	return constraints.EncodeConstraints(cons), nil
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
func (s *MigrationService) SetModelConstraints(ctx context.Context, cons coreconstraints.Value) error {
	modelCons := constraints.DecodeConstraints(cons)
	return s.modelSt.SetModelConstraints(ctx, modelCons)
}

// CreateModelForVersion is responsible for creating a new model within the
// model database, using the input agent version.
//
// The following error types can be expected to be returned:
// - [modelerrors.AlreadyExists]: When the model uuid is already in use.
func (s *MigrationService) CreateModelForVersion(
	ctx context.Context,
	controllerUUID uuid.UUID,
	agentVersion version.Number,
) error {
	return errors.Capture(createModelForVersion(
		ctx, s.modelID, controllerUUID, s.agentBinaryFinder, agentVersion, s.controllerSt, s.modelSt,
	))
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
