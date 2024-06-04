// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package handlers

import (
	"context"
	"fmt"

	"github.com/juju/errors"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/config"
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
}

// Name is the handler name.
func (h SecretBackendHandler) Name() string {
	return "model secret backend"
}

// OnSave is run when saving model config; any secret
// backend is removed from rawCfg and used to update the
// model secret backend config table.
// It returns an error satisfying [github.com/juju/juju/domain/secretbackend/errors.NotFound]
// if the secret  backend is not found or
// [github.com/juju/juju/domain/model/errors.NotFound] if the model is not found.
func (h SecretBackendHandler) OnSave(ctx context.Context, rawCfg map[string]any) (RollbackFunc, error) {
	// Exit early if secret backend is not being updated.
	val, ok := rawCfg[config.SecretBackendKey]
	if !ok {
		return noopRollback, nil
	}

	// Set up the rollback func so we can revert to the current backend
	// if there's any error.
	currentBackendName, err := h.BackendState.GetModelSecretBackendName(ctx, h.ModelUUID)
	if err != nil {
		return noopRollback, errors.Trace(err)
	}
	rollbackFunc := func(ctx context.Context) error {
		return h.BackendState.SetModelSecretBackend(ctx, h.ModelUUID, currentBackendName)
	}

	// Set the new backend.
	backendName := fmt.Sprint(val)
	err = h.BackendState.SetModelSecretBackend(ctx, h.ModelUUID, backendName)
	if err != nil {
		return noopRollback, fmt.Errorf("cannot set model %q secret backend to %q: %w", h.ModelUUID, backendName, err)
	}
	delete(rawCfg, config.SecretBackendKey)
	return rollbackFunc, nil
}

// OnLoad is run when loading model config. The result value is a map
// containing the name of the secret backend for the model.
func (h SecretBackendHandler) OnLoad(ctx context.Context) (map[string]string, error) {
	backendName, err := h.BackendState.GetModelSecretBackendName(ctx, h.ModelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return map[string]string{
		config.SecretBackendKey: backendName,
	}, nil
}
