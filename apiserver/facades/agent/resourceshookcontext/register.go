// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceshookcontext

import (
	"context"
	"reflect"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

var _ = Register

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("ResourcesHookContext", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newStateFacade(ctx)
	}, reflect.TypeOf((*UnitFacade)(nil)))
}

// newStateFacade provides the signature to register this resource facade
func newStateFacade(modelCtx facade.ModelContext) (*UnitFacade, error) {
	authorizer := modelCtx.Auth()

	if !authorizer.AuthUnitAgent() && !authorizer.AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}
	return NewUnitFacade(
		authorizer.GetAuthTag(),
		modelCtx.DomainServices().Application(),
		modelCtx.DomainServices().Resource())
}
