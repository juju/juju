// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserverargs

import (
	"github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/httpcontext"
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
) (httpcontext.LocalMacaroonAuthenticator, error)

// NewStateAuthenticator returns a new LocalMacaroonAuthenticator that
// authenticates users and agents using the given state pool. The
// authenticator will register handlers into the mux for dealing with
// local macaroon logins.
func NewStateAuthenticator(
	statePool *state.StatePool,
	mux *apiserverhttp.Mux,
	clock clock.Clock,
	abort <-chan struct{},
) (httpcontext.LocalMacaroonAuthenticator, error) {
	stateAuthenticator, err := stateauthenticator.NewAuthenticator(statePool, clock)
	if err != nil {
		return nil, errors.Trace(err)
	}
	stateAuthenticator.AddHandlers(mux)
	go stateAuthenticator.Maintain(abort)
	return stateAuthenticator, nil
}
