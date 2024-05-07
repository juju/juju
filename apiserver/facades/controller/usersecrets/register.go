// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecrets

import (
	stdcontext "context"
	"reflect"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/secrets/provider"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("UserSecretsManager", 1, func(stdCtx stdcontext.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return NewUserSecretsManager(ctx)
	}, reflect.TypeOf((*UserSecretsManager)(nil)))
}

// NewUserSecretsManager creates a UserSecretsManager.
func NewUserSecretsManager(ctx facade.ModelContext) (*UserSecretsManager, error) {
	if !ctx.Auth().AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	model, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	serviceFactory := ctx.ServiceFactory()
	backendService := serviceFactory.SecretBackend()
	backendConfigGetter := func(ctx stdcontext.Context) (*provider.ModelBackendConfigInfo, error) {
		return backendService.GetSecretBackendConfigForAdmin(ctx, coremodel.UUID(model.UUID()))
	}

	return &UserSecretsManager{
		authorizer:    ctx.Auth(),
		resources:     ctx.Resources(),
		secretService: serviceFactory.Secret(backendConfigGetter),
	}, nil
}
