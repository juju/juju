// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"encoding/base64"
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

// NewEncodedJWT returns a base64-encoded JWT token string ready for use
// in a LoginRequest.Token field.
func NewEncodedJWT(params JWTParams) (string, error) {
	tok, err := NewJWT(params)
	if err != nil {
		return "", err
	}
	serialized, err := jwt.NewSerializer().Serialize(tok)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(serialized), nil
}

// InsecureJWTParser implements the TokenParser interface defined in
// apiserver/authentication/jwt for testing purposes.
// It decodes base64 JWT tokens and parses them without signature verification.
type InsecureJWTParser struct{}

// Parse implements TokenParser.
func (p *InsecureJWTParser) Parse(_ context.Context, tok string) (jwt.Token, error) {
	data, err := base64.StdEncoding.DecodeString(tok)
	if err != nil {
		return nil, err
	}
	return jwt.ParseInsecure(data)
}
