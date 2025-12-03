// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jaas

import (
	"context"
	"sync"

	"github.com/juju/names/v6"
	"github.com/lestrrat-go/jwx/v2/jwt"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/trace"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/internal/errors"
)

// Authenticator provides an implementation of [auth.Authenticator] that is
// capable of authenticating users from a trusted JAAS controller.
//
// Authenticator is not safe for concurrent use.
type Authenticator struct {
	// token is the raw JWT token received from the context that has been
	// created by a trusted JAAS source.
	token string

	// userService is the domain user access service required for ensuring that
	// authenticated JAAS users are registered as external users within the
	// controller.
	userService UserService

	// verifier is a JAAS token verifier that confirms a given token was
	// created by a trusted JAAS controller.
	verifier TokenVerifier
}

// AuthResult represents the extracted token information from a JAAS jwt token
// after successful authentication.
//
// This AuthResult will ensure that the user from the token is registered in the
// current controller as an external user when [AuthResult.AuthenticatedActor]
// is called.
//
// AuthResult implements the [auth.AuthResult] interface. AuthResult is not safe
// for concurrent use.
type AuthResult struct {
	// jaasIdentifier is a unique identifier given by the controller to the JAAS
	// controller source that authenticated this [AuthResult].
	jaasIdentifier string

	// userUUIDGetter is a function that retrieves the controller user uuid for
	// the authenticated JAAS user. See [getJAASUserUUIDFunc]. This member is
	// expected to be nil until such time auth result information is required.
	userUUIDGetter func() (coreuser.UUID, error)

	// userDomain is the domain component extracted from the JAAS JWT token. If
	// no domain value was set by JAAS in the token then this value will be a
	// zero value string.
	userDomain string

	// userName is the user name component extracted from the JAAS JWT token.
	userName string

	// userService is the domain user access service required for ensuring that
	// authenticated JAAS users are registered as external users within the
	// controller.
	userService UserService
}

// TokenVerifier provides the verification service for validating a token
// recevied has been signed by a trusted JAAS source.
type TokenVerifier interface {
	// SourceID returns an ID value given to the origin source of the verifier.
	// Practical example of this would be the URL of the JAAS controller that
	// issued the token.
	//
	// No assumptions should be made about the value contained in the ID. It
	// MUST be considered opaque to the caller.
	SourceID() string

	// VerifyToken verifies the raw JAAS token returning the parsed [jwt.Token]
	// when the token is valid and trusted.
	VerifyToken(context.Context, string) (jwt.Token, error)
}

// UserService defines the required interface needed for ensuring external
// users in the controller.
type UserService interface {
	// EnsureExternalUser is responsible for making sure that the supplied
	// external user exists within the controller. If the display name differs
	// from any existing user for the supplied user name it will be updated to
	// the new value.
	EnsureExternalUser(
		context.Context,
		coreuser.Name,
		string,
	) (coreuser.UUID, error)
}

const (
	// jaasAuthenticatorType represents a well known string value used to
	// indicate the type of authenticator on offer by this package. This value
	// is meant for auditing so that a auditer can reasonably associate an
	// authentication with this package.
	jaasAuthenticatorType string = "jaas"

	// jaasUserDomain represents the domain applied to users that come from JAAS.
	// For the moment this value is hard coded but should be swapped out for a
	// more unique domain that uniquely represents the JAAS controller.
	jaasUserDomain string = "jaas"
)

// Authenticate checks to see that the recieved raw JAAS jwt token is valid and
// signed by a trusted JAAS source. The token is also checked to make sure that
// it hasn't expired.
//
// The following errors may be returned:
// - [auth.ErrStopAuthentication] when the received tokens expiry or issued at
// time are not valid. The token cannot be uses anymore and we should not
// continue performing anymore authentication attempts on it.
//
// Authenticate implements the [auth.Authenticator] interface.
func (a Authenticator) Authenticate(ctx context.Context) (
	auth.AuthResult, bool, error,
) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	verifiedToken, err := a.verifier.VerifyToken(ctx, a.token)
	switch {
	// We MUST never output the token that forms part of this context in error
	// messages. Tokens are secrets and MUST remain that way.
	case errors.Is(err, jwt.ErrTokenExpired()):
		return nil, false, errors.New(
			"recieved JAAS token has expired and cannot be used for authentication",
		).Add(auth.ErrStopAuthentication)
	case errors.Is(err, jwt.ErrInvalidIssuedAt()):
		return nil, false, errors.New(
			"recieved JAAS token has invalid issued atand cannot be used for authentication",
		).Add(auth.ErrStopAuthentication)
	case err != nil:
		return nil, false, errors.Errorf(
			"verifying recieved JAAS token: %w", err,
		)
	}

	// NOTE (tlm): It would be ideal if we didn't have the names package import
	// here. For the moment the contract that we have with JAAS is that a valid
	// names tag is supplied.
	tokenSubject := verifiedToken.Subject()
	userTag, err := names.ParseUserTag(tokenSubject)
	if err != nil {
		return nil, false, errors.Errorf(
			"parsing user tag from authenticated JAAS token: %w", err,
		)
	}

	userNameStr := userTag.Name()
	userNameDomainStr := userTag.Domain()

	return AuthResult{
		jaasIdentifier: a.verifier.SourceID(),
		userDomain:     userNameDomainStr,
		userName:       userNameStr,
		userService:    a.userService,
	}, true, nil
}

// AuthenticatedActor returns the authenticated user and their unique user uuid
// within the controller. Should the authenticated JAAS user not exist in the
// controller a new external user will be created.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the user name or domain of the authenticated
// JAAS user are not valid.
func (a AuthResult) AuthenticatedActor(ctx context.Context) (
	auth.AuthenticatedActorType, string, error,
) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	userUUID, err := a.getJAASUserUUID(ctx)
	if err != nil {
		return "", "", err
	}

	return auth.AuthenticatedEntityTypeUser, userUUID.String(), nil
}

// getJAASUserUUID returns the authenticated JAAS users uuid in the controller.
// The following errors may be returned:
// - [coreerrors.NotValid] when the user name or domain supplied are not valid.
func (a AuthResult) getJAASUserUUID(ctx context.Context) (coreuser.UUID, error) {
	if a.userUUIDGetter == nil {
		// If [AuthResult.userUUIDGetter] is nil we set it to a
		// [sync.OnceValues] function that will perform the ensure operation on
		// the database exactly once.
		//
		// Using [sync.OnceValues] is not to make this [AuthResult] safe for
		// concurrency. Concurrency is not a contract offered by this
		// [AuthResult].
		a.userUUIDGetter = sync.OnceValues(getJAASUserUUIDFunc(
			ctx, a.userName, a.userDomain, a.userService,
		))
	}

	return a.userUUIDGetter()
}

// getJAASUserUUIDFunc returns a func suitable for use in [sync.OnceValues] to
// ensure that the supplied JAAS user exists in the controller database
// returning the user uuid.
//
// This func exists in this form so that for many repetitive calls [AuthResult]
// does not create unnecessary database queries.
//
// Because of the intended use of this func we need to capture the context at
// the moment of the call and not creation.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the user name or domain supplied are not valid.
func getJAASUserUUIDFunc(
	ctx context.Context,
	userNameStr string,
	userDomain string,
	userService UserService,
) func() (coreuser.UUID, error) {
	return func() (coreuser.UUID, error) {
		ctx, span := trace.Start(ctx, trace.NameFromFunc())
		defer span.End()

		if userDomain == "" {
			// If the user has no domain set we apply the default JAAS domain.
			// NOTE (tlm): The Juju controller should be in control of this value
			// all the time and not JAAS. The best path would be to always
			// overwriting this value but it isn't clear what breakages this might
			// cause at the moment.
			userDomain = jaasUserDomain
		}

		userName, err := coreuser.ParseNameWithDomain(userNameStr, userDomain)
		if errors.Is(err, coreerrors.NotValid) {
			// The parsed user name from the JAAS token is not valid. As the
			// authentication has been performed this now becomes a processing
			// error.
			return "", errors.Errorf(
				"user name %q with domain %q from authenticated JAAS token is not valid",
				userName.Name(), userDomain,
			).Add(coreerrors.NotValid)
		} else if err != nil {
			return "", errors.Errorf(
				"parsing user name from authenticated JAAS token: %w", err,
			)
		}

		// Ensure that an external user exists in the controller to represents
		// the JAAS user.
		userUUID, err := userService.EnsureExternalUser(
			ctx, userName, userNameStr,
		)
		if err != nil {
			return "", errors.Errorf(
				"ensuring external JAAS user %q with domain %q exists in the controller: %w",
				userName.Name(), userDomain, err,
			)
		}

		return userUUID, nil
	}
}

// WithAuditContext returns a derived context with audit information set about
// the authentication result and how it was achieved. The required context audit
// keys for actor type, actor uuid, authenticator name and authenticator used
// will be set.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the user name or domain supplied are not valid.
func (a AuthResult) WithAuditContext(
	ctx context.Context,
) (context.Context, error) {
	userUUID, err := a.getJAASUserUUID(ctx)
	if err != nil {
		return ctx, errors.Errorf(
			"getting authenticated JAAS user uuid: %w", err,
		)
	}

	ctx = auth.WithAuditActorType(ctx, auth.AuthenticatedEntityTypeUser)
	ctx = auth.WithAuditActorUUID(ctx, userUUID.String())
	ctx = auth.WithAuditAuthenticatorUsed(ctx, jaasAuthenticatorType)
	ctx = auth.WithAuditAuthenticatorName(ctx, a.jaasIdentifier)
	return ctx, nil
}
