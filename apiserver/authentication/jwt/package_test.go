// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jwt_test

import (
	"context"
	"encoding/base64"
	. "testing"
	"time"

	"github.com/google/uuid"
	"github.com/juju/errors"
	"github.com/lestrrat-go/jwx/v2/jwt"
	gc "gopkg.in/check.v1"

	jwtauth "github.com/juju/juju/apiserver/authentication/jwt"
)

func TestPackage(t *T) {
	gc.TestingT(t)
}

type testJWTParser struct{}

func (m *testJWTParser) Parse(ctx context.Context, tok string) (jwt.Token, error) {
	data, err := base64.StdEncoding.DecodeString(tok)
	if err != nil {
		return nil, err
	}
	return jwt.ParseInsecure(data)
}

type testParserGetter struct {
	parserUnavailable bool
}

func (t testParserGetter) Get() (jwtauth.TokenParser, bool) {
	if t.parserUnavailable {
		return nil, false
	}
	return &testJWTParser{}, true
}

// JWTParams are the necessary params to issue a ready-to-go JWT.
type JWTParams struct {
	Controller string
	User       string
	Access     map[string]string
}

// EncodedJWT returns jwt as bytes signed by the specified key.
func EncodedJWT(params JWTParams) ([]byte, error) {
	token, err := jwt.NewBuilder().
		Audience([]string{params.Controller}).
		Subject(params.User).
		Issuer("test").
		JwtID(uuid.NewString()).
		Claim("access", params.Access).
		Expiration(time.Now().Add(time.Hour)).
		Build()
	if err != nil {
		return nil, errors.Trace(err)
	}

	freshToken, err := jwt.NewSerializer().Serialize(token)
	return freshToken, errors.Trace(err)
}
