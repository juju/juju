// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	stdcontext "context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Secrets", 1, func(stdCtx stdcontext.Context, ctx facade.Context) (facade.Facade, error) {
		return newSecretsAPIV1(ctx)
	}, reflect.TypeOf((*SecretsAPI)(nil)))
	registry.MustRegister("Secrets", 2, func(stdCtx stdcontext.Context, ctx facade.Context) (facade.Facade, error) {
		return newSecretsAPI(ctx)
	}, reflect.TypeOf((*SecretsAPI)(nil)))
}

func newSecretsAPIV1(context facade.Context) (*SecretsAPIV1, error) {
	api, err := newSecretsAPI(context)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &SecretsAPIV1{SecretsAPI: api}, nil
}

// newSecretsAPI creates a SecretsAPI.
func newSecretsAPI(context facade.Context) (*SecretsAPI, error) {
	if !context.Auth().AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	model, err := context.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	serviceFactory := context.ServiceFactory()
	leadershipChecker, err := context.LeadershipChecker()
	if err != nil {
		return nil, errors.Trace(err)
	}
	adminBackendConfigGetter := func(ctx stdcontext.Context) (*provider.ModelBackendConfigInfo, error) {
		return secrets.AdminBackendConfigInfo(
			ctx, secrets.SecretsModel(model),
			serviceFactory.Cloud(), serviceFactory.Credential(),
		)
	}
	backendConfigGetterForUserSecretsWrite := func(ctx stdcontext.Context, backendID string) (*provider.ModelBackendConfigInfo, error) {
		// User secrets are owned by the model.
		authTag := model.ModelTag()
		return secrets.BackendConfigInfo(
			ctx, secrets.SecretsModel(model),
			serviceFactory.Cloud(), serviceFactory.Credential(),
			[]string{backendID}, false, authTag, leadershipChecker,
		)
	}

	backendGetter := func(ctx stdcontext.Context, cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		p, err := provider.Provider(cfg.BackendType)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return p.NewBackend(cfg)
	}
	return &SecretsAPI{
		authorizer:                             context.Auth(),
		authTag:                                context.Auth().GetAuthTag(),
		controllerUUID:                         context.State().ControllerUUID(),
		modelUUID:                              context.State().ModelUUID(),
		modelName:                              model.Name(),
		secretsState:                           state.NewSecrets(context.State()),
		secretsConsumer:                        context.State(),
		backends:                               make(map[string]provider.SecretsBackend),
		adminBackendConfigGetter:               adminBackendConfigGetter,
		backendConfigGetterForUserSecretsWrite: backendConfigGetterForUserSecretsWrite,
		backendGetter:                          backendGetter,
	}, nil
}
