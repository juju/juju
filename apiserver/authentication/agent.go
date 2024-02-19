// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/state"
)

// Logger is the minimal logging interface required by the authenticator.
type Logger interface {
	Debugf(string, ...interface{})
}

// EntityAuthenticator performs authentication for juju entities.
type AgentAuthenticator struct {
	userService UserService
	legacyState *state.State
	logger      Logger
}

// NewAgentAuthenticator returns a new authenticator.
func NewAgentAuthenticator(userService UserService, legacyState *state.State, logger Logger) *AgentAuthenticator {
	return &AgentAuthenticator{
		userService: userService,
		legacyState: legacyState,
		logger:      logger,
	}
}

var _ EntityAuthenticator = (*AgentAuthenticator)(nil)

type taggedAuthenticator interface {
	state.Entity
	state.Authenticator
}

// Authenticate authenticates the provided entity.
// It takes an entityfinder and the tag used to find the entity that requires authentication.
func (a *AgentAuthenticator) Authenticate(ctx context.Context, authParams AuthParams) (state.Entity, error) {
	switch authParams.AuthTag.Kind() {
	case names.UserTagKind:
		return nil, errors.Trace(apiservererrors.ErrBadRequest)
	default:
		return a.fallbackAuth(ctx, authParams)
	}
}

func (a *AgentAuthenticator) fallbackAuth(ctx context.Context, authParams AuthParams) (state.Entity, error) {
	entity, err := a.legacyState.FindEntity(authParams.AuthTag)
	if errors.Is(err, errors.NotFound) {
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
