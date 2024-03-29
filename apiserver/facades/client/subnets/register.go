// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Subnets", 5, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newAPI(ctx) // Removes AddSubnets.
	}, reflect.TypeOf((*API)(nil)))
}

// newAPI creates a new Subnets API server-side facade with a
// state.State backing.
func newAPI(ctx facade.ModelContext) (*API, error) {
	st := ctx.State()
	stateShim, err := NewStateShim(st, ctx.ServiceFactory().Cloud(), ctx.ServiceFactory().Credential())
	if err != nil {
		return nil, errors.Trace(err)
	}

	return newAPIWithBacking(
		stateShim,
		credentialcommon.CredentialInvalidatorGetter(ctx),
		ctx.Resources(),
		ctx.Auth(),
		ctx.Logger().Child("subnets"),
		ctx.ServiceFactory().Network(),
	)
}
