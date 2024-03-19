// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserverargs

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/authentication/macaroon"
	"github.com/juju/juju/apiserver/stateauthenticator"
	"github.com/juju/juju/controller"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/state"
)

// ControllerConfigService is an interface that can be implemented by
// types that can return a controller config.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// UserService is the interface that wraps the methods required to
// authenticate a user.
type UserService interface {
	// GetUserByAuth returns the user with the given name and password.
	GetUserByAuth(ctx context.Context, name string, password auth.Password) (coreuser.User, error)
	// GetUserByName returns the user with the given name.
	GetUserByName(ctx context.Context, name string) (coreuser.User, error)
	// UpdateLastLogin updates the last login time for the user with the
	// given name.
	UpdateLastLogin(ctx context.Context, name string) error
}

// NewStateAuthenticatorFunc is a function type satisfied by
// NewStateAuthenticator.
type NewStateAuthenticatorFunc func(
	statePool *state.StatePool,
	controllerConfigService ControllerConfigService,
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
	controllerConfigService ControllerConfigService,
	userService UserService,
	mux *apiserverhttp.Mux,
	clock clock.Clock,
	abort <-chan struct{},
) (macaroon.LocalMacaroonAuthenticator, error) {
	systemState, err := statePool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	agentAuthFactory := authentication.NewAgentAuthenticatorFactory(systemState, nil)
	stateAuthenticator, err := stateauthenticator.NewAuthenticator(statePool, systemState, controllerConfigService, userService, agentAuthFactory, clock)
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
