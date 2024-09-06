// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecretsdrain

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("UserSecretsDrain", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newUserSecretsDrainAPI(stdCtx, ctx)
	}, reflect.TypeOf((*SecretsDrainAPI)(nil)))
}

// newUserSecretsDrainAPI creates a SecretsDrainAPI for draining user secrets.
func newUserSecretsDrainAPI(stdCtx context.Context, ctx facade.ModelContext) (*SecretsDrainAPI, error) {
	if !ctx.Auth().AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	leadershipChecker, err := ctx.LeadershipChecker()
	if err != nil {
		return nil, errors.Trace(err)
	}
	model, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	serviceFactory := ctx.ServiceFactory()
	backendService := serviceFactory.SecretBackend()

	secretService := ctx.ServiceFactory().Secret(
		secretservice.SecretServiceParams{
			BackendAdminConfigGetter: secretbackendservice.AdminBackendConfigGetterFunc(
				backendService, ctx.ModelUUID(),
			),
			BackendUserSecretConfigGetter: secretbackendservice.UserSecretBackendConfigGetterFunc(
				backendService, ctx.ModelUUID(),
			),
		},
	)

	authTag := model.ModelTag()
	commonDrainAPI, err := commonsecrets.NewSecretsDrainAPI(
		authTag,
		ctx.Auth(),
		ctx.Logger().Child("usersecretsdrain"),
		leadershipChecker,
		ctx.ModelUUID(),
		secretService,
		backendService,
		ctx.WatcherRegistry(),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &SecretsDrainAPI{
		SecretsDrainAPI:      commonDrainAPI,
		modelUUID:            ctx.ModelUUID().String(),
		secretBackendService: backendService,
		secretService:        secretService,
	}, nil
}
