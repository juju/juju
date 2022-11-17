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
}

func newFacadeV15(ctx facade.Context) (*APIv15, error) {
	api, err := newFacadeBase(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv15{api}, nil
}
