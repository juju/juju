// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecretsdrain

import (
	stdcontext "context"
	"reflect"

	"github.com/juju/errors"

	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("UserSecretsDrain", 1, func(stdCtx stdcontext.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newUserSecretsDrainAPI(stdCtx, ctx)
	}, reflect.TypeOf((*SecretsDrainAPI)(nil)))
}

// newUserSecretsDrainAPI creates a SecretsDrainAPI for draining user secrets.
func newUserSecretsDrainAPI(stdCtx stdcontext.Context, context facade.ModelContext) (*SecretsDrainAPI, error) {
	if !context.Auth().AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	leadershipChecker, err := context.LeadershipChecker()
	if err != nil {
		return nil, errors.Trace(err)
	}
	model, err := context.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	serviceFactory := context.ServiceFactory()
	backendService := serviceFactory.SecretBackend()

	secretBackendAdminConfigGetter := secretbackendservice.AdminBackendConfigGetterFunc(
		backendService, context.ModelUUID())
	secretBackendUserSecretConfigGetter := secretbackendservice.UserSecretBackendConfigGetterFunc(
		backendService, context.ModelUUID())
	secretService := context.ServiceFactory().Secret(secretBackendAdminConfigGetter, secretBackendUserSecretConfigGetter)

	authTag := model.ModelTag()
	commonDrainAPI, err := commonsecrets.NewSecretsDrainAPI(
		authTag,
		context.Auth(),
		context.Logger().Child("usersecretsdrain"),
		leadershipChecker,
		context.ModelUUID(),
		secretService,
		backendService,
		context.WatcherRegistry(),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &SecretsDrainAPI{
		SecretsDrainAPI:      commonDrainAPI,
		modelUUID:            context.ModelUUID().String(),
		secretBackendService: backendService,
		secretService:        secretService,
	}, nil
}
