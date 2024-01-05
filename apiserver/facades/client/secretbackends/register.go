// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"context"
	"reflect"

	"github.com/juju/clock"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("SecretBackends", 1, func(stdCtx context.Context, ctx facade.Context) (facade.Facade, error) {
		return newSecretBackendsAPI(ctx)
	}, reflect.TypeOf((*SecretBackendsAPI)(nil)))
}

// newSecretBackendsAPI creates a SecretBackendsAPI.
func newSecretBackendsAPI(context facade.Context) (*SecretBackendsAPI, error) {
	if !context.Auth().AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &SecretBackendsAPI{
		authorizer:     context.Auth(),
		controllerUUID: context.State().ControllerUUID(),
		clock:          clock.WallClock,
		backendState:   state.NewSecretBackends(context.State()),
		secretState:    state.NewSecrets(context.State()),
		statePool:      &statePoolShim{pool: context.StatePool()},
	}, nil
}
