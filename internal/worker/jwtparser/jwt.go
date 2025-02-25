// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jwtparser

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/juju/errors"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// JWTParser is a parser responsible for parsing JWT tokens.
type JWTParser struct {
	cache      *jwk.Cache
	httpClient HTTPClient
	refreshURL string
}

// DefaultHTTPClient returns a defaulthttp client
// that follows redirects with a sensible timeout.
func DefaultHTTPClient() HTTPClient {
	return &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// NewParserWithHTTPClient creates a new JWT parser with a custom http client.
func NewParserWithHTTPClient(
	client HTTPClient,
	refreshURL string,
) *JWTParser {
	return &JWTParser{
		httpClient: client,
		refreshURL: refreshURL,
	}
}

// Parse parses a base64 encoded string into a jwt token.
func (j *JWTParser) Parse(ctx context.Context, tok string) (jwt.Token, error) {
	if j == nil || j.refreshURL == "" {
		return nil, errors.New("no jwt authToken parser configured")
	}
	tokBytes, err := base64.StdEncoding.DecodeString(tok)
	if err != nil {
		return nil, errors.Annotate(err, "invalid jwt authToken in request")
	}

	jwkSet, err := j.cache.Get(ctx, j.refreshURL)
	if err != nil {
		return nil, errors.Annotate(err, "refreshing jwt key")
	}

	jwtTok, err := jwt.Parse(
		tokBytes,
		jwt.WithKeySet(jwkSet),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return jwtTok, err
}

// RegisterJWKSCache sets up the token key cache and refreshes the public key.
func (j *JWTParser) RegisterJWKSCache(ctx context.Context) error {
	j.cache = jwk.NewCache(ctx)

	err := j.cache.Register(j.refreshURL, jwk.WithHTTPClient(j.httpClient))
	if err != nil {
		return fmt.Errorf("registering jwk cache with url %q: %w", j.refreshURL, err)
	}
	_, err = j.cache.Refresh(ctx, j.refreshURL)
	if err != nil {
		return fmt.Errorf("refreshing jwk cache at %q: %w", j.refreshURL, err)
	}
	return nil
}
