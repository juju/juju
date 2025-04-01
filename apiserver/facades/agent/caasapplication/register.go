// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplication

import (
	"context"
	"reflect"

	"github.com/juju/errors"

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

	resources := ctx.Resources()

	domainServices := ctx.DomainServices()

	systemState, err := ctx.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}

	applicationService := domainServices.Application()

	return NewFacade(
		resources,
		authorizer,
		systemState,
		ctx.ControllerUUID(),
		ctx.ModelUUID(),
		domainServices.ControllerConfig(),
		applicationService,
		domainServices.Agent(),
		ctx.Logger().Child("caasapplication"),
	), nil
}
