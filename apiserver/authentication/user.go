// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
)

// UserIdentityProvider performs authentication for users.
type UserAuthenticator struct {
	AgentAuthenticator
}

var _ EntityAuthenticator = (*UserAuthenticator)(nil)

// Authenticate authenticates the provided entity and returns an error on authentication failure.
func (u *UserAuthenticator) Authenticate(entity state.Entity, password, nonce string) error {
	if _, ok := entity.(*state.User); ok {
		return u.AgentAuthenticator.Authenticate(entity, password, nonce)
	}

	return common.ErrBadRequest
}
