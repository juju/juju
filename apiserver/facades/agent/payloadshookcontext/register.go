// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloadshookcontext

import (
	"context"
	"reflect"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("PayloadsHookContext", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacadeV1(ctx)
	}, reflect.TypeOf((*UnitFacadeV1)(nil)))
	registry.MustRegister("PayloadsHookContext", 2, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacadeV2(ctx)
	}, reflect.TypeOf((*UnitFacadeV2)(nil)))
}

// newFacadeV1 provides the signature to register this resource facade
func newFacadeV1(ctx facade.ModelContext) (*UnitFacadeV1, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthUnitAgent() && !authorizer.AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}

	return NewHookContextFacadeV1()
}

// newFacadeV2 provides the signature to register this resource facade
func newFacadeV2(ctx facade.ModelContext) (*UnitFacadeV2, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthUnitAgent() && !authorizer.AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}

	return NewHookContextFacadeV2()
}
