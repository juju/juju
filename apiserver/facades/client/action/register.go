// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Action", 7, func(stdCtx context.Context, ctx facade.Context) (facade.Facade, error) {
		return newActionAPIV7(ctx)
	}, reflect.TypeOf((*APIv7)(nil)))
}

// newActionAPIV7 returns an initialized ActionAPI for version 7.
func newActionAPIV7(ctx facade.Context) (*APIv7, error) {
	api, err := newActionAPI(&stateShim{st: ctx.State()}, ctx.Resources(), ctx.Auth(), ctx.LeadershipReader)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv7{api}, nil
}
