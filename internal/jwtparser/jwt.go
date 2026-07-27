// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jwtparser

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"

	"github.com/juju/errors"
	"github.com/lestrrat-go/httprc/v3"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

// Parser is a JWT parser responsible for parsing JWT tokens.
type Parser struct {
	cache      *jwk.Cache
	httpClient jwk.HTTPClient
	mu         sync.RWMutex
	refreshURL string
}

// NewParserWithHTTPClient creates a new JWT parser with a custom http client.
// The parser holds a cache with routines that will terminate when the context
// is done.
func NewParserWithHTTPClient(ctx context.Context, client httprc.HTTPClient) (*Parser, error) {
	httpClient := httprc.NewClient(
		httprc.WithHTTPClient(client),
	)
	cache, err := jwk.NewCache(
		ctx,
		httpClient,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &Parser{
		httpClient: client,
		cache:      cache,
	}, nil
}

// Parse parses a base64 encoded string into a jwt token.
// It will return a NotProvisioned error if SetJWKSCache
// has not been run on the parser.
func (j *Parser) Parse(ctx context.Context, tok string) (jwt.Token, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	if j.refreshURL == "" {
		return nil, errors.NotProvisionedf("no refresh url configured")
	}
	tokBytes, err := base64.StdEncoding.DecodeString(tok)
	if err != nil {
		return nil, errors.Annotate(err, "invalid jwt authToken in request")
	}

	jwkSet, err := j.cache.Lookup(ctx, j.refreshURL)
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

// SetJWKSCache sets up the token key cache and refreshes the public key.
func (j *Parser) SetJWKSCache(ctx context.Context, refreshURL string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.refreshURL == refreshURL {
		return nil
	}
	// Use WithWaitReady(false) to match the non-blocking behaviour of the
	// former jwk.Cache (v2).
	err := j.cache.Register(ctx, refreshURL,
		jwk.WithHTTPClient(j.httpClient),
		jwk.WithWaitReady(false),
	)
	if err != nil {
		return fmt.Errorf("registering jwk cache with url %q: %w", refreshURL, err)
	}
	j.refreshURL = refreshURL
	return nil
}
