// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/state"
)

// AgentAuthenticatorFactory is a factory for creating authenticators, which
// can create authenticators for a given state.
type AgentAuthenticatorFactory struct {
	legacyState *state.State
	logger      corelogger.Logger
}

// NewAgentAuthenticatorFactory returns a new agent authenticator factory, for
// a known state.
func NewAgentAuthenticatorFactory(legacyState *state.State, logger corelogger.Logger) AgentAuthenticatorFactory {
	return AgentAuthenticatorFactory{
		legacyState: legacyState,
		logger:      logger,
	}
}

// Authenticator returns an authenticator using the factory's state.
func (f AgentAuthenticatorFactory) Authenticator() EntityAuthenticator {
	return agentAuthenticator{
		state:  f.legacyState,
		logger: f.logger,
	}
}

// AuthenticatorForState returns an authenticator for the given state.
func (f AgentAuthenticatorFactory) AuthenticatorForState(st *state.State) EntityAuthenticator {
	return agentAuthenticator{
		state:  st,
		logger: f.logger,
	}
}

type agentAuthenticator struct {
	state  *state.State
	logger corelogger.Logger
}

type taggedAuthenticator interface {
	state.Entity
	state.Authenticator
}

// Authenticate authenticates the provided entity.
// It takes an entityfinder and the tag used to find the entity that requires authentication.
func (a agentAuthenticator) Authenticate(ctx context.Context, authParams AuthParams) (state.Entity, error) {
	switch authParams.AuthTag.Kind() {
	case names.UserTagKind:
		return nil, errors.Trace(apiservererrors.ErrBadRequest)
	default:
		return a.fallbackAuth(ctx, authParams)
	}
}

func (a *agentAuthenticator) fallbackAuth(ctx context.Context, authParams AuthParams) (state.Entity, error) {
	entity, err := a.state.FindEntity(authParams.AuthTag)
	if errors.Is(err, errors.NotFound) {
		logger.Debugf("cannot authenticate unknown entity: %v", authParams.AuthTag)
		return nil, errors.Trace(apiservererrors.ErrUnauthorized)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	authenticator, ok := entity.(taggedAuthenticator)
	if !ok {
		return nil, errors.Trace(apiservererrors.ErrBadRequest)
	}
	if !authenticator.PasswordValid(authParams.Credentials) {
		return nil, errors.Trace(apiservererrors.ErrUnauthorized)
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
