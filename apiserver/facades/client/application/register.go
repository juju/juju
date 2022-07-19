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
	registry.MustRegister("Application", 14, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV14(ctx)
	}, reflect.TypeOf((*APIv14)(nil)))
}

func newFacadeV14(ctx facade.Context) (*APIv14, error) {
	api, err := newFacadeBase(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv14{api}, nil
}
