// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"context"
	"reflect"

	"github.com/juju/errors"
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
	if !ctx.Auth().AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	st := ctx.State()

	domainServices := ctx.DomainServices()

	stateShim, err := NewStateShim(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	check := common.NewBlockChecker(domainServices.BlockCommand())
	auth := ctx.Auth()

	return newAPIWithBacking(apiConfig{
		modelTag:                names.NewModelTag(ctx.ModelUUID().String()),
		ControllerConfigService: domainServices.ControllerConfig(),
		NetworkService:          domainServices.Network(),
		ApplicationService:      domainServices.Application(),
		Backing:                 stateShim,
		Check:                   check,
		Resources:               ctx.Resources(),
		Authorizer:              auth,
		logger:                  ctx.Logger().Child("spaces"),
	})
}
