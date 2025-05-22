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
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

type mockHTTPClient struct {
	url  string
	keys string
}

func (m mockHTTPClient) Get(url string) (*http.Response, error) {
	if url != m.url {
		return nil, errors.New("not found")
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(m.keys)),
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

	err := signingKey.Set(jwk.AlgorithmKey, jwa.RS256)
	if err != nil {
		return nil, errors.Trace(err)
	}
	err = signingKey.Set(jwk.KeyIDKey, pubKey.KeyID())
	if err != nil {
		return nil, errors.Trace(err)
	}

	token, err := jwt.NewBuilder().
		Audience([]string{params.audience}).
		Subject(params.subject).
		Issuer("test").
		JwtID(uuid.NewString()).
		Claim("access", params.claims).
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
