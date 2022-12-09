// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Application", 15, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV15(ctx)
	}, reflect.TypeOf((*APIv15)(nil)))
	registry.MustRegister("Application", 16, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV16(ctx) // DestroyApplication & DestroyUnit gains dry-run
	}, reflect.TypeOf((*APIv16)(nil)))
}

func newFacadeV16(ctx facade.Context) (*APIv16, error) {
	api, err := newFacadeBase(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv16{api}, nil
}

func newFacadeV15(ctx facade.Context) (*APIv15, error) {
	api, err := newFacadeV16(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv15{api}, nil
}
