// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
<<<<<<< HEAD
=======
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state"
>>>>>>> 3.6
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Secrets", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newSecretsAPIV1(stdCtx, ctx)
	}, reflect.TypeOf((*SecretsAPI)(nil)))
	registry.MustRegister("Secrets", 2, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newSecretsAPI(stdCtx, ctx)
	}, reflect.TypeOf((*SecretsAPI)(nil)))
}

func newSecretsAPIV1(stdCtx context.Context, context facade.ModelContext) (*SecretsAPIV1, error) {
	api, err := newSecretsAPI(stdCtx, context)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &SecretsAPIV1{SecretsAPI: api}, nil
}

// newSecretsAPI creates a SecretsAPI.
func newSecretsAPI(stdCtx context.Context, ctx facade.ModelContext) (*SecretsAPI, error) {
	if !ctx.Auth().AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	domainServices := ctx.DomainServices()
	backendService := domainServices.SecretBackend()

	modelInfoService := domainServices.ModelInfo()
	modelInfo, err := modelInfoService.GetModelInfo(stdCtx)
	if err != nil {
		return nil, errors.Trace(err)
	}
<<<<<<< HEAD
=======
	leadershipChecker, err := context.LeadershipChecker()
	if err != nil {
		return nil, errors.Trace(err)
	}
	adminBackendConfigGetter := func() (*provider.ModelBackendConfigInfo, error) {
		return secrets.AdminBackendConfigInfo(secrets.SecretsModel(model))
	}
	backendConfigGetterForUserSecretsWrite := func(
		backendID string, only []*coresecrets.URI,
	) (*provider.ModelBackendConfigInfo, error) {
		// User secrets are owned by the model.
		authTag := model.ModelTag()
		return secrets.BackendConfigInfo(
			secrets.SecretsModel(model), true, []string{backendID}, false,
			authTag, leadershipChecker, only,
		)
	}
>>>>>>> 3.6

	secretService := domainServices.Secret()

	return &SecretsAPI{
		authorizer:           ctx.Auth(),
		authTag:              ctx.Auth().GetAuthTag(),
		controllerUUID:       ctx.ControllerUUID(),
		modelUUID:            ctx.ModelUUID().String(),
		modelName:            modelInfo.Name,
		secretService:        secretService,
		secretBackendService: backendService,
	}, nil
}
