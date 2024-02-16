// Copyright 2014-2018 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/identchecker"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/authentication"
)

// TODO update the tests moved from apiserver to test via the public
// interface, and then get rid of these.
func EntityAuthenticator(ctx context.Context, authenticator *Authenticator, tag names.Tag) (authentication.EntityAuthenticator, error) {
	return authenticator.authContext.authenticator("testing.invalid:1234").authenticatorForTag(ctx, tag)
}

func ServerBakery(ctx context.Context, a *Authenticator, identClient identchecker.IdentityClient) (*identchecker.Bakery, error) {
	auth, err := a.authContext.externalMacaroonAuth(ctx, identClient)
	if err != nil {
		return nil, err
	}
	return auth.(*authentication.ExternalMacaroonAuthenticator).Bakery, nil
}

func ServerBakeryExpiresImmediately(ctx context.Context, a *Authenticator, identClient identchecker.IdentityClient) (*identchecker.Bakery, error) {
	st := a.authContext.st
	controllerConfigService := a.authContext.controllerConfigService

	auth, err := newExternalMacaroonAuth(ctx, st, controllerConfigService, a.authContext.clock, 0, identClient)
	if err != nil {
		return nil, err
	}
	return auth.Bakery, nil
}
