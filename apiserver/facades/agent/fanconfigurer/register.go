// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fanconfigurer

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("FanConfigurer", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFanConfigurerAPIV1(ctx)
	}, reflect.TypeOf((*FanConfigurerAPIV1)(nil)))
	registry.MustRegister("FanConfigurer", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newFanConfigurerAPI(ctx)
	}, reflect.TypeOf((*FanConfigurerAPI)(nil)))
}

// newFanConfigurerAPIV1 creates a new FanConfigurer API V1 endpoint on server-side.
func newFanConfigurerAPIV1(ctx facade.Context) (*FanConfigurerAPIV1, error) {
	api, err := newFanConfigurerAPI(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &FanConfigurerAPIV1{FanConfigurerAPI: api}, nil
}

// newFanConfigurerAPI creates a new FanConfigurer API endpoint on server-side.
func newFanConfigurerAPI(ctx facade.Context) (*FanConfigurerAPI, error) {
	model, err := ctx.State().Model()
	if err != nil {
		return nil, err
	}
	return NewFanConfigurerAPIForModel(model, stateShim{ctx.State()}, ctx.Resources(), ctx.Auth())
}
