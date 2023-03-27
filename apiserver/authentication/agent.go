// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"context"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/state"
)

// EntityAuthenticator performs authentication for juju entities.
type EntityAuthenticator struct{}

var _ Authenticator = (*EntityAuthenticator)(nil)

type taggedAuthenticator interface {
	state.Entity
	state.Authenticator
}

// Authenticate authenticates the provided entity.
// It takes an entityfinder and the tag used to find the entity that requires authentication.
func (*EntityAuthenticator) Authenticate(ctx context.Context, entityFinder EntityFinder, authParams AuthParams) (state.Entity, error) {
	entity, err := entityFinder.FindEntity(authParams.AuthTag)
	if errors.IsNotFound(err) {
		logger.Debugf("cannot authenticate unknown entity: %v", authParams.AuthTag)
		return nil, errors.Trace(apiservererrors.ErrBadCreds)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	authenticator, ok := entity.(taggedAuthenticator)
	if !ok {
		return nil, errors.Trace(apiservererrors.ErrBadRequest)
	}
	if !authenticator.PasswordValid(authParams.Credentials) {
		return nil, errors.Trace(apiservererrors.ErrBadCreds)
	}

	// If this is a machine agent connecting, we need to check the
	// nonce matches, otherwise the wrong agent might be trying to
	// connect.
	//
	// NOTE(axw) with the current implementation of Login, it is
	// important that we check the password before checking the
	// nonce, or an unprovisioned machine in a hosted model will
	// prevent a controller machine from logging into the hosted
	// model.
	if machine, ok := authenticator.(*state.Machine); ok {
		if !machine.CheckProvisioned(authParams.Nonce) {
			return nil, errors.NotProvisionedf("machine %v", machine.Id())
		}
	}

	return entity, nil
}
