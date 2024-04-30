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
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/secrets/provider"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("UserSecretsDrain", 1, func(stdCtx stdcontext.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newUserSecretsDrainAPI(ctx)
	}, reflect.TypeOf((*SecretsDrainAPI)(nil)))
}

// newUserSecretsDrainAPI creates a SecretsDrainAPI for draining user secrets.
func newUserSecretsDrainAPI(context facade.ModelContext) (*SecretsDrainAPI, error) {
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
	modelUUID := coremodel.UUID(model.UUID())
	serviceFactory := context.ServiceFactory()
	backendService := serviceFactory.SecretBackend(model.ControllerUUID(), provider.Provider)

	secretBackendAdminConfigGetter := func(stdCtx stdcontext.Context) (*provider.ModelBackendConfigInfo, error) {
		return backendService.GetSecretBackendConfigForAdmin(stdCtx, modelUUID)
	}
	secretService := context.ServiceFactory().Secret(secretBackendAdminConfigGetter)

	authTag := model.ModelTag()
	commonDrainAPI, err := commonsecrets.NewSecretsDrainAPI(
		authTag,
		context.Auth(),
		context.Logger().Child("usersecretsdrain"),
		leadershipChecker,
		modelUUID,
		model,
		secretService,
		backendService,
		context.WatcherRegistry(),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &SecretsDrainAPI{
		SecretsDrainAPI:      commonDrainAPI,
		modelUUID:            model.UUID(),
		secretBackendService: backendService,
		secretService:        secretService,
	}, nil
}
