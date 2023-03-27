// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/names/v4"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/state"
)

// AuthParams holds the info used to authenticate a login request.
type AuthParams struct {
	// These are used for user or agent auth.
	AuthTag     names.Tag
	Credentials string

	// Token is used for rebac based auth.
	Token jwt.Token

	// None is used for agent auth.
	Nonce string

	// These are used for macaroon auth.
	Macaroons     []macaroon.Slice
	BakeryVersion bakery.Version
}

// Authenticator is the interface all entity authenticators need to implement
// to authenticate juju entities.
type Authenticator interface {
	// Authenticate authenticates the given entity.
	Authenticate(ctx context.Context, entityFinder EntityFinder, authParams AuthParams) (state.Entity, error)
}

// EntityFinder finds the entity described by the tag.
type EntityFinder interface {
	FindEntity(tag names.Tag) (state.Entity, error)
}

// TokenParser parses a jwt token returning the token and
// entity derived from the token subject.
type TokenParser interface {
	Parse(ctx context.Context, tok string) (jwt.Token, state.Entity, error)
}
