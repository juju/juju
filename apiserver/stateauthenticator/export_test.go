// Copyright 2014-2018 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator

import (
	"gopkg.in/juju/names.v3"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/authentication"
)

// TODO update the tests moved from apiserver to test via the public
// interface, and then get rid of these.
func EntityAuthenticator(authenticator *Authenticator, tag names.Tag) (authentication.EntityAuthenticator, error) {
	return authenticator.authContext.authenticator("testing.invalid:1234").authenticatorForTag(tag)
}

func ServerMacaroon(a *Authenticator) (*macaroon.Macaroon, error) {
	auth, err := a.authContext.externalMacaroonAuth()
	if err != nil {
		return nil, err
	}
	return auth.(*authentication.ExternalMacaroonAuthenticator).Macaroon, nil
}

func ServerBakeryService(a *Authenticator) (authentication.BakeryService, error) {
	auth, err := a.authContext.externalMacaroonAuth()
	if err != nil {
		return nil, err
	}
	return auth.(*authentication.ExternalMacaroonAuthenticator).Service, nil
}
