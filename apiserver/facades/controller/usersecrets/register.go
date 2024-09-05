// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecrets

import (
	"context"
	stdcontext "context"
	"reflect"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/internal/secrets/provider"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("UserSecretsManager", 1, func(stdCtx stdcontext.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return NewUserSecretsManager(stdCtx, ctx)
	}, reflect.TypeOf((*UserSecretsManager)(nil)))
}

// NewUserSecretsManager creates a UserSecretsManager.
func NewUserSecretsManager(stdCtx stdcontext.Context, ctx facade.ModelContext) (*UserSecretsManager, error) {
	if !ctx.Auth().AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	serviceFactory := ctx.ServiceFactory()

	modelInfoService := serviceFactory.ModelInfo()
	modelInfo, err := modelInfoService.GetModelInfo(stdCtx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	backendService := serviceFactory.SecretBackend()
	backendConfigGetter := func(ctx stdcontext.Context) (*provider.ModelBackendConfigInfo, error) {
		return backendService.GetSecretBackendConfigForAdmin(ctx, modelInfo.UUID)
	}
	backendUserSecretConfigGetter := func(
		stdCtx context.Context, gsg secretservice.GrantedSecretsGetter, accessor secretservice.SecretAccessor,
	) (*provider.ModelBackendConfigInfo, error) {
		return backendService.BackendConfigInfo(stdCtx, secretbackendservice.BackendConfigParams{
			GrantedSecretsGetter: gsg,
			Accessor:             accessor,
			ModelUUID:            modelInfo.UUID,
			SameController:       true,
		})
	}

	return &UserSecretsManager{
		watcherRegistry: ctx.WatcherRegistry(),
		secretService:   serviceFactory.Secret(backendConfigGetter, backendUserSecretConfigGetter),
	}, nil
}
