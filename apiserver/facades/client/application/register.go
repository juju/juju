// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/v3/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Application", 13, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV13(ctx)
	}, reflect.TypeOf((*APIv13)(nil))) // Adds CharmOrigin to Deploy
}

func newFacadeV13(ctx facade.Context) (*APIv13, error) {
	api, err := newFacadeBase(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv13{api}, nil
}
