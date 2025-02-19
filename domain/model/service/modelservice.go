// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/clock"
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/constraints"
	coremodel "github.com/juju/juju/core/model"
	corestatus "github.com/juju/juju/core/status"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// ModelState is the model state required by this service. This is the model
// database state, not the controller state.
type ModelState interface {
	// Create creates a new model with all of its associated metadata.
	Create(context.Context, model.ModelDetailArgs) error

	// Delete deletes a model.
	Delete(context.Context, coremodel.UUID) error

	// GetModel returns the read only model information set in the database.
	GetModel(context.Context) (coremodel.ModelInfo, error)

	// GetModelMetrics returns the model metrics information set in the
	// database.
	GetModelMetrics(context.Context) (coremodel.ModelMetrics, error)

	// GetModelCloudType returns the model cloud type set in the database.
	GetModelCloudType(context.Context) (string, error)

	// GetModelConstraints returns the currently set constraints for the model.
	// The following error types can be expected:
	// - [modelerrors.NotFound]: when no model exists to set constraints for.
	// - [modelerrors.ConstraintsNotFound]: when no model constraints have been
	// set for the model.
	GetModelConstraints(context.Context) (model.Constraints, error)

	// SetModelConstraints sets the model constraints to the new values removing
	// any previously set values.
	// The following error types can be expected:
	// - [networkerrors.SpaceNotFound]: when a space constraint is set but the
	// space does not exist.
	// - [machineerrors.InvalidContainerType]: when the container type set on
	// the constraints is invalid.
	// - [modelerrors.NotFound]: when no model exists to set constraints for.
	SetModelConstraints(context.Context, model.Constraints) error
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

// AgentBinaryFinder represents a helper for establishing if agent binaries for
// a specific Juju version are available.
type AgentBinaryFinder interface {
	// HasBinariesForVersion will interrogate the tools available in the system
	// and return true or false if agent binaries exist for the provided
	// version. Any errors finding the requested binaries will be returned
	// through error.
	HasBinariesForVersion(version.Number) (bool, error)
}

// ModelService defines a service for interacting with the underlying model
// state, as opposed to the controller state.
type ModelService struct {
	clock                 clock.Clock
	modelID               coremodel.UUID
	controllerSt          ControllerState
	modelSt               ModelState
	environProviderGetter EnvironVersionProviderFunc
	agentBinaryFinder     AgentBinaryFinder
}

// NewModelService returns a new Service for interacting with a models state.
func NewModelService(
	modelID coremodel.UUID,
	controllerSt ControllerState,
	modelSt ModelState,
	environProviderGetter EnvironVersionProviderFunc,
	agentBinaryFinder AgentBinaryFinder,
) *ModelService {
	return &ModelService{
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
func (s *ModelService) GetModelConstraints(ctx context.Context) (constraints.Value, error) {
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
func (s *ModelService) SetModelConstraints(ctx context.Context, cons constraints.Value) error {
	modelCons := model.FromCoreConstraints(cons)

	return s.modelSt.SetModelConstraints(ctx, modelCons)
}

// GetModelInfo returns the readonly model information for the model in
// question.
func (s *ModelService) GetModelInfo(ctx context.Context) (coremodel.ModelInfo, error) {
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
	agentVersion version.Number,
) error {
	if agentVersion == version.Zero {
		agentVersion = agentVersionSelector()
	}

	m, err := s.controllerSt.GetModel(ctx, s.modelID)
	if err != nil {
		return err
	}

	if err := validateAgentVersion(agentVersion, s.agentBinaryFinder); err != nil {
		return fmt.Errorf("creating model %q with agent version %q: %w", m.Name, agentVersion, err)
	}

	args := model.ModelDetailArgs{
		UUID:            m.UUID,
		ControllerUUID:  controllerUUID,
		Name:            m.Name,
		Type:            m.ModelType,
		Cloud:           m.Cloud,
		CloudType:       m.CloudType,
		CloudRegion:     m.CloudRegion,
		CredentialOwner: m.Credential.Owner,
		CredentialName:  m.Credential.Name,

		// TODO (manadart 2024-01-13): Note that this comes from the arg.
		// It is not populated in the return from the controller state.
		// So that method should not return the core type.
		AgentVersion: agentVersion,
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

// agentBinaryFinderFn is func type for the AgentBinaryFinder interface.
type agentBinaryFinderFn func(version.Number) (bool, error)

func (t agentBinaryFinderFn) HasBinariesForVersion(v version.Number) (bool, error) {
	return t(v)
}

// DefaultAgentBinaryFinder is a transition implementation of the agent binary
// finder that will true for any version.
// This will be removed and replaced soon.
func DefaultAgentBinaryFinder() AgentBinaryFinder {
	return agentBinaryFinderFn(func(v version.Number) (bool, error) {
		// This is a temporary implementation that will be replaced. We need
		// to ensure that we always return true for now, so that we can be
		// sure that the 3.6 LTS release will work with the controller.
		return true, nil
	})
}

// validateAgentVersion is responsible for checking that the agent version that
// is about to be chosen for a model is valid for use.
//
// If the agent version is equal to that of the currently running controller
// then this will be allowed.
//
// If the agent version is greater than that of the currently running controller
// then a [modelerrors.AgentVersionNotSupported] error is returned as
// we can't run an agent version that is greater than that of a controller.
//
// If the agent version is less than that of the current controller we use the
// toolFinder to make sure that we have tools available for this version. If no
// tools are available to support the agent version a
// [modelerrors.AgentVersionNotSupported] error is returned.
func validateAgentVersion(
	agentVersion version.Number,
	agentFinder AgentBinaryFinder,
) error {
	n := agentVersion.Compare(jujuversion.Current)
	switch {
	// agentVersion is greater than that of the current version.
	case n > 0:
		return fmt.Errorf(
			"%w %q cannot be greater then the controller version %q",
			modelerrors.AgentVersionNotSupported,
			agentVersion.String(), jujuversion.Current.String(),
		)
	// agentVersion is less than that of the current version.
	case n < 0:
		has, err := agentFinder.HasBinariesForVersion(agentVersion)
		if err != nil {
			return fmt.Errorf(
				"validating agent version %q for available tools: %w",
				agentVersion.String(), err,
			)
		}
		if !has {
			return fmt.Errorf(
				"%w %q no agent binaries found",
				modelerrors.AgentVersionNotSupported,
				agentVersion,
			)
		}
	}

	return nil
}

// agentVersionSelector is used to find a suitable agent version to use for
// newly created models. This is useful when creating new models where no
// specific version has been requested.
func agentVersionSelector() version.Number {
	return jujuversion.Current
}
