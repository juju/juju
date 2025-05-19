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
	jc "github.com/juju/testing/checkers"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	gc "gopkg.in/check.v1"
)

type jwtParserSuite struct {
	url        string
	keySet     jwk.Set
	signingKey jwk.Key
	client     mockHTTPClient
}

var _ = gc.Suite(&jwtParserSuite{})

func (s *jwtParserSuite) SetUpTest(c *gc.C) {
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

func (s *jwtParserSuite) TestCacheRegistration(c *gc.C) {
	ctx, done := context.WithCancel(context.Background())
	defer done()
	authenticator := NewParserWithHTTPClient(ctx, s.client)
	err := authenticator.SetJWKSCache(context.Background(), s.url)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *jwtParserSuite) TestCacheRegistrationSucceedsWithBadURL(c *gc.C) {
	ctx, done := context.WithCancel(context.Background())
	defer done()
	authenticator := NewParserWithHTTPClient(ctx, s.client)
	err := authenticator.SetJWKSCache(context.Background(), "noexisturl")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(authenticator.refreshURL, gc.Equals, "noexisturl")
}

func (s *jwtParserSuite) TestParseJWT(c *gc.C) {
	ctx, done := context.WithCancel(context.Background())
	defer done()
	authenticator := NewParserWithHTTPClient(ctx, s.client)
	err := authenticator.SetJWKSCache(context.Background(), s.url)
	c.Assert(err, jc.ErrorIsNil)

	params := JWTParams{
		audience: "controller-1",
		subject:  "alice",
		claims:   map[string]string{"model-1": "read"},
	}
	jwt, err := EncodedJWT(params, s.keySet, s.signingKey)
	c.Assert(err, jc.ErrorIsNil)
	base64jwt := base64.StdEncoding.EncodeToString(jwt)

	token, err := authenticator.Parse(context.Background(), base64jwt)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(token, gc.NotNil)

	claims := token.PrivateClaims()
	c.Assert(token.Subject(), gc.Equals, "alice")
	c.Assert(token.Issuer(), gc.Equals, "test")
	c.Assert(token.Audience(), jc.DeepEquals, []string{"controller-1"})
	c.Assert(token.Expiration().After(token.IssuedAt()), gc.Equals, true)
	c.Assert(claims["access"], jc.DeepEquals, map[string]interface{}{"model-1": "read"})
}

// NewJWKSet returns a new key set and signing key.
func NewJWKSet(c *gc.C) (jwk.Set, jwk.Key) {
	jwkSet, pkeyPem := getJWKS(c)

	block, _ := pem.Decode(pkeyPem)

	pkeyDecoded, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	c.Assert(err, jc.ErrorIsNil)

	signingKey, err := jwk.FromRaw(pkeyDecoded)
	c.Assert(err, jc.ErrorIsNil)

	return jwkSet, signingKey
}

func getJWKS(c *gc.C) (jwk.Set, []byte) {
	keySet, err := rsa.GenerateKey(rand.Reader, 4096)
	c.Assert(err, jc.ErrorIsNil)

	privateKeyPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(keySet),
		},
	)

	kid, err := uuid.NewRandom()
	c.Assert(err, jc.ErrorIsNil)

	jwks, err := jwk.FromRaw(keySet.PublicKey)
	c.Assert(err, jc.ErrorIsNil)
	err = jwks.Set(jwk.KeyIDKey, kid.String())
	c.Assert(err, jc.ErrorIsNil)

	err = jwks.Set(jwk.KeyUsageKey, "sig")
	c.Assert(err, jc.ErrorIsNil)

	err = jwks.Set(jwk.AlgorithmKey, jwa.RS256)
	c.Assert(err, jc.ErrorIsNil)

	ks := jwk.NewSet()
	err = ks.AddKey(jwks)
	c.Assert(err, jc.ErrorIsNil)

	return ks, privateKeyPEM
}
