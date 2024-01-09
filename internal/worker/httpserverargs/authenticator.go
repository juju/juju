// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserverargs

import (
	"context"
	coreuser "github.com/juju/juju/core/user"

	"github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/authentication/macaroon"
	"github.com/juju/juju/apiserver/stateauthenticator"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/state"
)

// ControllerConfigGetter is an interface that can be implemented by
// types that can return a controller config.
type ControllerConfigGetter interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

type UserService interface {
	GetUserByName(ctx context.Context, name string) (coreuser.User, error)
}

// NewStateAuthenticatorFunc is a function type satisfied by
// NewStateAuthenticator.
type NewStateAuthenticatorFunc func(
	statePool *state.StatePool,
	controllerConfigGetter ControllerConfigGetter,
	userService UserService,
	mux *apiserverhttp.Mux,
	clock clock.Clock,
	abort <-chan struct{},
) (macaroon.LocalMacaroonAuthenticator, error)

// NewStateAuthenticator returns a new LocalMacaroonAuthenticator that
// authenticates users and agents using the given state pool. The
// authenticator will register handlers into the mux for dealing with
// local macaroon logins.
func NewStateAuthenticator(
	statePool *state.StatePool,
	controllerConfigGetter ControllerConfigGetter,
	userService UserService,
	mux *apiserverhttp.Mux,
	clock clock.Clock,
	abort <-chan struct{},
) (macaroon.LocalMacaroonAuthenticator, error) {
	stateAuthenticator, err := stateauthenticator.NewAuthenticator(statePool, controllerConfigGetter, userService, clock)
	if err != nil {
		return nil, errors.Trace(err)
	}
	errH := stateAuthenticator.AddHandlers(mux)
	if errH != nil {
		return nil, errors.Trace(errH)
	}
	go stateAuthenticator.Maintain(abort)
	return stateAuthenticator, nil
}
