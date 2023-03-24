// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"time"

	"github.com/google/uuid"
	"github.com/juju/errors"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// JWTParams are the necessary params to issue a ready-to-go JWT.
type JWTParams struct {
	Controller string
	User       string
	Access     map[string]string
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

func generateJTI() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// NewJWT returns a parsed jwt.
func NewJWT(params JWTParams) (jwt.Token, error) {
	tok, set, err := EncodedJWT(params)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return jwt.Parse(
		tok,
		jwt.WithKeySet(set),
	)
}

// EncodedJWT returns jwt as bytes plus the key set used to generate it.
func EncodedJWT(params JWTParams) ([]byte, jwk.Set, error) {
	if jti, err := generateJTI(); err != nil {
		return nil, nil, errors.Trace(err)
	} else {
		jwkSet, pkeyPem, err := getJWKS()
		if err != nil {
			return nil, nil, errors.Trace(err)
		}

		pubKey, ok := jwkSet.Key(jwkSet.Len() - 1)
		if !ok {
			return nil, nil, errors.Errorf("no jwk found")
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

		err = signingKey.Set(jwk.AlgorithmKey, jwa.RS256)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		err = signingKey.Set(jwk.KeyIDKey, pubKey.KeyID())
		if err != nil {
			return nil, nil, errors.Trace(err)
		}

		token, err := jwt.NewBuilder().
			Audience([]string{params.Controller}).
			Subject(params.User).
			Issuer("test").
			JwtID(jti).
			Claim("access", params.Access).
			Expiration(time.Now().Add(time.Hour)).
			Build()
		if err != nil {
			return nil, nil, errors.Trace(err)
		}

		freshToken, err := jwt.Sign(
			token,
			jwt.WithKey(
				jwa.RS256,
				signingKey,
			),
		)
		return freshToken, jwkSet, errors.Trace(err)
	}
}
