// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"bytes"
	"context"

	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	"github.com/lestrrat-go/jwx/v2/jwt"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/logger"
	coressh "github.com/juju/juju/core/ssh"
)

type authenticatedViaPublicKey struct{}

type userJWT struct{}
type tunnelIDKey struct{}

const jimmUser = "jimm"

// JWTParser parses JIMM's encoded JWT password authentication payload.
type JWTParser interface {
	// Parse parses the provided JWT string and returns a jwt.Token if valid.
	Parse(context.Context, string) (jwt.Token, error)
}

// TunnelAuthenticator authenticates machine reverse-tunnel connections.
type TunnelAuthenticator interface {
	AuthenticateTunnel(username, password string) (string, error)
}

// UserPublicKeyService retrieves the public keys registered for a user.
type UserPublicKeyService interface {
	PublicKeys(context.Context, string) ([]gossh.PublicKey, error)
}

// authenticator implements the Authenticator interface for the SSH server.
// It handles:
// 1. Public key authentication by users
// 2. JWT password authentication for JIMM
// 3. Reverse-tunnel authentication for machine agents.
type authenticator struct {
	logger        logger.Logger
	jwtParser     JWTParser
	tunnelTracker TunnelAuthenticator
	publicKeys    UserPublicKeyService
}

// PublicKeyAuthentication implements a public key authentication handler.
func (a authenticator) PublicKeyAuthentication(ctx ssh.Context, key ssh.PublicKey) (bool, error) {
	keys, err := a.publicKeys.PublicKeys(ctx, ctx.User())
	if err != nil {
		return false, errors.Annotatef(err, "getting SSH public keys for user %q", ctx.User())
	}

	for _, authorizedKey := range keys {
		if bytes.Equal(key.Marshal(), authorizedKey.Marshal()) {
			ctx.SetValue(authenticatedViaPublicKey{}, true)
			return true, nil
		}
	}

	return false, nil
}

// PasswordAuthentication implements a password authentication handler.
// It supports two types of password authentication:
// 1. Decoding a JWT as the password for JIMM.
// 2. Reverse-tunnel authentication for machine agents.
func (a authenticator) PasswordAuthentication(ctx ssh.Context, password string) (bool, error) {
	ctx.SetValue(authenticatedViaPublicKey{}, false)

	switch ctx.User() {
	case jimmUser:
		token, err := a.jwtParser.Parse(ctx, password)
		if err != nil {
			return false, errors.Annotate(err, "parsing SSH JWT")
		}
		ctx.SetValue(userJWT{}, token)
		return true, nil
	case coressh.ReverseTunnelUser:
		tunnelID, err := a.tunnelTracker.AuthenticateTunnel(ctx.User(), password)
		if err != nil {
			return false, errors.Annotate(err, "authenticating reverse SSH tunnel")
		}
		ctx.SetValue(tunnelIDKey{}, tunnelID)
		return true, nil
	}
	return false, nil
}
