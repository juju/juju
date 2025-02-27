// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jwtparser_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"

	"github.com/google/uuid"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/jwtparser"
)

type jwtParserSuite struct {
	url        string
	keySet     jwk.Set
	signingKey jwk.Key
	srv        *httptest.Server
}

var _ = gc.Suite(&jwtParserSuite{})

func (s *jwtParserSuite) SetUpTest(c *gc.C) {
	keySet, signingKey, err := NewJWKSet()
	c.Assert(err, jc.ErrorIsNil)
	s.keySet = keySet
	s.signingKey = signingKey

	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RequestURI != "/.well-known/jwks.json" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		hdrs := w.Header()
		hdrs.Set(`Content-Type`, `application/json`)
		pub, _ := s.keySet.Key(0)
		_ = json.NewEncoder(w).Encode(pub)
	}))

	s.url = s.srv.URL + "/.well-known/jwks.json"
}

func (s *jwtParserSuite) TearDownTest(_ *gc.C) {
	s.srv.Close()
}

func (s *jwtParserSuite) TestCacheRegistration(c *gc.C) {
	authenticator := jwtparser.NewParserWithHTTPClient(jwtparser.DefaultHTTPClient())
	err := authenticator.RegisterJWKSCache(context.Background(), s.url)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *jwtParserSuite) TestCacheRegistrationFailureWithBadURL(c *gc.C) {
	authenticator := jwtparser.NewParserWithHTTPClient(jwtparser.DefaultHTTPClient())
	err := authenticator.RegisterJWKSCache(context.Background(), "noexisturl")
	// We want to make sure that we get an error for a bad url.
	c.Assert(err, gc.NotNil)
}

func (s *jwtParserSuite) TestParseJWT(c *gc.C) {
	authenticator := jwtparser.NewParserWithHTTPClient(jwtparser.DefaultHTTPClient())
	err := authenticator.RegisterJWKSCache(context.Background(), s.url)
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
func NewJWKSet() (jwk.Set, jwk.Key, error) {
	jwkSet, pkeyPem, err := getJWKS()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	block, _ := pem.Decode(pkeyPem)

	pkeyDecoded, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	signingKey, err := jwk.FromRaw(pkeyDecoded)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return jwkSet, signingKey, nil
}

func getJWKS() (jwk.Set, []byte, error) {
	keySet, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	privateKeyPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(keySet),
		},
	)

	kid, err := uuid.NewRandom()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	jwks, err := jwk.FromRaw(keySet.PublicKey)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	err = jwks.Set(jwk.KeyIDKey, kid.String())
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	err = jwks.Set(jwk.KeyUsageKey, "sig")
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	err = jwks.Set(jwk.AlgorithmKey, jwa.RS256)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	ks := jwk.NewSet()
	err = ks.AddKey(jwks)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	return ks, privateKeyPEM, nil
}
