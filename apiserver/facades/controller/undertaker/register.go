// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	"github.com/juju/juju/apiserver/common/secrets"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/internal/secrets/provider"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Undertaker", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newUndertakerFacade(ctx)
	}, reflect.TypeOf((*UndertakerAPI)(nil)))
}

// newUndertakerFacade creates a new instance of the undertaker API.
func newUndertakerFacade(ctx facade.Context) (*UndertakerAPI, error) {
	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudService := ctx.ServiceFactory().Cloud()
	credentialService := ctx.ServiceFactory().Credential()
	secretsBackendsGetter := func() (*provider.ModelBackendConfigInfo, error) {
		model, err := st.Model()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return secrets.AdminBackendConfigInfo(context.Background(), secrets.SecretsModel(model), cloudService, credentialService)
	}
	cloudSpecAPI := cloudspec.NewCloudSpec(
		ctx.Resources(),
		cloudspec.MakeCloudSpecGetterForModel(st, cloudService, credentialService),
		cloudspec.MakeCloudSpecWatcherForModel(st, cloudService),
		cloudspec.MakeCloudSpecCredentialWatcherForModel(st),
		cloudspec.MakeCloudSpecCredentialContentWatcherForModel(st, ctx.ServiceFactory().Credential()),
		common.AuthFuncForTag(m.ModelTag()),
	)
	return newUndertakerAPI(&stateShim{st}, ctx.Resources(), ctx.Auth(), secretsBackendsGetter, cloudSpecAPI)
}
