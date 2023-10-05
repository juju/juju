// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretusersupplied

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("SecretUserSuppliedManager", 1, func(ctx facade.Context) (facade.Facade, error) {
		return NewSecretUserSuppliedManager(ctx)
	}, reflect.TypeOf((*SecretUserSuppliedManager)(nil)))
}

// NewSecretUserSuppliedManager creates a SecretUserSuppliedManager.
func NewSecretUserSuppliedManager(context facade.Context) (*SecretUserSuppliedManager, error) {
	if !context.Auth().AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	model, err := context.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	backendConfigGetter := func() (*provider.ModelBackendConfigInfo, error) {
		return secrets.AdminBackendConfigInfo(secrets.SecretsModel(model))
	}

	return &SecretUserSuppliedManager{
		authorizer:          context.Auth(),
		resources:           context.Resources(),
		authTag:             context.Auth().GetAuthTag(),
		controllerUUID:      context.State().ControllerUUID(),
		modelUUID:           context.State().ModelUUID(),
		secretsState:        state.NewSecrets(context.State()),
		backendConfigGetter: backendConfigGetter,
	}, nil
}
