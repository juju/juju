// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"bytes"
	"context"

	"github.com/juju/errors"
	ssh "github.com/tailscale/gliderssh"

	"github.com/juju/juju/core/logger"
)

// authenticatedPublicKey holds the public key that was used to authenticate
// the user. It is absent if the user did not authenticate with a public key.
// It is used later, once the target model is known, to verify that the key is
// associated with the model being accessed.
type authenticatedPublicKey struct{}

// UserPublicKeyService retrieves the public keys registered for a user.
type UserPublicKeyService interface {
	PublicKeys(context.Context, string) ([]publicKeyWithComment, error)
}

// authenticator implements the Authenticator interface for the SSH server.
// It handles public key authentication by users. Password authentication
// is removed: machine reverse tunnels and JIMM relay sessions now
// authenticate at the HTTP layer on the API server's upgrade endpoints,
// so the jump server rejects all passwords and key-less users get a clean
// public key rejection instead of an "enter password:" prompt.
type authenticator struct {
	logger     logger.Logger
	publicKeys UserPublicKeyService
}

// PublicKeyAuthentication implements a public key authentication handler.
func (a authenticator) PublicKeyAuthentication(ctx ssh.Context, key ssh.PublicKey) (bool, error) {
	keys, err := a.publicKeys.PublicKeys(ctx, ctx.User())
	if err != nil {
		return false, errors.Annotatef(err, "getting SSH public keys for user %q", ctx.User())
	}

	for _, authorizedKey := range keys {
		if bytes.Equal(key.Marshal(), authorizedKey.Marshal()) {
			ctx.SetValue(authenticatedPublicKey{}, authorizedKey)
			return true, nil
		}
	}

	return false, nil
}
