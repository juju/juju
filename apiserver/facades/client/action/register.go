// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/v3/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Action", 7, func(ctx facade.Context) (facade.Facade, error) {
		return newActionAPIV7(ctx)
	}, reflect.TypeOf((*APIv7)(nil)))
}

// newActionAPIV7 returns an initialized ActionAPI for version 7.
func newActionAPIV7(ctx facade.Context) (*APIv7, error) {
	st := ctx.State()
	api, err := newActionAPI(&stateShim{st: st}, ctx.Resources(), ctx.Auth())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv7{api}, nil
}
