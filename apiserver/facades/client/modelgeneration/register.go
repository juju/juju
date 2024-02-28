// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("ModelGeneration", 4, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newModelGenerationFacadeV4(ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newModelGenerationFacadeV4 provides the signature required for facade registration.
func newModelGenerationFacadeV4(ctx facade.ModelContext) (*API, error) {
	authorizer := ctx.Auth()
	st := &stateShim{State: ctx.State()}
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewModelGenerationAPI(st, authorizer, m)
}
