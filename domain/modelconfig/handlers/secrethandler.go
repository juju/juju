// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package handlers

import (
	"context"
	"fmt"

	"github.com/juju/errors"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
)

// SecretBackendState provides access to the state where secret backends are stored.
type SecretBackendState interface {
	SetModelSecretBackend(ctx context.Context, modelUUID coremodel.UUID, backendName string) error
	GetModelSecretBackendName(ctx context.Context, modelUUID coremodel.UUID) (string, error)
}

// SecretBackendHandler implements ConfigHandler.
type SecretBackendHandler struct {
	BackendState SecretBackendState
	ModelUUID    coremodel.UUID
	ModelType    coremodel.ModelType
}

// Name is the handler name.
func (h SecretBackendHandler) Name() string {
	return "model secret backend"
}

// RegisteredKeys returns the keys the handler will be asked to process.
func (h SecretBackendHandler) RegisteredKeys() []string {
	return []string{config.SecretBackendKey}
}

// OnSave is run when saving model config; any secret
// backend is removed from rawCfg and used to update the
// model secret backend config table.
// It returns an error satisfying [github.com/juju/juju/domain/secretbackend/errors.NotFound]
// if the secret  backend is not found or
// [github.com/juju/juju/domain/model/errors.NotFound] if the model is not found.
func (h SecretBackendHandler) OnSave(ctx context.Context, rawCfg map[string]any) error {
	// Exit early if secret backend is not being updated.
	val, ok := rawCfg[config.SecretBackendKey]
	if !ok {
		return nil
	}

	// Set the new backend.
	backendName := fmt.Sprint(val)
	if backendName == provider.Auto {
		switch h.ModelType {
		case coremodel.IAAS:
			backendName = provider.Internal
		case coremodel.CAAS:
			backendName = kubernetes.BackendName
		default:
			// Should never happen.
			return errors.NotValidf("model type %q", h.ModelType)
		}
	}

	err := h.BackendState.SetModelSecretBackend(ctx, h.ModelUUID, backendName)
	if err != nil {
		return fmt.Errorf("cannot set model %q secret backend to %q: %w", h.ModelUUID, backendName, err)
	}
	return nil
}

// OnLoad is run when loading model config. The result value is a map
// containing the name of the secret backend for the model.
func (h SecretBackendHandler) OnLoad(ctx context.Context) (map[string]string, error) {
	backendName, err := h.BackendState.GetModelSecretBackendName(ctx, h.ModelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	switch h.ModelType {
	case coremodel.IAAS:
		if backendName == provider.Internal {
			backendName = provider.Auto
		}
	case coremodel.CAAS:
		if backendName == kubernetes.BackendName {
			backendName = provider.Auto
		}
	default:
		// Should never happen.
		return nil, errors.NotValidf("model type %q", h.ModelType)
	}
	return map[string]string{
		config.SecretBackendKey: backendName,
	}, nil
}

// Validate is responsible for asserting the secret backend in the
// updated model config is valid for the model. If the secret backend has not
// changed or is the default backend then no validation is performed.
// Any validation errors will satisfy config.ValidationError.
func (h SecretBackendHandler) Validate(_ context.Context, cfg, old map[string]any) error {
	backendName := cfg[config.SecretBackendKey]
	if backendName == old[config.SecretBackendKey] {
		return nil
	}
	if backendName == "" {
		return &config.ValidationError{
			InvalidAttrs: []string{config.SecretBackendKey},
			Cause:        errors.ConstError("secret back cannot be empty"),
		}
	}
	if backendName == config.DefaultSecretBackend {
		return nil
	}
	if h.ModelType == coremodel.CAAS && backendName == juju.BackendName {
		return &config.ValidationError{
			InvalidAttrs: []string{config.SecretBackendKey},
			Cause:        errors.ConstError(`caas secret backend cannot be set to "internal"`),
		}
	}
	if h.ModelType == coremodel.IAAS && backendName == kubernetes.BackendName {
		return &config.ValidationError{
			InvalidAttrs: []string{config.SecretBackendKey},
			Cause:        errors.ConstError(`iaas secret backend cannot be set to "kubernetes"`),
		}
	}
	return nil
}
