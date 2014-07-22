// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"github.com/juju/juju/state"
)

// TagAuthenticator is the interface all tag authenticators need to implement
// to authenticate juju entities.
type TagAuthenticator interface {
	Authenticate(tag state.Entity, password, nonce string) error
}
