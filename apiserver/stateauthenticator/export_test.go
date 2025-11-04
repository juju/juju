// Copyright 2014-2018 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/dbrootkeystore"
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
	controllerConfigService := a.authContext.controllerConfigService
	macaroonService := a.authContext.macaroonService

	auth, err := newExternalMacaroonAuth(ctx, externalMacaroonAuthenticatorConfig{
		controllerConfigService: controllerConfigService,
		macaroonService:         macaroonService,
		clock:                   a.authContext.clock,
		policy: dbrootkeystore.Policy{
			ExpiryDuration:   0,
			GenerateInterval: 0,
		},
		identClient: identClient,
	})
	if err != nil {
		return nil, err
	}
	return auth.Bakery, nil
}
