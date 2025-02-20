// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"time"

	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// JWTParams are the necessary params to issue a ready-to-go JWT.
type JWTParams struct {
	Controller string
	User       string
	Access     any
}

// NewJWT returns a JWT token.
func NewJWT(params JWTParams) (jwt.Token, error) {
	return jwt.NewBuilder().
		Audience([]string{params.Controller}).
		Subject(params.User).
		Issuer("test").
		JwtID(uuid.NewString()).
		Claim("access", params.Access).
		Expiration(time.Now().Add(time.Hour)).
		Build()
}
