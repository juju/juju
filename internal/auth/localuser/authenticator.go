// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package localuser

import (
	"context"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/trace"
	coreuser "github.com/juju/juju/core/user"
	domainaccesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/internal/errors"
)

// UserService represents the required interface needed from the domain access
// service.
type UserService interface {
	// GetUserByAuth will find and return the user identified by the supplied
	// user name confirming that the users password also matches. Only users
	// that are active within the current controller will be considered. The
	// user MUST also be a local user in the controller and not external.
	//
	// The following errors may be returned:
	// - [accesserrors.UserNotFound] when no user exists matching the supplied
	// user name or the user is considered an external user in the controller.
	// - [accesserrors.UserUnauthorized] when the supplied password does not
	// match the controllers stored password for the user.
	// - [accesserrors.UserNameNotValid] when the supplied user name is
	// not considered valid.
	// - [auth.ErrPasswordDestroyed] when the supplied password has already been
	// accessed and cannot be used again.
	// - [auth.ErrPasswordNotValid] when the supplied password is not considered
	// valid.
	GetUserByAuth(context.Context, coreuser.Name, auth.Password) (coreuser.User, error)
}

// AuthResult is an implementation of [auth.AuthResult] representing an
// authenticated user uuid within the local controllers records.
type AuthResult coreuser.UUID

// Authenticator provides an implementation of [auth.Authenticator] that is
// capable of authenticating users via username and password within the scope of
// the model.
//
// Users are only ever authenticated against the controllers user records and
// considered in context with the model being used. Model adds context for the
// authentication but does not act as a means of authorisation.
type Authenticator struct {
	// userService is the domain service used for both authenticating a user
	// and establishing their details within the controller.
	userService UserService

	// modelUUID is the uuid of the model being accessed as part of the
	// authentication.
	modelUUID coremodel.UUID

	// password is the supplied password for the user to have authenticated
	// along with [Authenticator.userName].
	password auth.Password

	// userName is the supplied username for the user to have authenticated
	// along with [Authenticator.password].
	userName coreuser.Name
}

// Authenticate checks a user name and password against the controller's user
// database. If the user is not found or no username and password match exist
// then authentication fails.
//
// Should the user to be authenticated have a domain component set for their
// username the authenticator will stop and return an error. This authenticator
// will only considered non external users.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when a zero value username has been supplied to the
// authenticator.
//
// Authenticate implements the [auth.Authenticator] interface.
func (a Authenticator) Authenticate(ctx context.Context) (
	auth.AuthResult, bool, error,
) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if a.userName.IsZero() {
		return nil, false, errors.Errorf(
			"supplied username is empty and cannot be used for authentication",
		).Add(coreerrors.NotValid)
	}

	// Check if the username has a domain component.
	if a.userName.Domain() != "" {
		return nil, false, errors.New(
			"user name has a domain component and cannotbe authenticated against the local controller",
		)
	}

	// Copy the password to a new temporary. This is done so that this func can
	// maintain its contract of being idempotent.
	pCopy := a.password
	user, err := a.userService.GetUserByAuth(ctx, a.userName, pCopy)

	switch {
	case errors.Is(err, domainaccesserrors.UserUnauthorized):
		// Username and password don't match. User cannot be authenticated.
		return nil, false, nil
	case errors.Is(err, domainaccesserrors.UserNotFound):
		// User not found in the controller. User cannot be authenticated.
		return nil, false, nil
	case errors.Is(err, domainaccesserrors.UserNameNotValid):
		// We don't respond to invalid usernames because this authenticator does
		// not have enough context about authentication. This might be a valid
		// user name for another authenticator. All we know is that they are not
		// authenticated at this point.
		return nil, false, nil
	case err != nil:
		return nil, false, errors.Errorf(
			"authenticating user %q against local controller records: %w", err,
		)
	}

	// User exists and their password matches.
	return AuthResult(user.UUID), true, nil
}

// AuthenticatedActor returns the user uuid of the authenticated local user
// with in the controller.
//
// Implements the [auth.AuthResult] interface.
func (a AuthResult) AuthenticatedActor(context.Context) (
	auth.AuthenticatedActorType, string, error,
) {
	return auth.AuthenticatedEntityTypeUser, coreuser.UUID(a).String(), nil
}

// NewAuthenticator creates a new [Authenticator] using the supplied
// authentication context values.
func NewAuthenticator(
	accessService UserService,
	modelUUID coremodel.UUID,
	password auth.Password,
	userName coreuser.Name,
) Authenticator {
	return Authenticator{
		userService: accessService,
		modelUUID:   modelUUID,
		password:    password,
		userName:    userName,
	}
}
