// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/secretbackend"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/secrets/provider/vault"
)

// ModelSecretBackendService is a service for interacting with the secret backend state for a specific model.
type ModelSecretBackendService struct {
	st            State
	modelUUID     coremodel.UUID
	versionGetter AgentVersionGetter
}

// NewModelSecretBackendService creates a new ModelSecretBackendService for interacting with the secret backend state for a specific model.
func NewModelSecretBackendService(modelID coremodel.UUID, st State, versionGetter AgentVersionGetter) *ModelSecretBackendService {
	return &ModelSecretBackendService{modelUUID: modelID, st: st, versionGetter: versionGetter}
}

// GetModelSecretBackend returns the secret backend name for the current model ID,
// returning an error satisfying [modelerrors.NotFound] if the model provided does not exist.
func (s *ModelSecretBackendService) GetModelSecretBackend(ctx context.Context) (string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	modelSecretBackend, err := s.st.GetModelSecretBackendDetails(ctx, s.modelUUID)
	if err != nil {
		return "", errors.Errorf("getting model secret backend detail for %q: %w", s.modelUUID, err)
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
		modelType, err := s.st.GetModelType(ctx, s.modelUUID)
		if err != nil {
			return errors.Errorf("getting model type for %q: %w", s.modelUUID, err)
		}
		switch modelType {
		case coremodel.IAAS:
			backendName = provider.Internal
		case coremodel.CAAS:
			backendName = kubernetes.BackendName
		default:
			// Should never happen.
			return errors.Errorf("setting model secret backend for unsupported model type %q for model %q",
				modelType, s.modelUUID)

		}
	}

	if err := s.checkBackendCompatibility(ctx, backendName); err != nil {
		return errors.Errorf("checking backend compatibility for %q: %w", backendName, err)
	}

	if err := s.st.SetModelSecretBackend(ctx, s.modelUUID, backendName); err != nil {
		return errors.Errorf("setting model secret backend for %q: %w", s.modelUUID, err)
	}
	return nil
}

// checkBackendCompatibility verifies if the specified secret backend is
// compatible with the model's target Juju agent version.
// It checks for built-in backends or applies version-specific logic for other
// backends, returning an error if incompatible.
func (s *ModelSecretBackendService) checkBackendCompatibility(ctx context.Context, name string) error {
	if name == provider.Internal || name == kubernetes.BackendName {
		// Internal and k8s secret backends are compatible with all versions of Juju, since those are built-in backends
		return nil
	}
	backend, err := s.st.GetSecretBackend(ctx, secretbackend.BackendIdentifier{Name: name})
	if err != nil {
		return errors.Errorf("getting secret backend %q: %w", name, err)
	}
	if backend.BackendType == vault.BackendType {
		mountPath, _ := backend.Config[vault.MountPathKey].(string)
		if mountPath != "" {
			modelVersion, err := s.versionGetter.GetModelTargetAgentVersion(ctx)
			if err != nil {
				return errors.Errorf("getting model agent version for %q: %w", s.modelUUID, err)
			}

			if modelVersion.Compare(semversion.MustParse("3.6.12")) < 0 {
				return errors.Errorf(
					"model agent version should be at least 3.6.12 to support %q for %q secret backend",
					vault.MountPathKey, vault.BackendType).Add(secretbackenderrors.NotSupported)
			}
			if modelVersion.Major == 4 && modelVersion.Compare(semversion.MustParse("4.0.1")) < 0 {
				return errors.Errorf(
					"model agent version should be at least 4.0.1 to support %q for %q secret backend",
					vault.MountPathKey, vault.BackendType).Add(secretbackenderrors.NotSupported)
			}
		}
	}

	return nil
}
