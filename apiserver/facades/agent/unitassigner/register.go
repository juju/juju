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
	registry.MustRegister("UnitAssigner", 1, func(stdCtx context.Context, ctx facade.Context) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newFacade returns a new unitAssigner api instance.
func newFacade(ctx facade.Context) (*API, error) {
	st := ctx.State()
	setter := common.NewStatusSetter(&common.UnitAgentFinder{EntityFinder: st}, common.AuthAlways())
	return &API{
		st:           st,
		res:          ctx.Resources(),
		statusSetter: setter,
	}, nil
}
