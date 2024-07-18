// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager

import (
	"context"
	"net/url"

	coressh "github.com/juju/juju/core/ssh"
	"github.com/juju/juju/core/user"
)

// KeyManagerService is the interface for the service this facade requires to
// perform crud operations on a users public ssh keys within a model.
type KeyManagerService interface {
	// AddPublicKeysForUser is responsible for adding one or more new public
	// keys for a user to the current model.
	AddPublicKeysForUser(context.Context, user.UUID, ...string) error

	// DeleteKeysForUser is responsible removing one ore more keys from a user
	// within this model. Keys are identified by either the comment,
	// public key data or the fingerprint.
	DeleteKeysForUser(context.Context, user.UUID, ...string) error

	// ImportPublicKeysForUser is responsible for importing keys against a Juju
	// user for the current model from a third party source identified in the
	// url.
	ImportPublicKeysForUser(context.Context, user.UUID, *url.URL) error

	// ListPublicKeysForUser is responsible returning all of the public keys
	// for a user within the current model.
	ListPublicKeysForUser(context.Context, user.UUID) ([]coressh.PublicKey, error)
}

// UserService is the interface for the service this facade requires to lookup
// users by their username in exchange for the internal information we know about
type UserService interface {
	// GetUserByName find thes the Juju identified by their username and returns
	// the internal user representation matching that username.
	GetUserByName(context.Context, user.Name) (user.User, error)
}
