// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"context"
	"reflect"

	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Spaces", 6, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newAPI(ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newAPI creates a new Space API server-side facade with a
// state.State backing.
func newAPI(ctx facade.ModelContext) (*API, error) {
	// Only clients can access the Spaces facade.
	auth := ctx.Auth()
	if !auth.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	domainServices := ctx.DomainServices()

	check := common.NewBlockChecker(domainServices.BlockCommand())

	return &API{
		auth:                    auth,
		modelTag:                names.NewModelTag(ctx.ModelUUID().String()),
		controllerConfigService: domainServices.ControllerConfig(),
		networkService:          domainServices.Network(),
		applicationService:      domainServices.Application(),
		machineService:          domainServices.Machine(),
		check:                   check,
		logger:                  ctx.Logger().Child("spaces"),
	}, nil
}
