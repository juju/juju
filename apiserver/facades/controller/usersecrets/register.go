// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecrets

import (
	stdcontext "context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("UserSecretsManager", 1, func(stdCtx stdcontext.Context, ctx facade.Context) (facade.Facade, error) {
		return NewUserSecretsManager(ctx)
	}, reflect.TypeOf((*UserSecretsManager)(nil)))
}

// NewUserSecretsManager creates a UserSecretsManager.
func NewUserSecretsManager(context facade.Context) (*UserSecretsManager, error) {
	if !context.Auth().AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	model, err := context.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	serviceFactory := context.ServiceFactory()
	backendConfigGetter := func() (*provider.ModelBackendConfigInfo, error) {
		return secrets.AdminBackendConfigInfo(
			stdcontext.Background(), secrets.SecretsModel(model),
			serviceFactory.Cloud(), serviceFactory.Credential(),
		)
	}

	return &UserSecretsManager{
		authorizer:          context.Auth(),
		resources:           context.Resources(),
		authTag:             context.Auth().GetAuthTag(),
		controllerUUID:      context.State().ControllerUUID(),
		modelUUID:           context.State().ModelUUID(),
		secretsState:        state.NewSecrets(context.State()),
		backendConfigGetter: backendConfigGetter,
	}, nil
}
