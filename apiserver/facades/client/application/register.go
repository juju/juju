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
	registry.MustRegister("Application", 19, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV19(ctx) // Added new DeployFromRepository
	}, reflect.TypeOf((*APIv19)(nil)))
}

func newFacadeV19(ctx facade.Context) (*APIv19, error) {
	api, err := newFacadeBase(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv19{api}, nil
}
