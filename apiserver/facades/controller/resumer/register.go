// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resumer

import (
	"reflect"

	apiservererrors "github.com/juju/juju/v3/apiserver/errors"
	"github.com/juju/juju/v3/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Resumer", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newResumerAPI(ctx)
	}, reflect.TypeOf((*ResumerAPI)(nil)))
}

// newResumerAPI creates a new instance of the Resumer API.
func newResumerAPI(ctx facade.Context) (*ResumerAPI, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	return &ResumerAPI{
		st:   getState(ctx.State()),
		auth: authorizer,
	}, nil
}
