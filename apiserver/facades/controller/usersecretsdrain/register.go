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
	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/internal/secrets/provider"
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

	modelInfoService := serviceFactory.ModelInfo()
	modelInfo, err := modelInfoService.GetModelInfo(stdCtx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	secretBackendAdminConfigGetter := func(stdCtx stdcontext.Context) (*provider.ModelBackendConfigInfo, error) {
		return backendService.GetSecretBackendConfigForAdmin(stdCtx, modelInfo.UUID)
	}
	backendUserSecretConfigGetter := func(
		stdCtx stdcontext.Context, gsg secretservice.GrantedSecretsGetter, accessor secretservice.SecretAccessor,
	) (*provider.ModelBackendConfigInfo, error) {
		return backendService.BackendConfigInfo(stdCtx, secretbackendservice.BackendConfigParams{
			GrantedSecretsGetter: gsg,
			Accessor:             accessor,
			ModelUUID:            modelInfo.UUID,
			SameController:       true,
		})
	}
	secretService := context.ServiceFactory().Secret(secretBackendAdminConfigGetter, backendUserSecretConfigGetter)

	authTag := model.ModelTag()
	commonDrainAPI, err := commonsecrets.NewSecretsDrainAPI(
		authTag,
		context.Auth(),
		context.Logger().Child("usersecretsdrain"),
		leadershipChecker,
		modelInfo.UUID,
		secretService,
		backendService,
		context.WatcherRegistry(),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &SecretsDrainAPI{
		SecretsDrainAPI:      commonDrainAPI,
		modelUUID:            modelInfo.UUID.String(),
		secretBackendService: backendService,
		secretService:        secretService,
	}, nil
}
