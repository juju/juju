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

	// MatchesPublicKeyInModelForUser reports whether the supplied fingerprint
	// belongs to a public key the named user is authorized to use in the model.
	MatchesPublicKeyInModelForUser(context.Context, string, string, string) (bool, error)

	// GetSSHServerPort returns the port the controller SSH jump server listens
	// on. If no port has been set, it returns an error satisfying
	// [github.com/juju/juju/core/errors.NotFound].
	GetSSHServerPort(context.Context) (int, error)

	// SetSSHServerPort sets the port the controller SSH jump server listens on.
	SetSSHServerPort(context.Context, int) error

	// NamespaceForWatchSSHServerPort returns the change-stream namespace used
	// to watch for changes to the controller SSH server port.
	NamespaceForWatchSSHServerPort() string
}
