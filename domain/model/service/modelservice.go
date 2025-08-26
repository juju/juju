// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/clock"

	"github.com/juju/juju/core/agentbinary"
	coreconstraints "github.com/juju/juju/core/constraints"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/semversion"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/trace"
	coreuser "github.com/juju/juju/core/user"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modelagent"
	"github.com/juju/juju/domain/storage"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/internal/errors"
	internalstorage "github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/uuid"
)

// AgentBinaryFinder represents a helper for establishing if agent binaries for
// a specific Juju version are available.
type AgentBinaryFinder interface {
	// HasBinariesForVersion will interrogate agent binaries available in the
	// system and return true or false if agent binaries exist for the provided
	// version.
	HasBinariesForVersion(semversion.Number) (bool, error)
}

// CloudInfoProvider instances provide a means to get
// the API version of the underlying cloud.
type CloudInfoProvider interface {
	// APIVersion returns the version info for provider's cloud.
	APIVersion() (string, error)
}

// ControllerState is the controller state required by this service. This is the
// controller database, not the model state.
type ControllerState interface {
	// GetModelSeedInformation returns information related to a model for the
	// purposes of seeding this information into other parts of a Juju controller.
	// This method is similar to [State.GetModel] but it allows for the returning of
	// information on models that are not activated yet.
	//
	// The following error types can be expected:
	// - [modelerrors.NotFound]: When the model is not found for the given uuid
	// regardless of the activated status.
	GetModelSeedInformation(context.Context, coremodel.UUID) (coremodel.ModelInfo, error)

	// GetModelState returns the model state for the given model.
	// It returns [modelerrors.NotFound] if the model does not exist for the given UUID.
	GetModelState(context.Context, coremodel.UUID) (model.ModelState, error)

	// GetModelSummary provides summary based information for the model
	// identified by the uuid. The information returned is intended to augment
	// the information that lives in the model state.
	// The following error types can be expected:
	// - [modelerrors.NotFound] when the model is not found for the given model
	// uuid.
	GetModelSummary(context.Context, coremodel.UUID) (model.ModelSummary, error)

	// GetUserModelSummary returns a summary of the model information that is
	// only available in the controller database from the perspective of the
	// user. This assumes that the user has access to the model.
	// The following error types can be expected:
	// - [modelerrors.NotFound] when the model is not found for the given model
	// uuid.
	// - [github.com/juju/juju/domain/access/errors.UserNotFound] when the user
	// is not found for the given user uuid.
	// - [github.com/juju/juju/domain/access/errors.AccessNotFound] when the
	// user does not have access to the model.
	GetUserModelSummary(context.Context, coreuser.UUID, coremodel.UUID) (model.UserModelSummary, error)

	// HasValidCredential returns true if the model has a valid credential.
	// The following errors may be returned:
	// - [modelerrors.NotFound] when the model no longer exists.
	HasValidCredential(context.Context, coremodel.UUID) (bool, error)
}

// ModelResourcesProvider mirrors the [environs.ModelResources] interface that is
// used by the model service when creating a new model.
type ModelResourcesProvider interface {
	// ValidateProviderForNewModel is part of the [environs.ModelResources] interface.
	ValidateProviderForNewModel(ctx context.Context) error
	// CreateModelResources is part of the [environs.ModelResources] interface.
	CreateModelResources(context.Context, environs.CreateParams) error
	// ConstraintsValidator returns a Validator instance which
	// is used to validate and merge constraints.
	ConstraintsValidator(ctx context.Context) (coreconstraints.Validator, error)
}

// ModelService defines a service for interacting with the underlying model
// state, as opposed to the controller state.
type ModelService struct {
	agentBinaryFinder     AgentBinaryFinder
	controllerSt          ControllerState
	clock                 clock.Clock
	environProviderGetter EnvironVersionProviderFunc
	modelSt               ModelState
	modelUUID             coremodel.UUID
}

// ModelState is the model state required by this service. This is the model
// database state, not the controller state.
type ModelState interface {
	// Create creates a new model with all of its associated metadata.
	Create(context.Context, model.ModelDetailArgs) error

	// Delete deletes a model.
	Delete(context.Context, coremodel.UUID) error

	// GetControllerUUID returns the controller uuid for the model.
	// It is expected that CreateModel has been called before reading this value
	// from the database.
	// The following error types can be expected:
	// - [modelerrors.NotFound] when no model has been created in the state
	// layer.
	GetControllerUUID(context.Context) (uuid.UUID, error)

	// GetModel returns the read only model information set in the database.
	GetModel(context.Context) (coremodel.ModelInfo, error)

	// GetModelInfoSummary returns a summary of the model information contained
	// in this database.
	// The following errors can be expected:
	// - [modelerrors.NotFound] if no model has been established in this model
	// database.
	GetModelInfoSummary(context.Context) (model.ModelInfoSummary, error)

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

	// GetModelType returns the [coremodel.ModelType] for the current model.
	// The following errors can be expected:
	// - [modelerrors.NotFound] when no read only model has been established in
	// the model's state layer.
	GetModelType(context.Context) (coremodel.ModelType, error)

	// CreateDefaultStoragePools is responsible for inserting a model's set of
	// default storage pools into the model. It is the responsibility of the
	// caller to make sure that no conflicts exist and the operation is
	// performed once.
	CreateDefaultStoragePools(
		context.Context, []model.CreateModelDefaultStoragePoolArg,
	) error

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

// RegionProvider instances provide a means to get the CloudSpec for this
// model from a provider which supports it.
type RegionProvider interface {
	// Region returns the necessary attributes to uniquely identify the model's
	// actual cloud deployment via a CloudSpec.
	Region() (simplestreams.CloudSpec, error)
}

// StorageProviderRegistryGetter represents a getter for returning the storage
// provider registry of the current model.
type StorageProviderRegistryGetter interface {
	// GetStorageRegistry returns the provider registry for the current model.
	GetStorageRegistry(context.Context) (internalstorage.ProviderRegistry, error)
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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.modelSt.GetModelCloudType(ctx)
}

// GetModelSummary returns a summary of the current model as a
// [coremodel.ModelSummary] type.
// The following error types can be expected:
// - [modelerrors.NotFound] when the model does not exist.
func (s *ModelService) GetModelSummary(
	ctx context.Context,
) (coremodel.ModelSummary, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	mSummary, err := s.controllerSt.GetModelSummary(ctx, s.modelUUID)
	if err != nil {
		return coremodel.ModelSummary{}, errors.Capture(err)
	}

	miSummary, err := s.modelSt.GetModelInfoSummary(ctx)
	if err != nil {
		return coremodel.ModelSummary{}, errors.Capture(err)
	}

	status := s.statusFromModelState(mSummary.State)
	return coremodel.ModelSummary{
		Name:           miSummary.Name,
		Qualifier:      miSummary.Qualifier,
		UUID:           miSummary.UUID,
		ModelType:      miSummary.ModelType,
		CloudName:      miSummary.CloudName,
		CloudType:      miSummary.CloudType,
		CloudRegion:    miSummary.CloudRegion,
		ControllerUUID: miSummary.ControllerUUID,
		IsController:   miSummary.IsController,
		Life:           mSummary.Life,
		AgentVersion:   miSummary.AgentVersion,
		Status: corestatus.StatusInfo{
			Status:  status.Status,
			Message: status.Message,
			Since:   &status.Since,
		},
		MachineCount: miSummary.MachineCount,
		CoreCount:    miSummary.CoreCount,
		UnitCount:    miSummary.UnitCount,
		// TODO (tlm): Fill out migration status when this information is
		// available in Dqlite.
		Migration: nil,
	}, nil
}

// GetUserModelSummary returns a summary of the current model from the provided
// user's perspective. This is similar to the [ModelService.GetModelSummary]
// method but it will return information that is specific to the user.
// The following error types can be expected:
// - [coreerrors.NotValid] when the user uuid is not valid.
// - [modelerrors.NotFound] when the model does not exist.
// - [github.com/juju/juju/domain/access/errors.UserNotFound] when the user
// is not found for the given user uuid.
// - [github.com/juju/juju/domain/access/errors.AccessNotFound] when the
// user does not have access to the model.
func (s *ModelService) GetUserModelSummary(
	ctx context.Context,
	userUUID coreuser.UUID,
) (coremodel.UserModelSummary, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := userUUID.Validate(); err != nil {
		return coremodel.UserModelSummary{}, errors.Errorf(
			"invalid user uuid: %w", err,
		)
	}

	userSummary, err := s.controllerSt.GetUserModelSummary(ctx, userUUID, s.modelUUID)
	if err != nil {
		return coremodel.UserModelSummary{}, errors.Capture(err)
	}

	miSummary, err := s.modelSt.GetModelInfoSummary(ctx)
	if err != nil {
		return coremodel.UserModelSummary{}, errors.Capture(err)
	}

	status := s.statusFromModelState(userSummary.State)
	return coremodel.UserModelSummary{
		ModelSummary: coremodel.ModelSummary{
			Name:           miSummary.Name,
			Qualifier:      miSummary.Qualifier,
			UUID:           miSummary.UUID,
			ModelType:      miSummary.ModelType,
			CloudName:      miSummary.CloudName,
			CloudType:      miSummary.CloudType,
			CloudRegion:    miSummary.CloudRegion,
			ControllerUUID: miSummary.ControllerUUID,
			IsController:   miSummary.IsController,
			Life:           userSummary.Life,
			AgentVersion:   miSummary.AgentVersion,
			Status: corestatus.StatusInfo{
				Status:  status.Status,
				Message: status.Message,
				Since:   &status.Since,
			},
			MachineCount: miSummary.MachineCount,
			CoreCount:    miSummary.CoreCount,
			UnitCount:    miSummary.UnitCount,
			Migration:    nil,
		},
		UserAccess:         userSummary.UserAccess,
		UserLastConnection: userSummary.UserLastConnection,
	}, nil
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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	modelCons := constraints.DecodeConstraints(cons)
	return s.modelSt.SetModelConstraints(ctx, modelCons)
}

// GetModelInfo returns the readonly model information for the model in
// question.
func (s *ModelService) GetModelInfo(ctx context.Context) (coremodel.ModelInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	return s.modelSt.GetModel(ctx)
}

// GetModelMetrics returns the model metrics information set in the
// database.
func (s *ModelService) GetModelMetrics(ctx context.Context) (coremodel.ModelMetrics, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	return s.modelSt.GetModelMetrics(ctx)
}

// GetModelType returns the [coremodel.ModelType] for the current model.
// The following errors can be expected:
// - [modelerrors.NotFound] when the model does not exist.
func (s *ModelService) GetModelType(ctx context.Context) (coremodel.ModelType, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	return s.modelSt.GetModelType(ctx)
}

// CreateModel is responsible for creating a new model within the model
// database, using the input agent version.
//
// The following error types can be expected to be returned:
// - [modelerrors.AlreadyExists] when the model uuid is already in use.
func (s *ModelService) CreateModel(
	ctx context.Context,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	defaultAgentVersion, defaultAgentStream := agentVersionSelector()
	return s.CreateModelWithAgentVersionStream(
		ctx, defaultAgentVersion, defaultAgentStream,
	)
}

// CreateModelWithAgentVersion is responsible for creating a new model within
// the model database using the specified agent version.
//
// The following error types can be expected to be returned:
// - [modelerrors.AlreadyExists] when the model uuid is already in use.
// - [modelerrors.AgentVersionNotSupported] when the agent version is not
// supported.
func (s *ModelService) CreateModelWithAgentVersion(
	ctx context.Context,
	agentVersion semversion.Number,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	_, defaultAgentStream := agentVersionSelector()
	return s.CreateModelWithAgentVersionStream(ctx, agentVersion, defaultAgentStream)
}

// CreateModelWithAgentVersionStream is responsible for creating a new model
// within the model database, using the input agent version and agent stream.
//
// The following error types can be expected to be returned:
// - [modelerrors.AlreadyExists] when the model uuid is already in use.
// - [coreerrors.NotValid] when the agent stream is not valid.
// - [modelerrors.AgentVersionNotSupported] when the agent version is not
// supported.
func (s *ModelService) CreateModelWithAgentVersionStream(
	ctx context.Context,
	agentVersion semversion.Number,
	agentStream agentbinary.AgentStream,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	m, err := s.controllerSt.GetModelSeedInformation(ctx, s.modelUUID)
	if err != nil {
		return err
	}

	argAgentStream, err := modelagent.AgentStreamFromCoreAgentStream(agentStream)
	if errors.Is(err, coreerrors.NotValid) {
		return errors.New(
			"agent stream %q is not a valid agent stream identifier for a model",
		).Add(modelerrors.AgentStreamNotValid)
	} else if err != nil {
		return errors.Errorf(
			"converting agent stream core type to domain type: %w", err,
		)
	}

	if err := validateAgentVersion(agentVersion, s.agentBinaryFinder); err != nil {
		return errors.Errorf("creating model %q with agent version %q: %w", m.Name, agentVersion, err)
	}

	args := model.ModelDetailArgs{
		UUID:            m.UUID,
		ControllerUUID:  m.ControllerUUID,
		Name:            m.Name,
		Qualifier:       m.Qualifier,
		Type:            m.Type,
		Cloud:           m.Cloud,
		CloudType:       m.CloudType,
		CloudRegion:     m.CloudRegion,
		CredentialOwner: m.CredentialOwner,
		CredentialName:  m.CredentialName,

		AgentStream: argAgentStream,
		// TODO (manadart 2024-01-13): Note that this comes from the arg.
		// It is not populated in the return from the controller state.
		// So that method should not return the core type.
		AgentVersion:       agentVersion,
		LatestAgentVersion: agentVersion,
	}
	return s.modelSt.Create(ctx, args)
}

// SeedDefaultStoragePools ensures that all of the default storage pools
// available to the model have been seeded for use. If the default storage pools
// already exist the function will return an error to the caller.
func (s *ProviderModelService) SeedDefaultStoragePools(
	ctx context.Context,
) error {
	modelStorageRegistry, err := s.storageProviderRegistryGetter.GetStorageRegistry(ctx)
	if err != nil {
		return errors.Errorf(
			"getting storage provider registry for model: %w", err,
		)
	}

	providerTypes, err := modelStorageRegistry.StorageProviderTypes()
	if err != nil {
		return errors.Errorf(
			"getting storage provider types for model storage registry: %w", err,
		)
	}

	poolArgs := make([]model.CreateModelDefaultStoragePoolArg, 0, len(providerTypes))
	for _, providerType := range providerTypes {
		registry, err := modelStorageRegistry.StorageProvider(providerType)
		if err != nil {
			return errors.Errorf(
				"getting storage provider %q from registry: %w",
				providerType, err,
			)
		}

		providerDefaultPools := registry.DefaultPools()
		for _, providerDefaultPool := range providerDefaultPools {
			uuid, err := storage.NewStoragePoolUUID()
			if err != nil {
				return errors.Errorf(
					"generating new default storage pool %q uuid: %w",
					providerDefaultPool.Name(), err,
				)
			}

			poolArgs = append(poolArgs, model.CreateModelDefaultStoragePoolArg{
				Attributes: transformStoragePoolAttributes(providerDefaultPool.Attrs()),
				Name:       providerDefaultPool.Name(),
				Origin:     storage.StoragePoolOriginProviderDefault,
				Type:       providerDefaultPool.Provider().String(),
				UUID:       uuid,
			})
		}
	}

	return s.modelSt.CreateDefaultStoragePools(ctx, poolArgs)
}

// transformStoragePoolAttributes exists to transform internal storage pool
// attribute representations from map[string]any to map[string]string.
//
// Within Juju there has never existed a definitive coerce function for this
// data. This is not ideal but it is what we have today.
func transformStoragePoolAttributes(attr map[string]any) map[string]string {
	rval := make(map[string]string, len(attr))
	for k, v := range attr {
		rval[k] = fmt.Sprint(v)
	}
	return rval
}

// DeleteModel is responsible for removing a model from the system.
//
// The following error types can be expected to be returned:
// - [modelerrors.NotFound]: When the model does not exist.
func (s *ModelService) DeleteModel(
	ctx context.Context,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	return s.modelSt.Delete(ctx, s.modelUUID)
}

// statusFromModelState is responsible for converting the a [model.ModelState]
// into a model status representation.
func (s *ModelService) statusFromModelState(
	statusState model.ModelState,
) model.StatusInfo {
	now := s.clock.Now()
	if statusState.HasInvalidCloudCredential {
		return model.StatusInfo{
			Status:  corestatus.Suspended,
			Message: "suspended since cloud credential is not valid",
			Reason:  statusState.InvalidCloudCredentialReason,
			Since:   now,
		}
	}
	if statusState.Destroying {
		return model.StatusInfo{
			Status:  corestatus.Destroying,
			Message: "the model is being destroyed",
			Since:   now,
		}
	}
	if statusState.Migrating {
		return model.StatusInfo{
			Status:  corestatus.Busy,
			Message: "the model is being migrated",
			Since:   now,
		}
	}

	return model.StatusInfo{
		Status: corestatus.Available,
		Since:  now,
	}
}

// GetEnvironVersion retrieves the version of the environment provider associated with the model.
//
// The following error types can be expected:
// - [modelerrors.NotFound]: Returned if the model does not exist.
func (s *ModelService) GetEnvironVersion(ctx context.Context) (int, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

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

// IsControllerModel returns true if the model is the controller model.
// The following errors may be returned:
// - [modelerrors.NotFound] when the model no longer exists.
func (s *ModelService) IsControllerModel(ctx context.Context) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	return s.modelSt.IsControllerModel(ctx)
}

// HasValidCredential returns true if the model has a valid credential.
// The following errors may be returned:
// - [modelerrors.NotFound] when the model no longer exists.
func (s *ModelService) HasValidCredential(ctx context.Context) (bool, error) {
	return s.controllerSt.HasValidCredential(ctx, s.modelUUID)
}

// ProviderModelService defines a service for interacting with the underlying model
// state, as opposed to the controller state and the provider.
type ProviderModelService struct {
	ModelService
	providerGetter                providertracker.ProviderGetter[ModelResourcesProvider]
	cloudInfoGetter               providertracker.ProviderGetter[CloudInfoProvider]
	environRegionGetter           providertracker.ProviderGetter[RegionProvider]
	storageProviderRegistryGetter StorageProviderRegistryGetter
}

// NewProviderModelService returns a new Service for interacting with a models state.
func NewProviderModelService(
	modelUUID coremodel.UUID,
	controllerSt ControllerState,
	modelSt ModelState,
	environProviderGetter EnvironVersionProviderFunc,
	providerGetter providertracker.ProviderGetter[ModelResourcesProvider],
	cloudInfoGetter providertracker.ProviderGetter[CloudInfoProvider],
	environRegionGetter providertracker.ProviderGetter[RegionProvider],
	storageProviderRegistryGetter StorageProviderRegistryGetter,
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
		providerGetter:                providerGetter,
		cloudInfoGetter:               cloudInfoGetter,
		environRegionGetter:           environRegionGetter,
		storageProviderRegistryGetter: storageProviderRegistryGetter,
	}
}

// CloudAPIVersion returns the cloud API version for the model's cloud.
func (s *ProviderModelService) CloudAPIVersion(ctx context.Context) (string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

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

// ResolveConstraints resolves the constraints against the models constraints,
// using the providers constraints validator. This will merge the incoming
// constraints with the model's constraints, and return the merged result.
func (s *ProviderModelService) ResolveConstraints(
	ctx context.Context,
	cons coreconstraints.Value,
) (coreconstraints.Value, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	modelCons, err := s.modelSt.GetModelConstraints(ctx)
	if err != nil {
		return coreconstraints.Value{}, errors.Errorf(
			"getting model constraints for model %q: %w", s.modelUUID, err,
		)
	}

	env, err := s.providerGetter(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		// Exit early if the provider does not support resolving constraints.
		return cons, nil
	} else if err != nil {
		return coreconstraints.Value{}, errors.Errorf("opening provider: %w", err)
	}

	validator, err := env.ConstraintsValidator(ctx)
	if err != nil {
		return coreconstraints.Value{}, errors.Errorf(
			"getting constraints validator for model %q: %w", s.modelUUID, err,
		)
	}

	return validator.Merge(constraints.EncodeConstraints(modelCons), cons)
}

// CreateModel is responsible for creating a new model within the model
// database. Upon creating the model any information required in the model's
// provider will be initialised.
//
// The following error types can be expected to be returned:
// - [modelerrors.AlreadyExists] when the model uuid is already in use.
func (s *ProviderModelService) CreateModel(
	ctx context.Context,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.ModelService.CreateModel(ctx); err != nil {
		return errors.Capture(err)
	}

	err := s.SeedDefaultStoragePools(ctx)
	if err != nil {
		return errors.Errorf(
			"seeding default storage pools into new model: %w", err,
		)
	}

	return s.createModelProviderResources(ctx)
}

// CreateModelWithAgentVersion is responsible for creating a new model within
// the model database using the specified agent version. Upon creating the model
// any information required in the model's provider will be initialised.
//
// The following error types can be expected to be returned:
// - [modelerrors.AlreadyExists] when the model uuid is already in use.
// - [modelerrors.AgentVersionNotSupported] when the agent version is not
// supported.
func (s *ProviderModelService) CreateModelWithAgentVersion(
	ctx context.Context,
	agentVersion semversion.Number,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.ModelService.CreateModelWithAgentVersion(ctx, agentVersion); err != nil {
		return errors.Capture(err)
	}

	err := s.SeedDefaultStoragePools(ctx)
	if err != nil {
		return errors.Errorf(
			"seeding default storage pools into new model: %w", err,
		)
	}

	return s.createModelProviderResources(ctx)
}

// CreateModelWithAgentVersionStream is responsible for creating a new model
// within the model database using the specified agent version and agent stream.
// Upon creating the model any information required in the model's provider
// will be initialised.
//
// The following error types can be expected to be returned:
// - [modelerrors.AlreadyExists] when the model uuid is already in use.
// - [modelerrors.AgentVersionNotSupported] when the agent version is not
// supported.
// - [coreerrors.NotValid] when the agent stream is not valid.
func (s *ProviderModelService) CreateModelWithAgentVersionStream(
	ctx context.Context,
	agentVersion semversion.Number,
	agentStream agentbinary.AgentStream,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.ModelService.CreateModelWithAgentVersionStream(
		ctx, agentVersion, agentStream,
	); err != nil {
		return errors.Capture(err)
	}

	err := s.SeedDefaultStoragePools(ctx)
	if err != nil {
		return errors.Errorf(
			"seeding default storage pools into new model: %w", err,
		)
	}

	return s.createModelProviderResources(ctx)
}

// createModelProviderResources is responsible for creating the model resources
// in the underlying provider after the model has been created.
func (s *ProviderModelService) createModelProviderResources(
	ctx context.Context,
) error {
	env, err := s.providerGetter(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		// Exit early if the provider does not support creating model resources for the new model.
		return nil
	}
	if err != nil {
		return errors.Errorf("opening environ: %w", err)
	}

	if err := env.ValidateProviderForNewModel(ctx); err != nil {
		return errors.Errorf("validating provider for model %q: %w", s.modelUUID, err)
	}

	controllerUUID, err := s.modelSt.GetControllerUUID(ctx)
	if err != nil {
		return errors.Errorf(
			"getting controller uuid for model %q to initialise provider: %w",
			s.modelUUID, err,
		)
	}

	if err := env.CreateModelResources(ctx, environs.CreateParams{ControllerUUID: controllerUUID.String()}); err != nil {
		// TODO: we should cleanup the model related data created above from database.
		return errors.Errorf("creating model provider resources for %q: %w", s.modelUUID, err)
	}

	return nil
}

// GetRegionCloudSpec returns a CloudSpec representing the cloud deployment of
// this model if supported by the provider. If not, an empty structure is
// returned with no error.
func (s *ProviderModelService) GetRegionCloudSpec(ctx context.Context) (simplestreams.CloudSpec, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	provider, err := s.environRegionGetter(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		return simplestreams.CloudSpec{}, nil
	} else if err != nil {
		return simplestreams.CloudSpec{}, errors.Errorf("getting provider: %w", err)
	}

	return provider.Region()
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
