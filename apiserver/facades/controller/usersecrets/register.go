// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecrets

import (
	"context"
	"reflect"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("UserSecretsManager", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return NewUserSecretsManager(stdCtx, ctx)
	}, reflect.TypeOf((*UserSecretsManager)(nil)))
}

// NewUserSecretsManager creates a UserSecretsManager.
func NewUserSecretsManager(stdCtx context.Context, ctx facade.ModelContext) (*UserSecretsManager, error) {
	if !ctx.Auth().AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	domainServices := ctx.DomainServices()
	backendService := domainServices.SecretBackend()

	return &UserSecretsManager{
		watcherRegistry: ctx.WatcherRegistry(),
		secretService: domainServices.Secret(
			secretservice.SecretServiceParams{
				BackendUserSecretConfigGetter: secretbackendservice.UserSecretBackendConfigGetterFunc(
					backendService, ctx.ModelUUID(),
				),
			},
		),
	}, nil
}
