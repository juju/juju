// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"github.com/juju/juju/state"
	"github.com/juju/juju/apiserver/common"
)

// FindEntityAuthenticator looks up the authenticator for the entity identified tag.
func FindEntityAuthenticator(entity state.Entity) (EntityAuthenticator, error) {
	switch entity.(type) {
	case *state.Machine, *state.Unit:
		return &AgentAuthenticator{}, nil
	case *state.User:
		return &UserAuthenticator{}, nil
	}

	return nil, common.ErrBadRequest
}
