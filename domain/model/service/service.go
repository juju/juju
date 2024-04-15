// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	jujuversion "github.com/juju/juju/version"
)

// ModelActivator describes a closure type that must be called after creating a
// new model to indicate that all model creation operations have been performed
// and the model is active within the controller.
//
// This type may return an error satisfying [modelerrors.AlreadyActivated] if
// the model in question has been activated already.
type ModelActivator func(context.Context) error

// ModelTypeState represents the state required for determining the type of model
// based on the cloud being set for it.
type ModelTypeState interface {
	// CloudType is responsible for reporting the type for a given cloud name.
	// If no cloud exists for the provided name then an error of
	// [clouderrors.NotFound] will be returned.
	CloudType(context.Context, string) (string, error)
}

// State is the model state required by this service.
type State interface {
	ModelTypeState

	// Create creates a new model with all of its associated metadata.
	Create(context.Context, coremodel.ModelType, model.ModelCreationArgs) error

	// Activate is responsible for setting a model as fully constructed and
	// indicates the final system state for the model is ready for use.
	// If no model exists for the provided id then a [modelerrors.NotFound] will be
	// returned. If the model has previously been activated a
	// [modelerrors.AlreadyActivated] error will be returned.
	Activate(ctx context.Context, uuid coremodel.UUID) error

	// Get returns the model associated with the provided uuid.
	Get(context.Context, coremodel.UUID) (coremodel.Model, error)

	// GetModelType returns the model type for a model with the provided uuid.
	GetModelType(context.Context, coremodel.UUID) (coremodel.ModelType, error)

	// Delete removes a model and all of it's associated data from Juju.
	Delete(context.Context, coremodel.UUID) error

	// ListAllModels returns all models registered in the controller. If no
	// models exist a zero value slice will be returned.
	ListAllModels(context.Context) ([]coremodel.Model, error)

	// ListModelIDs returns a list of all model UUIDs.
	ListModelIDs(context.Context) ([]coremodel.UUID, error)

	// ListModelsOwnedByUser returns a slice of models owned by the user
	// specified by user id. If no user or models are found an empty slice is
	// returned.
	ListModelsForUser(context.Context, coreuser.UUID) ([]coremodel.Model, error)

	// ModelCloudNameAndCredential returns the cloud name and credential id for a
	// model identified by the model name and the owner. If no model exists for
	// the provided name and user a [modelerrors.NotFound] error is returned.
	ModelCloudNameAndCredential(context.Context, string, string) (string, credential.Key, error)

	// UpdateCredential updates a model's cloud credential.
	UpdateCredential(context.Context, coremodel.UUID, credential.Key) error
}

// ModelDeleter is an interface for deleting models.
type ModelDeleter interface {
	// DeleteDB is responsible for removing a model from Juju and all of it's
	// associated metadata.
	DeleteDB(string) error
}

// Service defines a service for interacting with the underlying state based
// information of a model.
type Service struct {
	st                State
	modelDeleter      ModelDeleter
	agentBinaryFinder AgentBinaryFinder
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

// agentBinaryFinderFn is func type for the AgentBinaryFinder interface.
type agentBinaryFinderFn func(version.Number) (bool, error)

var (
	caasCloudTypes = []string{cloud.CloudTypeKubernetes}
)

func (t agentBinaryFinderFn) HasBinariesForVersion(v version.Number) (bool, error) {
	return t(v)
}

// NewService returns a new Service for interacting with a models state.
func NewService(st State, modelDeleter ModelDeleter, agentBinaryFinder AgentBinaryFinder) *Service {
	return &Service{
		st:                st,
		modelDeleter:      modelDeleter,
		agentBinaryFinder: agentBinaryFinder,
	}
}

// DefaultAgentBinaryFinder is a transition implementation of the agent binary
// finder that will false for any version that is not the current controller
// version.
// This will be removed and replaced soon.
func DefaultAgentBinaryFinder() AgentBinaryFinder {
	return agentBinaryFinderFn(func(v version.Number) (bool, error) {
		if v.Compare(jujuversion.Current) == 0 {
			return true, nil
		}
		return false, nil
	})
}

// agentVersionSelector is used to find a suitable agent version to use for
// newly created models. This is useful when creating new models where no
// specific version has been requested.
func agentVersionSelector() version.Number {
	return jujuversion.Current
}

// DefaultModelCloudNameAndCredential returns the default cloud name and
// credential that should be used for newly created models that haven't had
// either cloud or credential specified. If no default credential is available
// the zero value of [credential.ID] will be returned.
//
// The defaults that are sourced come from the controller's default model. If
// there is a no controller model a [modelerrors.NotFound] error will be
// returned.
func (s *Service) DefaultModelCloudNameAndCredential(
	ctx context.Context,
) (string, credential.Key, error) {
	cloudName, cred, err := s.st.ModelCloudNameAndCredential(
		ctx, coremodel.ControllerModelName, coremodel.ControllerModelOwnerUsername,
	)

	if err != nil {
		return "", credential.Key{}, fmt.Errorf("getting default model cloud name and credential: %w", err)
	}
	return cloudName, cred, nil
}

// CreateModel is responsible for creating a new model from start to finish with
// its associated metadata. The function will return the created model's uuid.
// If the ModelCreationArgs does not have a credential name set then no cloud
// credential will be associated with model.
//
// If the caller has not prescribed a specific agent version to use for the
// model the current controllers supported agent version will be used.
//
// Models created by this function must be activated using the returned
// ModelActivator.
//
// The following error types can be expected to be returned:
// - [modelerrors.AlreadyExists]: When the model uuid is already in use or a model
// with the same name and owner already exists.
// - [errors.NotFound]: When the cloud, cloud region, or credential do not exist.
// - [github.com/juju/juju/domain/access/errors.NotFound]: When the owner of the
// model can not be found.
// - [modelerrors.AgentVersionNotSupported]: When the prescribed agent version
// cannot be used with this controller.
func (s *Service) CreateModel(
	ctx context.Context,
	args model.ModelCreationArgs,
) (func(context.Context) error, error) {
	if err := args.Validate(); err != nil {
		return nil, err
	}

	modelType, err := ModelTypeForCloud(ctx, s.st, args.Cloud)
	if err != nil {
		return nil, fmt.Errorf(
			"determining model type when creating model %q: %w",
			args.Name, err,
		)
	}

	agentVersion := args.AgentVersion
	if args.AgentVersion == version.Zero {
		agentVersion = agentVersionSelector()
	}

	if err := validateAgentVersion(agentVersion, s.agentBinaryFinder); err != nil {
		return nil, fmt.Errorf(
			"creating model %q with agent version %q: %w",
			args.Name, agentVersion, err,
		)
	}

	args.AgentVersion = agentVersion

	activator := ModelActivator(func(ctx context.Context) error {
		return s.st.Activate(ctx, args.UUID)
	})

	return activator, s.st.Create(ctx, modelType, args)
}

// Model returns the model associated with the provided uuid.
// The following error types can be expected to be returned:
// - [modelerrors.ModelNotFound]: When the model does not exist.
func (s *Service) Model(ctx context.Context, uuid coremodel.UUID) (coremodel.Model, error) {
	if err := uuid.Validate(); err != nil {
		return coremodel.Model{}, fmt.Errorf("model uuid: %w", err)
	}

	return s.st.Get(ctx, uuid)
}

// ModelType returns the current model type based on the cloud name being used
// for the model.
func (s *Service) ModelType(ctx context.Context, uuid coremodel.UUID) (coremodel.ModelType, error) {
	if err := uuid.Validate(); err != nil {
		return coremodel.ModelType(""), fmt.Errorf("model type uuid: %w", err)
	}

	return s.st.GetModelType(ctx, uuid)
}

// DeleteModel is responsible for removing a model from Juju and all of it's
// associated metadata.
// - errors.NotValid: When the model uuid is not valid.
// - modelerrors.ModelNotFound: When the model does not exist.
func (s *Service) DeleteModel(
	ctx context.Context,
	uuid coremodel.UUID,
) error {
	if err := uuid.Validate(); err != nil {
		return fmt.Errorf("delete model, uuid: %w", err)
	}

	// Delete common items from the model. This helps to ensure that the
	// model is cleaned up correctly.
	if err := s.st.Delete(ctx, uuid); err != nil {
		return fmt.Errorf("delete model: %w", err)
	}

	// Delete the db completely from the system. Currently, this will remove
	// the db from the dbaccessor, but it will not drop the db (currently not
	// supported in dqlite). For now we do a best effort to remove all items
	// with in the db.
	if err := s.modelDeleter.DeleteDB(uuid.String()); err != nil {
		return fmt.Errorf("delete model: %w", err)
	}

	return nil
}

// ListModelIDs returns a list of all model UUIDs in the system that have not been
// deleted. This list does not represent one or more lifecycle states for
// models.
func (s *Service) ListModelIDs(ctx context.Context) ([]coremodel.UUID, error) {
	uuids, err := s.st.ListModelIDs(ctx)
	if err != nil {
		return nil, errors.Annotatef(err, "retrieving model list")
	}
	return uuids, nil
}

// ListAllModels  lists all models in the controller. If no models exist then
// an empty slice is returned.
func (s *Service) ListAllModels(ctx context.Context) ([]coremodel.Model, error) {
	return s.st.ListAllModels(ctx)
}

// ListModelsForUser lists the models that are either owned by the user or
// accessible  by the user specified by the user id. If no user or models exists
// an empty slice of models will be returned.
func (s *Service) ListModelsForUser(ctx context.Context, userID coreuser.UUID) ([]coremodel.Model, error) {
	if err := userID.Validate(); err != nil {
		return nil, fmt.Errorf("listing models owned by user: %w", err)
	}

	return s.st.ListModelsForUser(ctx, userID)
}

// ModelTypeForCloud is responsible returning the model type based on the cloud
// name being used for the model. If no cloud exists for the provided name then
// an error of [clouderrors.NotFound] will be returned.
func ModelTypeForCloud(
	ctx context.Context,
	state ModelTypeState,
	cloudName string,
) (coremodel.ModelType, error) {
	cloudType, err := state.CloudType(ctx, cloudName)
	if err != nil {
		return coremodel.ModelType(""), fmt.Errorf("determining model type from cloud: %w", err)
	}

	if set.NewStrings(caasCloudTypes...).Contains(cloudType) {
		return coremodel.CAAS, nil
	}
	return coremodel.IAAS, nil
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
	uuid coremodel.UUID,
	key credential.Key,
) error {
	if err := uuid.Validate(); err != nil {
		return fmt.Errorf("updating cloud credential model uuid: %w", err)
	}
	if err := key.Validate(); err != nil {
		return fmt.Errorf("updating cloud credential: %w", err)
	}

	return s.st.UpdateCredential(ctx, uuid, key)
}

// validateAgentVersion is responsible for checking that the agent version that
// is about to be chosen for a model is valid for use.
//
// If the agent version is equal to that of the currently running controller
// then this will be allowed.
//
// If the agent version is greater then that of the currently running controller
// then a [modelerrors.AgentVersionNotSupported] error is returned as
// we can't run a agent version that is greater then that of a controller.
//
// If the agent version is less then that of the current controller we use the
// toolFinder to make sure that we have tools available for this version. If no
// tools are available to support the agent version a
// [modelerrors.AgentVersionNotSupported] error is returned.
func validateAgentVersion(
	agentVersion version.Number,
	agentFinder AgentBinaryFinder,
) error {
	n := agentVersion.Compare(jujuversion.Current)
	switch {
	// agentVersion is greater then that of the current version.
	case n > 0:
		return fmt.Errorf(
			"%w %q cannot be greater then the controller version %q",
			modelerrors.AgentVersionNotSupported,
			agentVersion.String(), jujuversion.Current.String(),
		)
	// agentVersion is less then that of the current version.
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
