// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/apiserver/common"
)

// AgentIdentityProvider performs authentication for machine and unit agents.
type AgentAuthenticator struct{}

var _ EntityAuthenticator = (*AgentAuthenticator)(nil)

type taggedAuthenticator interface {
	state.Entity
	state.Authenticator
}

// Authenticate authenticates the provided entity and returns an error on authentication failure.
func (*AgentAuthenticator) Authenticate(entity state.Entity, password, nonce string) error {
	authenticator, ok := entity.(taggedAuthenticator)
	if !ok {
		return common.ErrBadRequest
	}
	if !authenticator.PasswordValid(password) {
		return common.ErrBadCreds
	}

	// If this is a machine agent connecting, we need to check the
	// nonce matches, otherwise the wrong agent might be trying to
	// connect.
	if machine, ok := authenticator.(*state.Machine); ok {
		if !machine.CheckProvisioned(nonce) {
			return state.NotProvisionedError(machine.Id())
		}
	}

	return nil
}
