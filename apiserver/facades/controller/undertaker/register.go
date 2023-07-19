// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	"github.com/juju/juju/apiserver/facade"
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
	cloudSpecAPI := cloudspec.NewCloudSpec(
		ctx.Resources(),
		cloudspec.MakeCloudSpecGetterForModel(st),
		cloudspec.MakeCloudSpecWatcherForModel(st),
		cloudspec.MakeCloudSpecCredentialWatcherForModel(st),
		cloudspec.MakeCloudSpecCredentialContentWatcherForModel(st),
		common.AuthFuncForTag(m.ModelTag()),
	)
	return newUndertakerAPI(&stateShim{st}, ctx.Resources(), ctx.Auth(), cloudSpecAPI)
}
