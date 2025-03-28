// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/juju/errors"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// tunnelAuthentication provides a way of creating
// and validating passwords for SSH connections
// from machines to the Juju controller.
// The password is a JWT with an expiry.
type tunnelAuthentication struct {
	sharedSecret []byte
	jwtAlg       jwa.KeyAlgorithm
	clock        Clock
}

func newTunnelAuthentication(clock Clock) (tunnelAuthentication, error) {
	// The shared secret is generated dynamically because
	// user's SSH connections to the controller are tied
	// to the life of the Tracker object.
	key := make([]byte, 64) // 64 bytes for HS512
	if _, err := rand.Read(key); err != nil {
		return tunnelAuthentication{}, errors.Annotate(err, "failed to generate shared secret")
	}
	return tunnelAuthentication{
		sharedSecret: key,
		jwtAlg:       jwa.HS512,
		clock:        clock,
	}, nil
}

func (tAuth *tunnelAuthentication) generatePassword(tunnelID string, now, deadline time.Time) (string, error) {
	token, err := jwt.NewBuilder().
		Issuer(tokenIssuer).
		Subject(tokenSubject).
		IssuedAt(now).
		Expiration(deadline).
		Claim(tunnelIDClaimKey, tunnelID).
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

	tunnelID, ok := token.PrivateClaims()[tunnelIDClaimKey].(string)
	if !ok {
		return "", errors.New("invalid token")
	}
	return tunnelID, nil
}
