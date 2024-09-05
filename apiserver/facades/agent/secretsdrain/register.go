// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrain

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("SecretsDrain", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newSecretsDrainAPI(stdCtx, ctx)
	}, reflect.TypeOf((*commonsecrets.SecretsDrainAPI)(nil)))
}

// newSecretsDrainAPI creates a SecretsDrainAPI.
func newSecretsDrainAPI(stdCtx context.Context, ctx facade.ModelContext) (*commonsecrets.SecretsDrainAPI, error) {
	if !ctx.Auth().AuthUnitAgent() {
		return nil, apiservererrors.ErrPerm
	}
	leadershipChecker, err := ctx.LeadershipChecker()
	if err != nil {
		return nil, errors.Trace(err)
	}
	serviceFactory := ctx.ServiceFactory()
	secretBackendService := ctx.ServiceFactory().SecretBackend()

	authTag := ctx.Auth().GetAuthTag()

	secretBackendAdminConfigGetter := secretbackendservice.AdminBackendConfigGetterFunc(
		secretBackendService, ctx.ModelUUID())
	backendUserSecretConfigGetter := secretbackendservice.UserSecretBackendConfigGetterFunc(
		secretBackendService, ctx.ModelUUID())
	return commonsecrets.NewSecretsDrainAPI(
		authTag,
		ctx.Auth(),
		ctx.Logger().Child("secretsdrain"),
		leadershipChecker,
		ctx.ModelUUID(),
		serviceFactory.Secret(secretBackendAdminConfigGetter, backendUserSecretConfigGetter),
		secretBackendService,
		ctx.WatcherRegistry(),
	)
}
