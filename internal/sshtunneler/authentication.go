// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	"encoding/base64"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

type tunnelAuthentication struct {
	sharedSecret []byte
	jwtAlg       jwa.KeyAlgorithm
	clock        clock.Clock
}

func (tAuth *tunnelAuthentication) generatePassword(tunnelID string, expiry time.Time) (string, error) {
	token, err := jwt.NewBuilder().
		Issuer(tokenIssuer).
		Subject(tokenSubject).
		IssuedAt(tAuth.clock.Now()).
		Expiration(expiry).
		Claim(tunnelIDClaim, tunnelID).
		Build()
	if err != nil {
		return "", errors.Annotate(err, "failed to build token")
	}

	signedToken, err := jwt.Sign(token, jwt.WithKey(tAuth.jwtAlg, tAuth.sharedSecret))
	if err != nil {
		return "", errors.Annotate(err, "failed to sign token")
	}

	return base64.StdEncoding.EncodeToString(signedToken), nil
}

func (tAuth *tunnelAuthentication) validatePassword(password string) (string, error) {
	decodedToken, err := base64.StdEncoding.DecodeString(password)
	if err != nil {
		return "", errors.Annotate(err, "failed to decode token")
	}

	token, err := jwt.Parse(decodedToken,
		jwt.WithKey(tAuth.jwtAlg, tAuth.sharedSecret),
		jwt.WithClock(tAuth.clock),
	)
	if err != nil {
		return "", errors.Annotate(err, "failed to parse token")
	}

	tunnelID, ok := token.PrivateClaims()[tunnelIDClaim].(string)
	if !ok {
		return "", errors.New("invalid token")
	}
	return tunnelID, nil
}
