// Copyright 2014-2018 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator

import (
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/names/v4"
)

// TODO update the tests moved from apiserver to test via the public
// interface, and then get rid of these.
func EntityAuthenticator(authenticator *Authenticator, tag names.Tag) (authentication.EntityAuthenticator, error) {
	return authenticator.authContext.authenticator("testing.invalid:1234").authenticatorForTag(tag)
}

func ServerBakery(a *Authenticator, identClient identchecker.IdentityClient) (*identchecker.Bakery, error) {
	auth, err := a.authContext.externalMacaroonAuth(identClient)
	if err != nil {
		return nil, err
	}
	return auth.(*authentication.ExternalMacaroonAuthenticator).Bakery, nil
}
