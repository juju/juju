// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/state"
)

// EntityAuthenticator performs authentication for juju entities.
type AgentAuthenticator struct {
	UserService UserService
}

// UserService is used to operate with Users from the database.
type UserService interface {
	GetUserByAuth(ctx context.Context, name string, password string) (coreuser.User, error)
}

var _ EntityAuthenticator = (*AgentAuthenticator)(nil)

type taggedAuthenticator interface {
	state.Entity
	state.Authenticator
}

type userEntity struct {
	tag names.UserTag
}

func (u *userEntity) Tag() names.Tag {
	return u.tag
}

// Authenticate authenticates the provided entity.
// It takes an entityfinder and the tag used to find the entity that requires authentication.
func (a *AgentAuthenticator) Authenticate(ctx context.Context, entityFinder EntityFinder, authParams AuthParams) (state.Entity, error) {
	// Check if the entity is a user, in another case, use the legacy method.
	switch authParams.AuthTag.Kind() {
	case names.UserTagKind:
		user, err := a.UserService.GetUserByAuth(ctx, authParams.AuthTag.Id(), authParams.Credentials)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !names.IsValidUser(user.Name) {
			return nil, errors.Trace(apiservererrors.ErrBadCreds)
		}
		return &userEntity{tag: names.NewUserTag(user.Name)}, nil
	default:
		return a.legacy(ctx, entityFinder, authParams)
	}
}

// Legacy is used to authenticate entities that are not moved to a Dqlite database. This function should be gone after
// all entities are moved to the Dqlite database.
func (*AgentAuthenticator) legacy(ctx context.Context, entityFinder EntityFinder, authParams AuthParams) (state.Entity, error) {
	entity, err := entityFinder.FindEntity(authParams.AuthTag)
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
