// Copyright 2014-2018 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/identchecker"

	"github.com/juju/juju/apiserver/authentication"
)

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
	bakeryConfigService := a.authContext.bakeryConfigService

	auth, err := newExternalMacaroonAuth(ctx, st, controllerConfigService, bakeryConfigService, a.authContext.clock, 0, identClient)
	if err != nil {
		return nil, err
	}
	return auth.Bakery, nil
}
