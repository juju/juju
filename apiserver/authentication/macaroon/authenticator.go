// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package macaroon

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/names/v5"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/authentication"
)

// LocalMacaroonAuthenticator extends Authenticator with a method of
// creating a local login macaroon. The authenticator is expected to
// honour the resulting macaroon.
type LocalMacaroonAuthenticator interface {
	authentication.RequestAuthenticator

	// CreateLocalLoginMacaroon creates a macaroon that may be
	// provided to a user as proof that they have logged in with
	// a valid username and password. This macaroon may then be
	// used to obtain a discharge macaroon so that the user can
	// log in without presenting their password for a set amount
	// of time.
	CreateLocalLoginMacaroon(context.Context, names.UserTag, bakery.Version) (*macaroon.Macaroon, error)
}
