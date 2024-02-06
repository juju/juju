// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jwt_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/juju/errors"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
	gc "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

// JWTParams are the necessary params to issue a ready-to-go JWT.
type JWTParams struct {
	Controller string
	User       string
	Access     map[string]string
}

// EncodedJWT returns jwt as bytes signed by the specified key.
func EncodedJWT(params JWTParams, jwkSet jwk.Set, signingKey jwk.Key) ([]byte, error) {
	jti, err := generateJTI()
	if err != nil {
		return nil, errors.Trace(err)
	}
	pubKey, ok := jwkSet.Key(jwkSet.Len() - 1)
	if !ok {
		return nil, errors.Errorf("no jwk found")
	}

	err = signingKey.Set(jwk.AlgorithmKey, jwa.RS256)
	if err != nil {
		return nil, errors.Trace(err)
	}
	err = signingKey.Set(jwk.KeyIDKey, pubKey.KeyID())
	if err != nil {
		return nil, errors.Trace(err)
	}

	token, err := jwt.NewBuilder().
		Audience([]string{params.Controller}).
		Subject(params.User).
		Issuer("test").
		JwtID(jti).
		Claim("access", params.Access).
		Expiration(time.Now().Add(time.Hour)).
		Build()
	if err != nil {
		return nil, errors.Trace(err)
	}

	freshToken, err := jwt.Sign(
		token,
		jwt.WithKey(
			jwa.RS256,
			signingKey,
		),
	)
	return freshToken, errors.Trace(err)
}

func generateJTI() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
