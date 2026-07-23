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
	"testing"

	"github.com/google/uuid"
	"github.com/juju/tc"
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
