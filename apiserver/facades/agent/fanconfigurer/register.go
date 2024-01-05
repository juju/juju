// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fanconfigurer

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("FanConfigurer", 1, func(stdCtx context.Context, ctx facade.Context) (facade.Facade, error) {
		return newFanConfigurerAPI(ctx)
	}, reflect.TypeOf((*FanConfigurerAPI)(nil)))
}

// newFanConfigurerAPI creates a new FanConfigurer API endpoint on server-side.
func newFanConfigurerAPI(ctx facade.Context) (*FanConfigurerAPI, error) {
	model, err := ctx.State().Model()
	if err != nil {
		return nil, err
	}
	return NewFanConfigurerAPIForModel(model, ctx.Resources(), ctx.Auth())
}
