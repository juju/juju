// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("HighAvailability", 2, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newHighAvailabilityAPIV2(stdCtx, ctx)
	}, reflect.TypeOf((*HighAvailabilityAPIV2)(nil)))
	registry.MustRegister("HighAvailability", 3, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newHighAvailabilityAPI(stdCtx, ctx)
	}, reflect.TypeOf((*HighAvailabilityAPI)(nil)))
}

// newHighAvailabilityAPI creates a new server-side highavailability API end point.
func newHighAvailabilityAPIV2(stdCtx context.Context, ctx facade.ModelContext) (*HighAvailabilityAPIV2, error) {
	v3, err := newHighAvailabilityAPI(stdCtx, ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &HighAvailabilityAPIV2{*v3}, nil
}

// newHighAvailabilityAPI creates a new server-side highavailability API end point.
func newHighAvailabilityAPI(stdCtx context.Context, ctx facade.ModelContext) (*HighAvailabilityAPI, error) {
	domainServices := ctx.DomainServices()
	return NewHighAvailabilityAPI(
		stdCtx,
		ctx.State(),
		ctx.Auth(),
		domainServices.ControllerNode(),
		domainServices.Machine(),
		domainServices.Application(),
		domainServices.ModelInfo(),
		domainServices.ControllerConfig(),
		domainServices.Network(),
		domainServices.BlockCommand(),
		ctx.Logger().Child("highavailability"),
	)
}
