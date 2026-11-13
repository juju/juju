// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jwtparser

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/google/uuid"
	"github.com/juju/tc"
	"github.com/lestrrat-go/httprc/v3"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
)

type jwtParserSuite struct {
	url        string
	keySet     jwk.Set
	signingKey jwk.Key
	client     mockHTTPClient
}

func TestJwtParserSuite(t *testing.T) {
	tc.Run(t, &jwtParserSuite{})
}

func (s *jwtParserSuite) SetUpTest(c *tc.C) {
	s.keySet, s.signingKey = NewJWKSet(c)
	s.url = "fakeurl.com/keys"

	buf := bytes.Buffer{}
	pub, _ := s.keySet.Key(0)
	_ = json.NewEncoder(&buf).Encode(pub)
	s.client = mockHTTPClient{
		keys: buf.String(),
		url:  s.url,
	}
}

func (s *jwtParserSuite) TestCacheRegistration(c *tc.C) {
	ctx, done := context.WithCancel(c.Context())
	defer done()
	authenticator, err := NewParserWithHTTPClient(ctx, s.client)
	c.Assert(err, tc.ErrorIsNil)
	err = authenticator.SetJWKSCache(c.Context(), s.url)
	c.Assert(err, tc.ErrorIsNil)

	resource, err := authenticator.cache.LookupResource(ctx, s.url)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(resource.MinInterval(), tc.Equals, 30*time.Second)
	c.Check(resource.MaxInterval(), tc.Equals, httprc.DefaultMaxInterval)
}

func (s *jwtParserSuite) TestCacheRefreshIntervals(c *tc.C) {
	tests := []struct {
		name             string
		statusCode       int
		cacheControl     string
		requestError     error
		expectError      bool
		expectedInterval time.Duration
	}{
		{
			name:             "connection failure",
			requestError:     fmt.Errorf("endpoint unavailable"),
			expectError:      true,
			expectedInterval: 30 * time.Second,
		},
		{
			name:             "http failure",
			statusCode:       http.StatusInternalServerError,
			expectError:      true,
			expectedInterval: 30 * time.Second,
		},
		{
			name:             "success",
			statusCode:       http.StatusOK,
			cacheControl:     "max-age=600",
			expectedInterval: 10 * time.Minute,
		},
	}
	for _, test := range tests {
		c.Logf("scenario: %s", test.name)
		ctrl := gomock.NewController(c)
		defer ctrl.Finish()
		client := NewMockHTTPClient(ctrl)
		client.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
			if test.requestError != nil {
				return nil, test.requestError
			}
			return &http.Response{
				StatusCode: test.statusCode,
				Header:     http.Header{"Cache-Control": []string{test.cacheControl}},
				Body:       io.NopCloser(strings.NewReader(s.client.keys)),
				Request:    req,
			}, nil
		}).MinTimes(1)
		ctx, cancel := context.WithCancel(c.Context())
		defer cancel()
		parser, err := NewParserWithHTTPClient(ctx, client)
		c.Assert(err, tc.ErrorIsNil)
		err = parser.SetJWKSCache(ctx, s.url)
		c.Assert(err, tc.ErrorIsNil)
		resource, err := parser.cache.LookupResource(ctx, s.url)
		c.Assert(err, tc.ErrorIsNil)
		err = parser.cache.Shutdown(ctx)
		c.Assert(err, tc.ErrorIsNil)

		before := time.Now()
		err = resource.Sync(c.Context())
		if test.expectError {
			c.Check(err, tc.NotNil)
		} else {
			c.Check(err, tc.ErrorIsNil)
		}
		c.Check(resource.Next().Before(before.Add(test.expectedInterval)), tc.IsFalse)
		c.Check(resource.Next().After(time.Now().Add(test.expectedInterval)), tc.IsFalse)
	}
}

func (s *jwtParserSuite) TestCacheRegistrationSucceedsWithBadURL(c *tc.C) {
	ctx, done := context.WithCancel(c.Context())
	defer done()
	authenticator, err := NewParserWithHTTPClient(ctx, s.client)
	c.Assert(err, tc.ErrorIsNil)
	err = authenticator.SetJWKSCache(c.Context(), "noexisturl")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(authenticator.refreshURL, tc.Equals, "noexisturl")
}

func (s *jwtParserSuite) TestParseJWT(c *tc.C) {
	ctx, done := context.WithCancel(c.Context())
	defer done()
	authenticator, err := NewParserWithHTTPClient(ctx, s.client)
	c.Assert(err, tc.ErrorIsNil)
	err = authenticator.SetJWKSCache(c.Context(), s.url)
	c.Assert(err, tc.ErrorIsNil)

	// SetJWKSCache is non-blocking; force an initial synchronous fetch so the
	// cache is ready before we call Parse.
	_, err = authenticator.cache.Refresh(c.Context(), s.url)
	c.Assert(err, tc.ErrorIsNil)

	params := JWTParams{
		audience: "controller-1",
		subject:  "alice",
		claims:   map[string]string{"model-1": "read"},
	}
	jwt, err := EncodedJWT(params, s.keySet, s.signingKey)
	c.Assert(err, tc.ErrorIsNil)
	base64jwt := base64.StdEncoding.EncodeToString(jwt)

	token, err := authenticator.Parse(c.Context(), base64jwt)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(token, tc.NotNil)

	subject, ok := token.Subject()
	c.Assert(ok, tc.IsTrue)
	c.Assert(subject, tc.Equals, "alice")
	issuer, ok := token.Issuer()
	c.Assert(ok, tc.IsTrue)
	c.Assert(issuer, tc.Equals, "test")
	audience, ok := token.Audience()
	c.Assert(ok, tc.IsTrue)
	c.Assert(audience, tc.DeepEquals, []string{"controller-1"})
	expiration, ok := token.Expiration()
	c.Assert(ok, tc.IsTrue)
	issuedAt, ok := token.IssuedAt()
	c.Assert(ok, tc.IsTrue)
	c.Assert(expiration.After(issuedAt), tc.Equals, true)
	var access map[string]any
	err = token.Get("access", &access)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.DeepEquals, map[string]any{"model-1": "read"})
}

// NewJWKSet returns a new key set and signing key.
func NewJWKSet(c *tc.C) (jwk.Set, jwk.Key) {
	jwkSet, pkeyPem := getJWKS(c)

	block, _ := pem.Decode(pkeyPem)

	pkeyDecoded, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	c.Assert(err, tc.ErrorIsNil)

	signingKey, err := jwk.Import(pkeyDecoded)
	c.Assert(err, tc.ErrorIsNil)

	return jwkSet, signingKey
}

func getJWKS(c *tc.C) (jwk.Set, []byte) {
	keySet, err := rsa.GenerateKey(rand.Reader, 4096)
	c.Assert(err, tc.ErrorIsNil)

	privateKeyPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(keySet),
		},
	)

	kid, err := uuid.NewRandom()
	c.Assert(err, tc.ErrorIsNil)

	jwks, err := jwk.Import(keySet.PublicKey)
	c.Assert(err, tc.ErrorIsNil)
	err = jwks.Set(jwk.KeyIDKey, kid.String())
	c.Assert(err, tc.ErrorIsNil)

	err = jwks.Set(jwk.KeyUsageKey, "sig")
	c.Assert(err, tc.ErrorIsNil)

	err = jwks.Set(jwk.AlgorithmKey, jwa.RS256())
	c.Assert(err, tc.ErrorIsNil)

	ks := jwk.NewSet()
	err = ks.AddKey(jwks)
	c.Assert(err, tc.ErrorIsNil)

	return ks, privateKeyPEM
}
