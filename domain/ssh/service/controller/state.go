// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	coressh "github.com/juju/juju/core/ssh"
	"github.com/juju/juju/core/user"
)

// State describes controller-scoped persistence for SSH host keys.
type State interface {
	// GetSSHServerHostKey returns the stored controller jump host key.
	GetSSHServerHostKey(context.Context) (string, error)

	// GetSSHServerHostPublicKey returns the marshalled public host key of the
	// controller SSH jump server. The public key is derived once at bootstrap
	// and stored alongside the private key, so this method never handles
	// private key material.
	GetSSHServerHostPublicKey(context.Context) ([]byte, error)

	// GetPublicKeysForUser returns all public keys registered for a user.
	GetPublicKeysForUser(context.Context, user.Name) ([]coressh.PublicKey, error)
}
