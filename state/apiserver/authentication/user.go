// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/state"
)

// UserIdentityProvider performs authentication for users.
type UserAuthenticator struct {
	AgentAuthenticator
}

var _ TagAuthenticator = (*UserAuthenticator)(nil)

// Authenticate authenticates the provided entity and returns an error on authentication failure.
func (u *UserAuthenticator) Authenticate(entity state.Entity, password, nonce string) error {
	if kind := entity.Tag().Kind(); kind != names.UserTagKind {
		return errors.Errorf("entity with tag '%s' cannot be authenticated as a user", kind)
	}
	return u.AgentAuthenticator.Authenticate(entity, password, nonce)
}
