// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserverargs

import (
	"github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/authentication/macaroon"
	"github.com/juju/juju/apiserver/stateauthenticator"
	"github.com/juju/juju/state"
)

// NewStateAuthenticatorFunc is a function type satisfied by
// NewStateAuthenticator.
type NewStateAuthenticatorFunc func(
	statePool *state.StatePool,
	mux *apiserverhttp.Mux,
	clock clock.Clock,
	abort <-chan struct{},
	ctrlConfigService ControllerConfigService,
) (macaroon.LocalMacaroonAuthenticator, error)

// NewStateAuthenticator returns a new LocalMacaroonAuthenticator that
// authenticates users and agents using the given state pool. The
// authenticator will register handlers into the mux for dealing with
// local macaroon logins.
func NewStateAuthenticator(
	statePool *state.StatePool,
	mux *apiserverhttp.Mux,
	clock clock.Clock,
	abort <-chan struct{},
	ctrlConfigService ControllerConfigService,
) (macaroon.LocalMacaroonAuthenticator, error) {
	// TODO(anvial): ctrlConfigService will be needed in stateauthenticator.NewAuthenticator after controller config
	// service will be injected in apiserver (auth.go)
	stateAuthenticator, err := stateauthenticator.NewAuthenticator(statePool, clock)
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
