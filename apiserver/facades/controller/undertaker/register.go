// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Undertaker", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newUndertakerFacade(ctx)
	}, reflect.TypeOf((*UndertakerAPI)(nil)))
}

// newUndertakerFacade creates a new instance of the undertaker API.
func newUndertakerFacade(ctx facade.ModelContext) (*UndertakerAPI, error) {
	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	serviceFactory := ctx.ServiceFactory()
	cloudService := serviceFactory.Cloud()
	credentialService := serviceFactory.Credential()
	backendService := serviceFactory.SecretBackend()
	cloudSpecAPI := cloudspec.NewCloudSpec(
		ctx.Resources(),
		cloudspec.MakeCloudSpecGetterForModel(st, cloudService, credentialService),
		cloudspec.MakeCloudSpecWatcherForModel(st, cloudService),
		cloudspec.MakeCloudSpecCredentialWatcherForModel(st),
		cloudspec.MakeCloudSpecCredentialContentWatcherForModel(st, serviceFactory.Credential()),
		common.AuthFuncForTag(model.ModelTag()),
	)
	return newUndertakerAPI(&stateShim{st}, ctx.Resources(), ctx.Auth(), cloudSpecAPI, backendService)
}
