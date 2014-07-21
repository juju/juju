// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package identityprovider

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/state"
)

// UserIdentityProvider performs authentication for users.
type UserIdentityProvider struct {
	AgentIdentityProvider
}

var _ IdentityProvider = (*UserIdentityProvider)(nil)

// Login authenticates the provided entity and returns an error on authentication failure.
func (u *UserIdentityProvider) Login(st *state.State, tag names.Tag, password, nonce string) error {
	if kind := tag.Kind(); kind != names.UserTagKind {
		return errors.Errorf("%s tag cannot be authenticated with a user identity provider", kind)
	}
	return u.AgentIdentityProvider.Login(st, tag, password, nonce)
}
