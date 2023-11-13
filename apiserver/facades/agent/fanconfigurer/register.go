// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fanconfigurer

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/facades"
)

// FacadesVersions returns the versions of the facades that this package
// implements.
func FacadesVersions() facades.NamedFacadeVersion {
	return facades.NamedFacadeVersion{
		Name:     "FanConfigurer",
		Versions: facades.FacadeVersion{1},
	}
}

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("FanConfigurer", 1, func(ctx facade.Context) (facade.Facade, error) {
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
