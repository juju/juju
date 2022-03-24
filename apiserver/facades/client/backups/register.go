// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Backups", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*API)(nil)))
	registry.MustRegister("Backups", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV2(ctx)
	}, reflect.TypeOf((*APIv2)(nil)))
}

// newFacadeV2 provides the required signature for version 2 facade registration.
func newFacadeV2(ctx facade.Context) (*APIv2, error) {
	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewAPIv2(&stateShim{st, model}, ctx.Resources(), ctx.Auth())
}

// newFacade provides the required signature for facade registration.
func newFacade(ctx facade.Context) (*API, error) {
	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewAPI(&stateShim{st, model}, ctx.Resources(), ctx.Auth())
}
