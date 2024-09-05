// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecrets

import (
	stdcontext "context"
	"reflect"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
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
	backendService := serviceFactory.SecretBackend()
	secretBackendAdminConfigGetter := secretbackendservice.AdminBackendConfigGetterFunc(
		backendService, ctx.ModelUUID())
	secretBackendUserSecretConfigGetter := secretbackendservice.UserSecretBackendConfigGetterFunc(
		backendService, ctx.ModelUUID())

	return &UserSecretsManager{
		watcherRegistry: ctx.WatcherRegistry(),
		secretService:   serviceFactory.Secret(secretBackendAdminConfigGetter, secretBackendUserSecretConfigGetter),
	}, nil
}
