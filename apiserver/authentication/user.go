// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"context"
	"net/http"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon-bakery.v2/bakery/checkers"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.authentication")

// UserAuthenticator performs authentication for local users. If a password
type UserAuthenticator struct {
	AgentAuthenticator

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

var _ EntityAuthenticator = (*UserAuthenticator)(nil)

// Authenticate authenticates the entity with the specified tag, and returns an
// error on authentication failure.
//
// If and only if no password is supplied, then Authenticate will check for any
// valid macaroons. Otherwise, password authentication will be performed.
func (u *UserAuthenticator) Authenticate(
	ctx context.Context, entityFinder EntityFinder, tag names.Tag, req params.LoginRequest,
) (state.Entity, error) {
	userTag, ok := tag.(names.UserTag)
	if !ok {
		return nil, errors.Errorf("invalid request")
	}
	if req.Credentials == "" && userTag.IsLocal() {
		return u.authenticateMacaroons(ctx, entityFinder, userTag, req)
	}
	return u.AgentAuthenticator.Authenticate(ctx, entityFinder, tag, req)
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
	a := auth.Auth(httpbakery.RequestMacaroons(req)...)
	ai, err := a.Allow(ctx, identchecker.LoginOp)
	if err != nil {
		return errors.Trace(err)
	}
	if len(ai.Conditions()) == 0 {
		return &bakery.VerificationError{Reason: errors.New("no caveats available")}
	}
	return errors.Trace(err)
}

// Discharge caveats returns the caveats to add to a login discharge macaroon.
func DischargeCaveats(tag names.UserTag, clock clock.Clock) []checkers.Caveat {
	firstPartyCaveats := []checkers.Caveat{
		checkers.DeclaredCaveat(usernameKey, tag.Id()),
		checkers.TimeBeforeCaveat(clock.Now().Add(localLoginExpiryTime)),
	}
	return firstPartyCaveats
}

func (u *UserAuthenticator) authenticateMacaroons(
	ctx context.Context, entityFinder EntityFinder, tag names.UserTag, req params.LoginRequest,
) (state.Entity, error) {
	// Check for a valid request macaroon.
	a := u.Bakery.Auth(req.Macaroons...)
	ai, err := a.Allow(ctx, identchecker.LoginOp)
	if err != nil || len(ai.Conditions()) == 0 {
		logger.Debugf("local-login macaroon authentication failed: %v", err)
		cause := err
		if cause == nil {
			cause = errors.New("invalid login macaroon")
		}
		// The root keys for these macaroons are stored in MongoDB.
		// Expire the documents after after a set amount of time.
		expiryTime := u.Clock.Now().Add(localLoginExpiryTime)
		bakery, err := u.Bakery.ExpireStorageAfter(localLoginExpiryTime)
		if err != nil {
			return nil, errors.Trace(err)
		}

		m, err := bakery.NewMacaroon(
			ctx,
			req.BakeryVersion,
			[]checkers.Caveat{
				checkers.TimeBeforeCaveat(expiryTime),
				checkers.NeedDeclaredCaveat(
					checkers.Caveat{
						Location:  u.LocalUserIdentityLocation,
						Condition: "is-authenticated-user " + tag.Id(),
					},
					usernameKey,
				),
			}, identchecker.LoginOp)

		if err != nil {
			return nil, errors.Annotate(err, "cannot create macaroon")
		}
		return nil, &common.DischargeRequiredError{
			Cause:          cause,
			LegacyMacaroon: m.M(),
			Macaroon:       m,
		}
	}
	loginMac := ai.Macaroons[ai.OpIndexes[identchecker.LoginOp]]
	declared := checkers.InferDeclared(charmstore.MacaroonNamespace, loginMac)
	username := declared[usernameKey]
	if tag.Id() != username {
		return nil, common.ErrPerm
	}
	entity, err := entityFinder.FindEntity(tag)
	if errors.IsNotFound(err) {
		logger.Debugf("entity %s not found", tag.String())
		return nil, errors.Trace(common.ErrBadCreds)
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return entity, nil
}

// ExternalMacaroonAuthenticator performs authentication for external users using
// macaroons. If the authentication fails because provided macaroons are invalid,
// and macaroon authentiction is enabled, it will return a *common.DischargeRequiredError
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
func (m *ExternalMacaroonAuthenticator) Authenticate(ctx context.Context, entityFinder EntityFinder, _ names.Tag, req params.LoginRequest) (state.Entity, error) {
	authChecker := m.Bakery.Checker.Auth(req.Macaroons...)
	ai, identErr := authChecker.Allow(ctx, identchecker.LoginOp)
	if de, ok := errors.Cause(identErr).(*bakery.DischargeRequiredError); ok {
		if dcMac, err := m.Bakery.Oven.NewMacaroon(ctx, req.BakeryVersion, de.Caveats, de.Ops...); err != nil {
			return nil, errors.Annotatef(err, "cannot create macaroon")
		} else {
			return nil, &common.DischargeRequiredError{
				Cause:    identErr,
				Macaroon: dcMac,
			}
		}
	}
	if identErr != nil {
		return nil, errors.Trace(identErr)
	}
	username := ai.Identity.Id()
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
			return nil, errors.Errorf("%q is an invalid user name", username)
		}
		tag = names.NewUserTag(username)
		if tag.IsLocal() {
			return nil, errors.Errorf("external identity provider has provided ostensibly local name %q", username)
		}
	}
	entity, err := entityFinder.FindEntity(tag)
	if errors.IsNotFound(err) {
		return nil, errors.Trace(common.ErrBadCreds)
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return entity, nil
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

// Bakery defines the subset of bakery.Bakery that we require for authentication.
type Bakery interface {
	MacaroonMinter
	MacaroonChecker
}

// MacaroonChecker exposes the methods needed from bakery.Checker.
type MacaroonChecker interface {
	Auth(mss ...macaroon.Slice) *bakery.AuthChecker
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
