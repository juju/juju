// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplication

import (
	"context"
	"reflect"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CAASApplication", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newStateFacade(ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

// newStateFacade provides the signature required for facade registration.
func newStateFacade(ctx facade.ModelContext) (*Facade, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthApplicationAgent() && !authorizer.AuthUnitAgent() {
		return nil, apiservererrors.ErrPerm
	}

	domainServices := ctx.DomainServices()

	applicationService := domainServices.Application()

	return NewFacade(
		authorizer,
		ctx.ControllerUUID(),
		ctx.ModelUUID(),
		domainServices.ControllerConfig(),
		domainServices.ControllerNode(),
		applicationService,
		domainServices.Agent(),
		ctx.Logger().Child("caasapplication"),
	), nil
}
