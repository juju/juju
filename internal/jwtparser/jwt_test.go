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

	"github.com/google/uuid"
	"github.com/juju/tc"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

type jwtParserSuite struct {
	url        string
	keySet     jwk.Set
	signingKey jwk.Key
	client     mockHTTPClient
}

var _ = tc.Suite(&jwtParserSuite{})

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
	ctx, done := context.WithCancel(context.Background())
	defer done()
	authenticator := NewParserWithHTTPClient(ctx, s.client)
	err := authenticator.SetJWKSCache(context.Background(), s.url)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *jwtParserSuite) TestCacheRegistrationFailureWithBadURL(c *tc.C) {
	ctx, done := context.WithCancel(context.Background())
	defer done()
	authenticator := NewParserWithHTTPClient(ctx, s.client)
	err := authenticator.SetJWKSCache(context.Background(), "noexisturl")
	// We want to make sure that we get an error for a bad url.
	c.Assert(err, tc.NotNil)
}

func (s *jwtParserSuite) TestParseJWT(c *tc.C) {
	ctx, done := context.WithCancel(context.Background())
	defer done()
	authenticator := NewParserWithHTTPClient(ctx, s.client)
	err := authenticator.SetJWKSCache(context.Background(), s.url)
	c.Assert(err, tc.ErrorIsNil)

	params := JWTParams{
		audience: "controller-1",
		subject:  "alice",
		claims:   map[string]string{"model-1": "read"},
	}
	jwt, err := EncodedJWT(params, s.keySet, s.signingKey)
	c.Assert(err, tc.ErrorIsNil)
	base64jwt := base64.StdEncoding.EncodeToString(jwt)

	token, err := authenticator.Parse(context.Background(), base64jwt)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(token, tc.NotNil)

	claims := token.PrivateClaims()
	c.Assert(token.Subject(), tc.Equals, "alice")
	c.Assert(token.Issuer(), tc.Equals, "test")
	c.Assert(token.Audience(), tc.DeepEquals, []string{"controller-1"})
	c.Assert(token.Expiration().After(token.IssuedAt()), tc.Equals, true)
	c.Assert(claims["access"], tc.DeepEquals, map[string]interface{}{"model-1": "read"})
}

// NewJWKSet returns a new key set and signing key.
func NewJWKSet(c *tc.C) (jwk.Set, jwk.Key) {
	jwkSet, pkeyPem := getJWKS(c)

	block, _ := pem.Decode(pkeyPem)

	pkeyDecoded, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	c.Assert(err, tc.ErrorIsNil)

	signingKey, err := jwk.FromRaw(pkeyDecoded)
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

	jwks, err := jwk.FromRaw(keySet.PublicKey)
	c.Assert(err, tc.ErrorIsNil)
	err = jwks.Set(jwk.KeyIDKey, kid.String())
	c.Assert(err, tc.ErrorIsNil)

	err = jwks.Set(jwk.KeyUsageKey, "sig")
	c.Assert(err, tc.ErrorIsNil)

	err = jwks.Set(jwk.AlgorithmKey, jwa.RS256)
	c.Assert(err, tc.ErrorIsNil)

	ks := jwk.NewSet()
	err = ks.AddKey(jwks)
	c.Assert(err, tc.ErrorIsNil)

	return ks, privateKeyPEM
}
