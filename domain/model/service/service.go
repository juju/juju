// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/version/v2"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/credential"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	jujuversion "github.com/juju/juju/version"
)

// State is the model state required by this service.
type State interface {
	// Create creates a new model with all of its associated metadata.
	Create(context.Context, coremodel.UUID, model.ModelCreationArgs) error

	// Delete removes a model and all of it's associated data from Juju.
	Delete(context.Context, coremodel.UUID) error

	// List returns a list of all model UUIDs.
	List(context.Context) ([]coremodel.UUID, error)

	// Get returns the model with the provided uuid.
	Get(context.Context, coremodel.UUID) (*model.Model, error)

	// UpdateCredential updates a model's cloud credential.
	UpdateCredential(context.Context, coremodel.UUID, credential.ID) error

	// SetSecretBackend sets the secret backend for a model.
	SetSecretBackend(ctx context.Context, modelUUID coremodel.UUID, backendName string) error

	// GetSecretBackend returns the secret backend for a model.
	GetSecretBackend(ctx context.Context, modelUUID coremodel.UUID) (model.SecretBackendIdentifier, error)
}

// Service defines a service for interacting with the underlying state based
// information of a model.
type Service struct {
	st                State
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

func (t agentBinaryFinderFn) HasBinariesForVersion(v version.Number) (bool, error) {
	return t(v)
}

// NewService returns a new Service for interacting with a models state.
func NewService(st State, agentBinaryFinder AgentBinaryFinder) *Service {
	return &Service{
		st:                st,
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

// CreateModel is responsible for creating a new model from start to finish with
// its associated metadata. The function will returned the created model's uuid.
// If the ModelCreationArgs do not have a credential name set then no cloud
// credential will be associated with the model.
//
// If the caller has not prescribed a spefici agent version to use for the model
// the current controllers supported agent version will be used.

// The following error types can be expected to be returned:
// - [modelerrors.AlreadyExists]: When the model uuid is already in use or a model
// with the same name and owner already exists.
// - [errors.NotFound]: When the cloud, cloud region, or credential do not exist.
// - [github.com/juju/juju/domain/user/errors.NotFound]: When the owner of the
// - [modelerrors.AgentVersionNotSupported]: When the prescribed agent version
// cannot be used with this controller.
// mode cannot be found.
func (s *Service) CreateModel(
	ctx context.Context,
	args model.ModelCreationArgs,
) (coremodel.UUID, error) {
	if err := args.Validate(); err != nil {
		return coremodel.UUID(""), err
	}

	agentVersion := args.AgentVersion
	if args.AgentVersion == version.Zero {
		agentVersion = agentVersionSelector()
	}

	if err := validateAgentVersion(agentVersion, s.agentBinaryFinder); err != nil {
		return coremodel.UUID(""), fmt.Errorf(
			"creating model %q with agent version %q: %w",
			args.Name, agentVersion, err,
		)
	}

	args.AgentVersion = agentVersion
	uuid := args.UUID
	if uuid == "" {
		var err error
		uuid, err = coremodel.NewUUID()
		if err != nil {
			return coremodel.UUID(""), fmt.Errorf("generating new model uuid: %w", err)
		}
	}

	return uuid, s.st.Create(ctx, uuid, args)
}

// GetModel is responsible for returning the model with the provided uuid.
func (s *Service) GetModel(ctx context.Context, uuid coremodel.UUID) (*coremodel.Model, error) {
	m, err := s.st.Get(ctx, uuid)
	if err != nil {
		return nil, domain.CoerceError(err)
	}
	modelType := coremodel.ModelType(m.ModelType)
	if !modelType.IsValid() {
		// THis should never happen.
		return nil, errors.NotValidf("model type %q", m.ModelType)
	}
	return &coremodel.Model{
		UUID:      m.UUID,
		Name:      m.Name,
		ModelType: modelType,
	}, nil
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

	return s.st.Delete(ctx, uuid)
}

// ModelList returns a list of all model UUIDs in the system that have not been
// deleted. This list does not represent one or more lifecycle states for
// models.
func (s *Service) ModelList(ctx context.Context) ([]coremodel.UUID, error) {
	uuids, err := s.st.List(ctx)
	if err != nil {
		return nil, errors.Annotatef(err, "retrieving model list")
	}
	return uuids, nil
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

// SetSecretBackend sets the secret backend for a model.
func (s *Service) SetSecretBackend(
	ctx context.Context, modelUUID coremodel.UUID, backendName string,
) error {
	return s.st.SetSecretBackend(ctx, modelUUID, backendName)
}

// GetSecretBackend returns the secret backend identifier for a model.
func (s *Service) GetSecretBackend(
	ctx context.Context, modelUUID coremodel.UUID,
) (model.SecretBackendIdentifier, error) {
	return s.st.GetSecretBackend(ctx, modelUUID)
}
