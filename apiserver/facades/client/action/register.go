// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"reflect"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegisterWithAuth("Action", 7, authorizeCheck, func(ctx facade.Context) (facade.Facade, error) {
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

func authorizeCheck(auth facade.Authorizer) error {
	if !auth.AuthClient() {
		return apiservererrors.ErrPerm
	}
	return nil
}
