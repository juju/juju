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
	registry.MustRegister("Secrets", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newSecretsAPIV1(ctx)
	}, reflect.TypeOf((*SecretsAPI)(nil)))
	registry.MustRegister("Secrets", 2, func(ctx facade.Context) (facade.Facade, error) {
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
	backendConfigGetter := func() (*provider.ModelBackendConfigInfo, error) {
		return secrets.AdminBackendConfigInfo(
			stdcontext.Background(), secrets.SecretsModel(model), context.ServiceFactory().Cloud(), context.ServiceFactory().Credential())
	}
	backendGetter := func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		p, err := provider.Provider(cfg.BackendType)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return p.NewBackend(cfg)
	}
	return &SecretsAPI{
		authorizer:          context.Auth(),
		authTag:             context.Auth().GetAuthTag(),
		controllerUUID:      context.State().ControllerUUID(),
		modelUUID:           context.State().ModelUUID(),
		modelName:           model.Name(),
		secretsState:        state.NewSecrets(context.State()),
		secretsConsumer:     context.State(),
		backends:            make(map[string]provider.SecretsBackend),
		backendConfigGetter: backendConfigGetter,
		backendGetter:       backendGetter,
	}, nil
}
