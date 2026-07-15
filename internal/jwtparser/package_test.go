// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jwtparser

import (
	"io"
	"net/http"
	"strings"
	. "testing"
	"time"

	"github.com/google/uuid"
	"github.com/juju/errors"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
	gc "gopkg.in/check.v1"
)

func TestPackage(t *T) {
	gc.TestingT(t)
}

type mockHTTPClient struct {
	url  string
	keys string
}

func (m mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if req.URL.String() != m.url {
		return nil, errors.New("not found")
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(m.keys)),
		Request:    req,
	}, nil
}

// JWTParams are the necessary params to issue a ready-to-go JWT.
type JWTParams struct {
	audience string
	subject  string
	claims   map[string]string
}

// EncodedJWT returns jwt as bytes signed by the specified key.
func EncodedJWT(params JWTParams, jwkSet jwk.Set, signingKey jwk.Key) ([]byte, error) {
	pubKey, ok := jwkSet.Key(jwkSet.Len() - 1)
	if !ok {
		return nil, errors.Errorf("no jwk found")
	}

	err := signingKey.Set(jwk.AlgorithmKey, jwa.RS256())
	if err != nil {
		return nil, errors.Trace(err)
	}
	key, ok := pubKey.KeyID()
	if !ok {
		return nil, errors.Errorf("no public key id")
	}
	err = signingKey.Set(jwk.KeyIDKey, key)
	if err != nil {
		return nil, errors.Trace(err)
	}

	token, err := jwt.NewBuilder().
		Audience([]string{params.audience}).
		Subject(params.subject).
		Issuer("test").
		JwtID(uuid.NewString()).
		Claim("access", params.claims).
		IssuedAt(time.Now()).
		Expiration(time.Now().Add(time.Hour)).
		Build()
	if err != nil {
		return nil, errors.Trace(err)
	}

	freshToken, err := jwt.Sign(
		token,
		jwt.WithKey(
			jwa.RS256(),
			signingKey,
		),
	)
	return freshToken, errors.Trace(err)
}
