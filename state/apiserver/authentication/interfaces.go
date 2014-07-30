// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"github.com/juju/juju/state"
)

// EntityAuthenticator is the interface all entity authenticators need to implement
// to authenticate juju entities.
type EntityAuthenticator interface {
	// Authenticate authenticates the given entity
	Authenticate(entity state.Entity, password, nonce string) error
}
