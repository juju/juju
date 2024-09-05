// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	stdcontext "context"
	"reflect"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Secrets", 1, func(stdCtx stdcontext.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newSecretsAPIV1(stdCtx, ctx)
	}, reflect.TypeOf((*SecretsAPI)(nil)))
	registry.MustRegister("Secrets", 2, func(stdCtx stdcontext.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newSecretsAPI(stdCtx, ctx)
	}, reflect.TypeOf((*SecretsAPI)(nil)))
}

func newSecretsAPIV1(stdCtx stdcontext.Context, context facade.ModelContext) (*SecretsAPIV1, error) {
	api, err := newSecretsAPI(stdCtx, context)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &SecretsAPIV1{SecretsAPI: api}, nil
}

// newSecretsAPI creates a SecretsAPI.
func newSecretsAPI(stdCtx stdcontext.Context, context facade.ModelContext) (*SecretsAPI, error) {
	if !context.Auth().AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	serviceFactory := context.ServiceFactory()
	backendService := serviceFactory.SecretBackend()

	modelInfoService := serviceFactory.ModelInfo()
	modelInfo, err := modelInfoService.GetModelInfo(stdCtx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	secretBackendAdminConfigGetter := secretbackendservice.AdminBackendConfigGetterFunc(
		serviceFactory.SecretBackend(), context.ModelUUID())
	secretBackendUserSecretConfigGetter := secretbackendservice.UserSecretBackendConfigGetterFunc(
		serviceFactory.SecretBackend(), context.ModelUUID())
	secretService := serviceFactory.Secret(secretBackendAdminConfigGetter, secretBackendUserSecretConfigGetter)

	return &SecretsAPI{
		authorizer:           context.Auth(),
		authTag:              context.Auth().GetAuthTag(),
		controllerUUID:       context.ControllerUUID(),
		modelUUID:            context.State().ModelUUID(),
		modelName:            modelInfo.Name,
		secretService:        secretService,
		secretBackendService: backendService,
	}, nil
}
