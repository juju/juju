// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/state"
)

// AuthInfoPermissions defined a type for a helper func that can answer
// questions an entity and what access they have on to a specific subject tag.
//
// It is up to the creator of the AuthInfo struct to provide a suitable
// implementation of this func. It's is for legacy reasons we allow querying of
// other entities besides the one focused on in the AuthInfo struct.
//
// TODO: tlm look at ways to gradually remove the Entity portion of this function
// so the question can only be asked of the one focuses on in the AuthInfo struct.
type AuthInfoPermissions func(Entity, names.Tag) (permission.Access, error)

// AuthInfo is returned by Authenticator and RequestAuthInfo.
type AuthInfo struct {
	// Delegator is the interface back to the authenticating mechanism for
	// helping with permission questions about the authed entity.
	Delegator PermissionDelegator

	// Entity is the user/machine/unit/etc that has authenticated.
	Entity Entity

	// PermissionsFn is a function that can return the permissions associated
	// with  the current AuthInfo. PermissionsFn should not be considered
	// concurrency safe.
	// PermissionsFn AuthInfoPermissions

	// ModelTag is the tag of the model for which access
	// may be required. Not all auth operations will use it,
	// eg checking for controller admin.
	// The model UUID for the tag comes off the login request.
	ModelTag names.ModelTag

	// Controller reports whether or not the authenticated
	// entity is a controller agent.
	Controller bool
}

// AuthParams holds the info used to authenticate a login request.
type AuthParams struct {
	// These are used for user or agent auth.
	AuthTag     names.Tag
	Credentials string

	// Token is used for rebac based auth.
	Token string

	// None is used for agent auth.
	Nonce string

	// These are used for macaroon auth.
	Macaroons     []macaroon.Slice
	BakeryVersion bakery.Version
}

// PermissionDelegator is an interface that represents a window back into the
// original authentication method that generated an AuthInfo struct. Specifically
// it allows users of AuthInfo to ask specific details about an entity's
// permissions that needs response aligned with the way in which they were
// authenticated.
type PermissionDelegator interface {
	// SubjectPermissions returns the permission the entity has for the
	// specified subject.
	SubjectPermissions(entity Entity, subject names.Tag) (permission.Access, error)

	// PermissionError is a helper implemented by the Authenticator for
	// returning the appropriate error when an authenticated entity is missing
	// permission for subject.
	PermissionError(subject names.Tag, permission permission.Access) error
}

// EntityAuthenticator is the interface all entity authenticators need to
// implement to authenticate juju entities.
type EntityAuthenticator interface {
	// Authenticate authenticates the given entity.
	Authenticate(ctx context.Context, entityFinder EntityFinder, authParams AuthParams) (state.Entity, error)
}

// Authorizer is a function type for authorizing a request.
//
// If this returns an error, the handler should return StatusForbidden.
type Authorizer interface {
	Authorize(AuthInfo) error
}

// Entity represents a user, machine, or unit that might be
// authenticated.
type Entity interface {
	Tag() names.Tag
}

// EntityFinder finds the entity described by the tag.
type EntityFinder interface {
	FindEntity(tag names.Tag) (state.Entity, error)
}

// HTTPAuthenticator provides an interface for authenticating a raw http request
// from a client.
type HTTPAuthenticator interface {
	// Authenticate authenticates the given request, returning the
	// auth info.
	//
	// If the request does not contain any authentication details,
	// then an error satisfying errors.Is(err, errors.NotFound) will be
	// returned.
	// If this returns an error that is not composable as HTTPWritableError then
	// the handler should return StatusUnauthorized.
	Authenticate(*http.Request) (AuthInfo, error)
}

// LoginAuthenticator provides an interface for authenticating RPC login
// requests from a client.
type LoginAuthenticator interface {
	// AuthenticateLoginRequest authenticates a LoginRequest.
	AuthenticateLoginRequest(
		ctx context.Context,
		serverHost string,
		modelUUID string,
		authParams AuthParams,
	) (AuthInfo, error)
}

// RequestAuthenticator is an interface the combines both the
// HTTPAuthenticator and LoginAuthenticator into a single interface as this
// functionality is most likely to be implemented together.
type RequestAuthenticator interface {
	HTTPAuthenticator
	LoginAuthenticator
}

// SubjectPermissions is a convenience wrapper around the AuthInfo permissions
// delegator. errors.NotImplemented is returned if the permission delegator
// on this AuthInfo is nil.
func (a *AuthInfo) SubjectPermissions(subject names.Tag) (permission.Access, error) {
	if a.Delegator == nil {
		return permission.NoAccess, fmt.Errorf("permissions delegator %w", errors.NotImplemented)
	}

	return a.Delegator.SubjectPermissions(a.Entity, subject)
}
