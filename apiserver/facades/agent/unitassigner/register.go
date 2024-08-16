// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("UnitAssigner", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacade(stdCtx, ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newFacade returns a new unitAssigner api instance.
func newFacade(stdCtx context.Context, ctx facade.ModelContext) (*API, error) {
	st := ctx.State()

	serviceFactory := ctx.ServiceFactory()

	prechecker, err := ctx.GetProvider(stdCtx)
	if err != nil {
		return nil, err
	}

	setter := common.NewStatusSetter(&common.UnitAgentFinder{EntityFinder: st}, common.AuthAlways())
	return &API{
		st:             stateShim{State: st, prechecker: prechecker},
		machineService: serviceFactory.Machine(),
		networkService: serviceFactory.Network(),
		res:            ctx.Resources(),
		statusSetter:   setter,
	}, nil
}
