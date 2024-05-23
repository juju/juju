// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package handlers

import (
	"context"
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/modeldefaults"
	backenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/environs/config"
)

// SecretBackendState provides access to the state where secret backends are stored.
type SecretBackendState interface {
	SetModelSecretBackend(ctx context.Context, modelUUID coremodel.UUID, backendName string) error
	GetModelSecretBackendName(ctx context.Context, modelUUID coremodel.UUID) (string, error)
}

// SecretBackendHandler implements ConfigHandler.
type SecretBackendHandler struct {
	Defaults     modeldefaults.Defaults
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
func (h SecretBackendHandler) OnSave(ctx context.Context, rawCfg map[string]any, removeAttrs []string) error {
	// If removing, set to the model default value.
	toRemove := set.NewStrings(removeAttrs...)
	if toRemove.Contains(config.SecretBackendKey) {
		var backendName any = config.DefaultSecretBackend
		defaultBackend, ok := h.Defaults[config.SecretBackendKey]
		if ok {
			backendName = defaultBackend.Value
		}
		rawCfg[config.SecretBackendKey] = backendName
	}

	// Exit early if secret backend is not being updated.
	val, ok := rawCfg[config.SecretBackendKey]
	if !ok {
		return nil
	}
	backendName := fmt.Sprint(val)

	err := h.BackendState.SetModelSecretBackend(ctx, h.ModelUUID, backendName)
	if err != nil && !errors.Is(err, backenderrors.NotFound) {
		return fmt.Errorf("cannot set secret backend to %q: %w", backendName, err)
	}
	if err != nil {
		return &config.ValidationError{
			InvalidAttrs: []string{config.SecretBackendKey},
			Reason:       fmt.Sprintf("secret backend %q not found", backendName),
		}
	}
	delete(rawCfg, config.SecretBackendKey)
	return nil
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
