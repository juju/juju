// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadatamanager

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state/stateenvirons"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("ImageMetadataManager", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newAPI(stdCtx, ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newAPI returns a new cloud image metadata API facade.
func newAPI(ctx context.Context, modelctx facade.ModelContext) (*API, error) {
	st := modelctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	newEnviron := func() (environs.Environ, error) {
		return stateenvirons.GetNewEnvironFunc(environs.New)(model, modelctx.ServiceFactory().Cloud(), modelctx.ServiceFactory().Credential())
	}
	return createAPI(ctx, getState(st), modelctx.ServiceFactory().Config(), newEnviron, modelctx.Resources(), modelctx.Auth())
}
