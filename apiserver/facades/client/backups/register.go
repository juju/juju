// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
	coremodel "github.com/juju/juju/core/model"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegisterForMultiModel("Backups", 3, func(stdCtx context.Context, ctx facade.MultiModelContext) (facade.Facade, error) {
		return newFacade(stdCtx, ctx)
	}, reflect.TypeFor[*API]())
}

// newFacade provides the required signature for facade registration.
func newFacade(stdCtx context.Context, ctx facade.MultiModelContext) (*API, error) {
	controllerModelUUID := ctx.ControllerModelUUID()
	controllerServices, err := ctx.DomainServicesForModel(stdCtx, controllerModelUUID)
	if err != nil {
		return nil, err
	}

	modelServicesFor := ModelServicesForFunc(
		func(stdCtx context.Context, modelUUID coremodel.UUID) (ModelExportDomainServices, error) {
			return ctx.DomainServicesForModel(stdCtx, modelUUID)
		},
	)

	return NewAPI(
		ctx.Auth(),
		ctx.MachineTag(),
		ctx.ControllerUUID(),
		controllerModelUUID,
		ctx.DataDir(),
		ctx.LogDir(),
		controllerServices.ControllerExport(),
		modelServicesFor,
		controllerServices.Config(),
		controllerServices.Controller(),
		controllerServices.ControllerNode(),
		ctx.Clock(),
		ctx.Logger().Child("backups"),
	)
}
