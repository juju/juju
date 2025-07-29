// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Firewaller", 7, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFirewallerAPIV7(ctx)
	}, reflect.TypeOf((*FirewallerAPI)(nil)))
}

// newFirewallerAPIV7 creates a new server-side FirewallerAPIv7 facade.
func newFirewallerAPIV7(ctx facade.ModelContext) (*FirewallerAPI, error) {
	domainServices := ctx.DomainServices()
	controllerConfigAPI := common.NewControllerConfigAPI(
		domainServices.ControllerConfig(),
		domainServices.ControllerNode(),
		domainServices.ExternalController(),
		domainServices.Model(),
	)

	return NewStateFirewallerAPI(
		domainServices.Network(),
		ctx.WatcherRegistry(),
		ctx.Auth(),
		controllerConfigAPI,
		domainServices.ControllerConfig(),
		domainServices.Config(),
		domainServices.Application(),
		domainServices.Machine(),
		domainServices.ModelInfo(),
		ctx.Logger().Child("firewaller"),
	)
}
