// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// AgentIdentityProvider performs authentication for machine and unit agents.
type AgentAuthenticator struct{}

var _ EntityAuthenticator = (*AgentAuthenticator)(nil)

type taggedAuthenticator interface {
	state.Entity
	state.Authenticator
}

// Authenticate authenticates the provided entity.
// It takes an entityfinder and the tag used to find the entity that requires authentication.
func (*AgentAuthenticator) Authenticate(entityFinder EntityFinder, tag names.Tag, req params.LoginRequest) (state.Entity, error) {
	entity, err := entityFinder.FindEntity(tag)
	if errors.IsNotFound(err) {
		return nil, errors.Trace(common.ErrBadCreds)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	authenticator, ok := entity.(taggedAuthenticator)
	if !ok {
		return nil, errors.Trace(common.ErrBadRequest)
	}
	if !authenticator.PasswordValid(req.Credentials) {
		return nil, errors.Trace(common.ErrBadCreds)
	}

	// If this is a machine agent connecting, we need to check the
	// nonce matches, otherwise the wrong agent might be trying to
	// connect.
	if machine, ok := authenticator.(*state.Machine); ok {
		if !machine.CheckProvisioned(req.Nonce) {
			return nil, errors.NotProvisionedf("machine %v", machine.Id())
		}
	}

	return entity, nil
}
