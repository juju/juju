// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/identchecker"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"gopkg.in/macaroon.v2"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	corelogger "github.com/juju/juju/core/logger"
	coreuser "github.com/juju/juju/core/user"
	usererrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/internal/auth"
	internallogger "github.com/juju/juju/internal/logger"
	internalmacaroon "github.com/juju/juju/internal/macaroon"
	"github.com/juju/juju/state"
)

const (
	// ErrInvalidLoginMacaroon is returned when a macaroon is not valid for
	// a local login request.
	ErrInvalidLoginMacaroon = errors.ConstError("invalid login macaroon")
)

var logger = internallogger.GetLogger("juju.apiserver.authentication")

// UserService is the interface that wraps the methods required to
// authenticate a user.
type UserService interface {
	// GetUserByAuth returns the user with the given name and password.
	GetUserByAuth(ctx context.Context, name coreuser.Name, password auth.Password) (coreuser.User, error)
	// GetUserByName returns the user with the given name.
	GetUserByName(ctx context.Context, name coreuser.Name) (coreuser.User, error)
}

// Bakery defines the subset of bakery.Bakery that we require for authentication.
type Bakery interface {
	MacaroonMinter
	MacaroonChecker
}

// MacaroonChecker exposes the methods needed from bakery.Checker.
type MacaroonChecker interface {
	Auth(ctx context.Context, mss ...macaroon.Slice) *bakery.AuthChecker
}

// MacaroonMinter exposes the methods needed from bakery.Oven.
type MacaroonMinter interface {
	NewMacaroon(ctx context.Context, version bakery.Version, caveats []checkers.Caveat, ops ...bakery.Op) (*bakery.Macaroon, error)
}

// ExpirableStorageBakery extends Bakery
// with the ExpireStorageAfter method so that root keys are
// removed from storage at that time.
type ExpirableStorageBakery interface {
	Bakery

	// ExpireStorageAfter returns a new ExpirableStorageBakery with
	// a store that will expire items added to it at the specified time.
	ExpireStorageAfter(time.Duration) (ExpirableStorageBakery, error)
}

// LocalUserAuthenticator performs authentication for local users. If a password
type LocalUserAuthenticator struct {
	UserService UserService
	// Bakery holds the bakery that is used to mint and verify macaroons.
	Bakery ExpirableStorageBakery

	// Clock is used to calculate the expiry time for macaroons.
	Clock clock.Clock

	// LocalUserIdentityLocation holds the URL of the trusted third party
	// that is used to address the is-authenticated-user third party caveat
	// to for local users. This always points at the same controller
	// agent that is servicing the authorisation request.
	LocalUserIdentityLocation string
}

const (
	usernameKey = "username"

	// LocalLoginInteractionTimeout is how long a user has to complete
	// an interactive login before it is expired.
	LocalLoginInteractionTimeout = 2 * time.Minute

	// TODO(axw) make this configurable via model config.
	localLoginExpiryTime = 24 * time.Hour

	// TODO(axw) check with cmars about this time limit. Seems a bit
	// too low. Are we prompting the user every hour, or just refreshing
	// the token every hour until the external IdM requires prompting
	// the user?
	externalLoginExpiryTime = 1 * time.Hour
)

var _ EntityAuthenticator = (*LocalUserAuthenticator)(nil)

// Authenticate authenticates the entity with the specified tag, and returns an
// error on authentication failure.
//
// If and only if no password is supplied, then Authenticate will check for any
// valid macaroons. Otherwise, password authentication will be performed.
func (u *LocalUserAuthenticator) Authenticate(
	ctx context.Context, authParams AuthParams,
) (state.Entity, bool, error) {
	// We know this is a user tag and can be nothing but. With those assumptions
	// made, we don't need a full AgentAuthenticator.
	userTag, ok := authParams.AuthTag.(names.UserTag)
	if !ok {
		return nil, false, errors.Errorf("invalid request")
	}
	if !userTag.IsLocal() {
		return nil, false, errors.Errorf("invalid request - expected local user")
	}

	// Empty credentials, will attempt to authenticate with macaroons.
	if authParams.Credentials == "" {
		return u.authenticateMacaroons(ctx, userTag, authParams)
	}

	// We believe we've got a password, so we'll try to authenticate with it.
	// This will check the user service for the user, ensuring that the user
	// isn't disabled or deleted.
	user, err := u.UserService.GetUserByAuth(ctx, coreuser.NameFromTag(userTag), auth.NewPassword(authParams.Credentials))
	if errors.Is(err, usererrors.UserNotFound) || errors.Is(err, usererrors.UserUnauthorized) {
		logger.Debugf(ctx, "user %s not found", userTag.String())
		return nil, false, errors.Trace(apiservererrors.ErrUnauthorized)
	} else if err != nil {
		return nil, false, errors.Trace(err)
	} else if user.Disabled {
		return nil, false, errors.Trace(apiservererrors.ErrUnauthorized)
	}

	// StateEntity requires the user to be returned as a state.Entity.
	return TaggedUser(user, userTag), false, nil
}

func (u *LocalUserAuthenticator) authenticateMacaroons(ctx context.Context, userTag names.UserTag, authParams AuthParams) (state.Entity, bool, error) {
	// Check for a valid request macaroon.
	if logger.IsLevelEnabled(corelogger.TRACE) {
		mac, _ := json.Marshal(authParams.Macaroons)
		logger.Tracef(ctx, "authentication macaroons for %s: %s", userTag, mac)
	}

	// Attempt to authenticate the user using the macaroons provided.
	a := u.Bakery.Auth(ctx, authParams.Macaroons...)
	macaroonAuthInfo, err := a.Allow(ctx, identchecker.LoginOp)
	if err != nil {
		return nil, false, u.handleDischargeRequiredError(ctx, userTag, authParams.BakeryVersion, err)
	} else if macaroonAuthInfo != nil && len(macaroonAuthInfo.Conditions()) == 0 {
		return nil, false, u.handleDischargeRequiredError(ctx, userTag, authParams.BakeryVersion, ErrInvalidLoginMacaroon)
	}

	logger.Tracef(ctx, "authenticated conditions: %v", macaroonAuthInfo.Conditions())

	// Locate the user name from the macaroon.
	index := macaroonAuthInfo.OpIndexes[identchecker.LoginOp]
	if index < 0 || index > len(macaroonAuthInfo.Macaroons) {
		return nil, false, errors.Trace(apiservererrors.ErrUnauthorized)
	}
	loginMac := macaroonAuthInfo.Macaroons[index]
	declared := checkers.InferDeclared(internalmacaroon.MacaroonNamespace, loginMac)
	username := declared[usernameKey]

	// If the userTag id is not the same as the username, then the user is not
	// authenticated.
	if userTag.Id() != username {
		return nil, false, apiservererrors.ErrPerm
	}

	// We've got a valid macaroon, so we can return the user.
	user, err := u.UserService.GetUserByName(ctx, coreuser.NameFromTag(userTag))
	if errors.Is(err, usererrors.UserNotFound) || errors.Is(err, usererrors.UserUnauthorized) {
		logger.Debugf(ctx, "user %s not found", userTag.String())
		return nil, false, errors.Trace(apiservererrors.ErrUnauthorized)
	} else if err != nil {
		return nil, false, errors.Trace(err)
	} else if user.Disabled {
		return nil, false, errors.Trace(apiservererrors.ErrUnauthorized)
	}

	// StateEntity requires the user to be returned as a state.Entity.
	return TaggedUser(user, userTag), false, nil
}

func (u *LocalUserAuthenticator) handleDischargeRequiredError(ctx context.Context, userTag names.UserTag, bakeryVersion bakery.Version, cause error) error {
	logger.Debugf(ctx, "local-login macaroon authentication failed: %v", cause)

	// The root keys for these macaroons are stored in MongoDB.
	// Expire the documents after a set amount of time.
	expiryTime := u.Clock.Now().Add(localLoginExpiryTime)
	bakery, err := u.Bakery.ExpireStorageAfter(localLoginExpiryTime)
	if err != nil {
		return errors.Trace(err)
	}

	// Make a new macaroon with a caveat for login operation.
	macaroon, err := bakery.NewMacaroon(
		ctx,
		bakeryVersion,
		[]checkers.Caveat{
			checkers.TimeBeforeCaveat(expiryTime),
			checkers.NeedDeclaredCaveat(
				checkers.Caveat{
					Location:  u.LocalUserIdentityLocation,
					Condition: "is-authenticated-user " + userTag.Id(),
				},
				usernameKey,
			),
		},
		identchecker.LoginOp,
	)
	if err != nil {
		return errors.Annotate(err, "cannot create macaroon")
	}

	return &apiservererrors.DischargeRequiredError{
		Cause:          cause,
		LegacyMacaroon: macaroon.M(),
		Macaroon:       macaroon,
	}
}

// ExternalMacaroonAuthenticator performs authentication for external users using
// macaroons. If the authentication fails because provided macaroons are invalid,
// and macaroon authentiction is enabled, it will return a *apiservererrors.DischargeRequiredError
// holding a macaroon to be discharged.
type ExternalMacaroonAuthenticator struct {
	// Bakery holds the bakery that is
	// used to verify macaroon authorization.
	Bakery *identchecker.Bakery

	// IdentityLocation holds the URL of the trusted third party
	// that is used to address the is-authenticated-user
	// third party caveat to.
	IdentityLocation string

	// Clock is used to set macaroon expiry time.
	Clock clock.Clock
}

var _ EntityAuthenticator = (*ExternalMacaroonAuthenticator)(nil)

// Authenticate authenticates the provided entity. If there is no macaroon provided, it will
// return a *DischargeRequiredError containing a macaroon that can be used to grant access.
func (m *ExternalMacaroonAuthenticator) Authenticate(ctx context.Context, authParams AuthParams) (state.Entity, bool, error) {
	authChecker := m.Bakery.Checker.Auth(authParams.Macaroons...)
	ai, identErr := authChecker.Allow(ctx, identchecker.LoginOp)
	if de, ok := errors.Cause(identErr).(*bakery.DischargeRequiredError); ok {
		if dcMac, err := m.Bakery.Oven.NewMacaroon(ctx, authParams.BakeryVersion, de.Caveats, de.Ops...); err != nil {
			return nil, false, errors.Annotatef(err, "cannot create macaroon")
		} else {
			return nil, false, &apiservererrors.DischargeRequiredError{
				Cause:    identErr,
				Macaroon: dcMac,
			}
		}
	}
	if identErr != nil {
		return nil, false, errors.Trace(identErr)
	}
	username := ai.Identity.Id()
	logger.Debugf(ctx, "authenticated external user %q", username)
	var tag names.UserTag
	if names.IsValidUserName(username) {
		// The name is a local name without an explicit @local suffix.
		// In this case, for compatibility with 3rd parties that don't
		// care to add their own domain, we add an @external domain
		// to ensure there is no confusion between local and external
		// users.
		// TODO(rog) remove this logic when deployed dischargers
		// always add an @ domain.
		tag = names.NewLocalUserTag(username).WithDomain("external")
	} else {
		// We have a name with an explicit domain (or an invalid user name).
		if !names.IsValidUser(username) {
			return nil, false, errors.Errorf("%q is an invalid user name", username)
		}
		tag = names.NewUserTag(username)
		if tag.IsLocal() {
			return nil, false, errors.Errorf("external identity provider has provided ostensibly local name %q", username)
		}
	}
	return externalUser{tag: tag}, false, nil
}

// IdentityFromContext implements IdentityClient.IdentityFromContext.
func (m *ExternalMacaroonAuthenticator) IdentityFromContext(ctx context.Context) (identchecker.Identity, []checkers.Caveat, error) {
	expiryTime := m.Clock.Now().Add(externalLoginExpiryTime)
	return nil, []checkers.Caveat{
		checkers.TimeBeforeCaveat(expiryTime),
		checkers.NeedDeclaredCaveat(
			checkers.Caveat{
				Location:  m.IdentityLocation,
				Condition: "is-authenticated-user",
			},
			usernameKey,
		),
	}, nil
}

// DeclaredIdentity implements IdentityClient.DeclaredIdentity.
func (ExternalMacaroonAuthenticator) DeclaredIdentity(ctx context.Context, declared map[string]string) (identchecker.Identity, error) {
	if username, ok := declared[usernameKey]; ok {
		return identchecker.SimpleIdentity(username), nil
	}
	return nil, errors.New("no identity declared")
}

// CreateLocalLoginMacaroon creates a macaroon that may be provided to a
// user as proof that they have logged in with a valid username and password.
// This macaroon may then be used to obtain a discharge macaroon so that
// the user can log in without presenting their password for a set amount
// of time.
func CreateLocalLoginMacaroon(
	ctx context.Context,
	tag names.UserTag,
	minter MacaroonMinter,
	clock clock.Clock,
	version bakery.Version,
) (*bakery.Macaroon, error) {
	// We create the macaroon with a random ID and random root key, which
	// enables multiple clients to login as the same user and obtain separate
	// macaroons without having them use the same root key.
	return minter.NewMacaroon(ctx, version, []checkers.Caveat{
		{Condition: "is-authenticated-user " + tag.Id()},
		checkers.TimeBeforeCaveat(clock.Now().Add(LocalLoginInteractionTimeout)),
	}, identchecker.LoginOp)
}

// CheckLocalLoginCaveat parses and checks that the given caveat string is
// valid for a local login request, and returns the tag of the local user
// that the caveat asserts is logged in. checkers.ErrCaveatNotRecognized will
// be returned if the caveat is not recognised.
func CheckLocalLoginCaveat(caveat string) (names.UserTag, error) {
	var tag names.UserTag
	op, rest, err := checkers.ParseCaveat(caveat)
	if err != nil {
		return tag, errors.Annotatef(err, "cannot parse caveat %q", caveat)
	}
	if op != "is-authenticated-user" {
		return tag, checkers.ErrCaveatNotRecognized
	}
	if !names.IsValidUser(rest) {
		return tag, errors.NotValidf("username %q", rest)
	}
	tag = names.NewUserTag(rest)
	if !tag.IsLocal() {
		tag = names.UserTag{}
		return tag, errors.NotValidf("non-local username %q", rest)
	}
	return tag, nil
}

// CheckLocalLoginRequest checks that the given HTTP request contains at least
// one valid local login macaroon minted by the given service using
// CreateLocalLoginMacaroon. It returns an error with a
// *bakery.VerificationError cause if the macaroon verification failed.
func CheckLocalLoginRequest(
	ctx context.Context,
	auth MacaroonChecker,
	req *http.Request,
) error {
	a := auth.Auth(ctx, httpbakery.RequestMacaroons(req)...)
	ai, err := a.Allow(ctx, identchecker.LoginOp)
	if err != nil {
		return errors.Annotatef(err, "local login request failed: %v", req.Header[httpbakery.MacaroonsHeader])
	}
	logger.Tracef(ctx, "authenticated conditions: %v", ai.Conditions())
	if len(ai.Conditions()) == 0 {
		return &bakery.VerificationError{Reason: errors.New("no caveats available")}
	}
	return errors.Trace(err)
}

// DischargeCaveats returns the caveats to add to a login discharge macaroon.
func DischargeCaveats(tag names.UserTag, clock clock.Clock) []checkers.Caveat {
	firstPartyCaveats := []checkers.Caveat{
		checkers.DeclaredCaveat(usernameKey, tag.Id()),
		checkers.TimeBeforeCaveat(clock.Now().Add(localLoginExpiryTime)),
	}
	return firstPartyCaveats
}
