package idprovider

import (
	"github.com/juju/errors"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/apiserver/common"
)

type AgentIdentityProvider struct {
}

var _ IdentityProvider = (*AgentIdentityProvider)(nil)

type taggedAuthenticator interface {
	state.Entity
	state.Authenticator
}

func NewAgentIdentityProvider() IdentityProvider {
	return &AgentIdentityProvider{}
}

func (*AgentIdentityProvider) Login(st *state.State, tag, password, nonce string) error {
	entity0, err := st.FindEntity(tag)
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
