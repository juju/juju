// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package identityprovider

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/apiserver/common"
)

// AgentIdentityProvider performs authentication for machine and unit agents.
type AgentIdentityProvider struct{}

var _ IdentityProvider = (*AgentIdentityProvider)(nil)

type taggedAuthenticator interface {
	state.Entity
	state.Authenticator
}

// Login authenticates the provided entity and returns an error on authentication failure.
func (*AgentIdentityProvider) Login(st *state.State, tag names.Tag, password, nonce string) error {
	entity0, err := st.FindEntity(tag.String())
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	// We return the same error when an entity
	// does not exist as for a bad password, so that
	// we don't allow unauthenticated users to find information
	// about existing entities.
	entity, ok := entity0.(taggedAuthenticator)
	if !ok {
		return common.ErrBadCreds
	}
	if err != nil || !entity.PasswordValid(password) {
		return common.ErrBadCreds
	}

	// If this is a machine agent connecting, we need to check the
	// nonce matches, otherwise the wrong agent might be trying to
	// connect.
	if machine, ok := entity.(*state.Machine); ok {
		if !machine.CheckProvisioned(nonce) {
			return state.NotProvisionedError(machine.Id())
		}
	}

	return nil
}
