// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/apiserver/common"
)

// AgentIdentityProvider performs authentication for machine and unit agents.
type AgentAuthenticator struct {
	state *state.State
}

var _ TagAuthenticator = (*AgentAuthenticator)(nil)

type taggedAuthenticator interface {
	state.Entity
	state.Authenticator
}

// NewAgentAuthenticator returns an AgentAuthenticator initialized with a connection to state.
func NewAgentAuthenticator(st *state.State) *AgentAuthenticator {
	return &AgentAuthenticator{
		state: st,
	}
}

// Authenticate authenticates the provided entity and returns an error on authentication failure.
func (a *AgentAuthenticator) Authenticate(entity state.Entity, password, nonce string) error {
	// We return the same error when an entity
	// does not exist as for a bad password, so that
	// we don't allow unauthenticated users to find information
	// about existing entities.
	entityA, ok := entity.(taggedAuthenticator)
	if !ok {
		return common.ErrBadCreds
	}
	if !entityA.PasswordValid(password) {
		return common.ErrBadCreds
	}

	// If this is a machine agent connecting, we need to check the
	// nonce matches, otherwise the wrong agent might be trying to
	// connect.
	if machine, ok := entityA.(*state.Machine); ok {
		if !machine.CheckProvisioned(nonce) {
			return state.NotProvisionedError(machine.Id())
		}
	}

	return nil
}
