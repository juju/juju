// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/clock"

	"github.com/juju/juju/core/agentbinary"
	coreconstraints "github.com/juju/juju/core/constraints"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/semversion"
	corestatus "github.com/juju/juju/core/status"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modelagent"
	"github.com/juju/juju/environs"
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
	GetModelConstraints(context.Context) (constraints.Constraints, error)

	// SetModelConstraints sets the model constraints to the new values removing
	// any previously set values.
	// The following error types can be expected:
	// - [networkerrors.SpaceNotFound]: when a space constraint is set but the
	// space does not exist.
	// - [machineerrors.InvalidContainerType]: when the container type set on
	// the constraints is invalid.
	// - [modelerrors.NotFound]: when no model exists to set constraints for.
	SetModelConstraints(context.Context, constraints.Constraints) error

	// IsControllerModel returns true if the model is the controller model.
	// The following errors may be returned:
	// - [modelerrors.NotFound] when the model does not exist.
	IsControllerModel(context.Context) (bool, error)
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
	// HasBinariesForVersion will interrogate agent binaries available in the
	// system and return true or false if agent binaries exist for the provided
	// version.
	HasBinariesForVersion(semversion.Number) (bool, error)
}

// ModelResourcesProvider mirrors the [environs.ModelResources] interface that is
// used by the model service when creating a new model.
type ModelResourcesProvider interface {
	// ValidateProviderForNewModel is part of the [environs.ModelResources] interface.
	ValidateProviderForNewModel(ctx context.Context) error
	// CreateModelResources is part of the [environs.ModelResources] interface.
	CreateModelResources(context.Context, environs.CreateParams) error
}

// CloudInfoProvider instances provide a means to get
// the API version of the underlying cloud.
type CloudInfoProvider interface {
	// APIVersion returns the version info for provider's cloud.
	APIVersion() (string, error)
}

// ModelService defines a service for interacting with the underlying model
// state, as opposed to the controller state.
type ModelService struct {
	clock                 clock.Clock
	modelUUID             coremodel.UUID
	controllerSt          ControllerState
	modelSt               ModelState
	environProviderGetter EnvironVersionProviderFunc
	agentBinaryFinder     AgentBinaryFinder
}

// NewModelService creates a new instance of ModelService.
func NewModelService(
	modelUUID coremodel.UUID,
	controllerSt ControllerState,
	modelSt ModelState,
	environProviderGetter EnvironVersionProviderFunc,
	agentBinaryFinder AgentBinaryFinder,
) *ModelService {
	return &ModelService{
		modelUUID:             modelUUID,
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
func (s *ModelService) GetModelConstraints(ctx context.Context) (coreconstraints.Value, error) {
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

// GetModelCloudType returns the type of the cloud that is in use by this model.
func (s *ModelService) GetModelCloudType(ctx context.Context) (string, error) {
	return s.modelSt.GetModelCloudType(ctx)
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
func (s *ModelService) SetModelConstraints(ctx context.Context, cons coreconstraints.Value) error {
	modelCons := constraints.DecodeConstraints(cons)
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

// CreateModelForVersion is responsible for creating a new model within the
// model database, using the input agent version.
//
// The following error types can be expected to be returned:
// - [modelerrors.AlreadyExists]: When the model uuid is already in use.
func (s *ModelService) CreateModelForVersion(
	ctx context.Context,
	controllerUUID uuid.UUID,
	agentVersion semversion.Number,
	agentStream agentbinary.AgentStream,
) error {
	m, err := s.controllerSt.GetModel(ctx, s.modelUUID)
	if err != nil {
		return err
	}

	argAgentStream, err := modelagent.AgentStreamFromCoreAgentStream(agentStream)
	if err != nil {
		return errors.Errorf(
			"validating agent stream %q when creating new model: %w",
			agentStream, err,
		)
	}

	if err := validateAgentVersion(agentVersion, s.agentBinaryFinder); err != nil {
		return errors.Errorf("creating model %q with agent version %q: %w", m.Name, agentVersion, err)
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

		AgentStream: argAgentStream,
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
	return s.modelSt.Delete(ctx, s.modelUUID)
}

// GetStatus returns the current status of the model.
//
// The following error types can be expected to be returned:
// - [modelerrors.NotFound]: When the model does not exist.
func (s *ModelService) GetStatus(ctx context.Context) (model.StatusInfo, error) {
	modelState, err := s.controllerSt.GetModelState(ctx, s.modelUUID)
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

// GetEnvironVersion retrieves the version of the environment provider associated with the model.
//
// The following error types can be expected:
// - [modelerrors.NotFound]: Returned if the model does not exist.
func (s *ModelService) GetEnvironVersion(ctx context.Context) (int, error) {
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

// ProviderModelService defines a service for interacting with the underlying model
// state, as opposed to the controller state and the provider.
type ProviderModelService struct {
	ModelService
	providerGetter  providertracker.ProviderGetter[ModelResourcesProvider]
	cloudInfoGetter providertracker.ProviderGetter[CloudInfoProvider]
}

// NewProviderModelService returns a new Service for interacting with a models state.
func NewProviderModelService(
	modelUUID coremodel.UUID,
	controllerSt ControllerState,
	modelSt ModelState,
	environProviderGetter EnvironVersionProviderFunc,
	providerGetter providertracker.ProviderGetter[ModelResourcesProvider],
	cloudInfoGetter providertracker.ProviderGetter[CloudInfoProvider],
	agentBinaryFinder AgentBinaryFinder,
) *ProviderModelService {
	return &ProviderModelService{
		ModelService: ModelService{
			modelUUID:             modelUUID,
			controllerSt:          controllerSt,
			modelSt:               modelSt,
			clock:                 clock.WallClock,
			environProviderGetter: environProviderGetter,
			agentBinaryFinder:     agentBinaryFinder,
		},
		providerGetter:  providerGetter,
		cloudInfoGetter: cloudInfoGetter,
	}
}

// CloudAPIVersion returns the cloud API version for the model's cloud.
func (s *ProviderModelService) CloudAPIVersion(ctx context.Context) (string, error) {
	env, err := s.cloudInfoGetter(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		// Exit early if the provider does not support getting a cloud api version.
		return "", nil
	}
	if err != nil {
		return "", errors.Errorf("opening provider: %w", err)
	}
	return env.APIVersion()
}

// CreateModel is responsible for creating a new model within the model
// database, using the default agent version.
//
// The following error types can be expected to be returned:
// - [modelerrors.AlreadyExists]: When the model uuid is already in use.
func (s *ProviderModelService) CreateModel(
	ctx context.Context,
	controllerUUID uuid.UUID,
) error {
	defaultAgentVersion, defaultAgentStream := agentVersionSelector()
	if err := s.CreateModelForVersion(
		ctx,
		controllerUUID,
		defaultAgentVersion,
		defaultAgentStream,
	); err != nil {
		return errors.Capture(err)
	}

	env, err := s.providerGetter(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		// Exit early if the provider does not support creating model resources for the new model.
		return nil
	}
	if err != nil {
		return errors.Errorf("opening environ: %w", err)
	}

	if err := env.ValidateProviderForNewModel(ctx); err != nil {
		return errors.Errorf("creating model %q: %w", s.modelUUID, err)
	}
	if err := env.CreateModelResources(ctx, environs.CreateParams{ControllerUUID: controllerUUID.String()}); err != nil {
		// TODO: we should cleanup the model related data created above from database.
		return errors.Errorf("creating model resources for %q: %w", s.modelUUID, err)
	}
	return nil
}

// agentBinaryFinderFn is func type for the AgentBinaryFinder interface.
type agentBinaryFinderFn func(semversion.Number) (bool, error)

// HasBinariesForVersion implements AgentBinaryFinder by calling the receiver.
func (t agentBinaryFinderFn) HasBinariesForVersion(v semversion.Number) (bool, error) {
	return t(v)
}

// DefaultAgentBinaryFinder is a transition implementation of the agent binary
// finder that will return true for any version.
// This will be removed and replaced soon.
func DefaultAgentBinaryFinder() AgentBinaryFinder {
	return agentBinaryFinderFn(func(v semversion.Number) (bool, error) {
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
// agentFinder to make sure that we have an agent available for this version.
// If no agent binaries are available to support the agent version a
// [modelerrors.AgentVersionNotSupported] error is returned.
func validateAgentVersion(
	agentVersion semversion.Number,
	agentFinder AgentBinaryFinder,
) error {
	n := agentVersion.Compare(jujuversion.Current)
	switch {
	// agentVersion is greater than that of the current version.
	case n > 0:
		return errors.Errorf(
			"%w %q cannot be greater then the controller version %q",
			modelerrors.AgentVersionNotSupported,
			agentVersion.String(), jujuversion.Current.String())

	// agentVersion is less than that of the current version.
	case n < 0:
		has, err := agentFinder.HasBinariesForVersion(agentVersion)
		if err != nil {
			return errors.Errorf(
				"validating agent version %q for available tools: %w",
				agentVersion.String(), err)

		}
		if !has {
			return errors.Errorf(
				"%w %q no agent binaries found",
				modelerrors.AgentVersionNotSupported,
				agentVersion)

		}
	}

	return nil
}

// agentVersionSelector is used to find a suitable agent version and stream to
// use for newly created models. This is useful when creating new models where
// no specific version or stream has been requested.
func agentVersionSelector() (semversion.Number, agentbinary.AgentStream) {
	return jujuversion.Current, agentbinary.AgentStreamReleased
}

// EnvironVersionProvider defines a minimal subset of the EnvironProvider interface
// that focuses specifically on the provider's versioning capabilities.
type EnvironVersionProvider interface {
	// Version returns the version of the provider. This is recorded as the
	// environ version for each model, and used to identify which upgrade
	// operations to run when upgrading a model's environ.
	Version() int
}

// EnvironVersionProviderFunc describes a type that is able to return a
// [EnvironVersionProvider] for the specified cloud type. If no
// environ version provider exists for the supplied cloud type then a
// [coreerrors.NotFound] error is returned. If the cloud type provider does not support
// the EnvironVersionProvider interface then a [coreerrors.NotSupported] error is returned.
type EnvironVersionProviderFunc func(string) (EnvironVersionProvider, error)

// EnvironVersionProviderGetter returns a [EnvironVersionProviderFunc]
// for retrieving an EnvironVersionProvider
func EnvironVersionProviderGetter() EnvironVersionProviderFunc {
	return func(cloudType string) (EnvironVersionProvider, error) {
		environProvider, err := environs.GlobalProviderRegistry().Provider(cloudType)
		if errors.Is(err, coreerrors.NotFound) {
			return nil, errors.Errorf(
				"no environ version provider exists for cloud type %q", cloudType,
			).Add(coreerrors.NotFound)
		}

		environVersionProvider, supports := environProvider.(EnvironVersionProvider)
		if !supports {
			return nil, errors.Errorf(
				"environ version provider not supported for cloud type %q", cloudType,
			).Add(coreerrors.NotSupported)
		}

		return environVersionProvider, nil
	}
}

// IsControllerModel returns true if the model is the controller model.
// The following errors may be returned:
// - [modelerrors.NotFound] when the model does not exist.
func (s *ModelService) IsControllerModel(ctx context.Context) (bool, error) {
	return s.modelSt.IsControllerModel(ctx)
}
