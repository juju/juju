// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/trace"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
)

// ModelSecretBackendService is a service for interacting with the secret backend state for a specific model.
type ModelSecretBackendService struct {
	st      State
	modelID coremodel.UUID
}

// NewModelSecretBackendService creates a new ModelSecretBackendService for interacting with the secret backend state for a specific model.
func NewModelSecretBackendService(modelID coremodel.UUID, st State) *ModelSecretBackendService {
	return &ModelSecretBackendService{modelID: modelID, st: st}
}

// GetModelSecretBackend returns the secret backend name for the current model ID,
// returning an error satisfying [modelerrors.NotFound] if the model provided does not exist.
func (s *ModelSecretBackendService) GetModelSecretBackend(ctx context.Context) (string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	modelSecretBackend, err := s.st.GetModelSecretBackendDetails(ctx, s.modelID)
	if err != nil {
		return "", errors.Errorf("getting model secret backend detail for %q: %w", s.modelID, err)
	}
	backendName := modelSecretBackend.SecretBackendName
	switch modelSecretBackend.ModelType {
	case coremodel.IAAS:
		if backendName == provider.Internal {
			backendName = provider.Auto
		}
	case coremodel.CAAS:
		if backendName == kubernetes.BackendName {
			backendName = provider.Auto
		}
	}
	return backendName, nil
}

// SetModelSecretBackend sets the secret backend config for the current model ID,
// returning an error satisfying [secretbackenderrors.NotFound] if the backend provided does not exist,
// returning an error satisfying [modelerrors.NotFound] if the model provided does not exist,
// returning an error satisfying [secretbackenderrors.NotValid] if the backend name provided is not valid.
func (s *ModelSecretBackendService) SetModelSecretBackend(ctx context.Context, backendName string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if backendName == "" {
		return errors.Errorf("missing backend name")
	}
	if backendName == provider.Internal || backendName == kubernetes.BackendName {
		return errors.Errorf("secret backend name %q not valid", backendName).Add(secretbackenderrors.NotValid)
	}

	if backendName == provider.Auto {
		modelType, err := s.st.GetModelType(ctx, s.modelID)
		if err != nil {
			return errors.Errorf("getting model type for %q: %w", s.modelID, err)
		}
		switch modelType {
		case coremodel.IAAS:
			backendName = provider.Internal
		case coremodel.CAAS:
			backendName = kubernetes.BackendName
		default:
			// Should never happen.
			return errors.Errorf("setting model secret backend for unsupported model type %q for model %q",
				modelType, s.modelID)

		}
	}
	if err := s.st.SetModelSecretBackend(ctx, s.modelID, backendName); err != nil {
		return errors.Errorf("setting model secret backend for %q: %w", s.modelID, err)
	}
	return nil
}
