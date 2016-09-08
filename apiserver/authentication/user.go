// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"net/http"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.authentication")

// UserAuthenticator performs authentication for local users. If a password
type UserAuthenticator struct {
	AgentAuthenticator

	// Service holds the service that is used to mint and verify macaroons.
	Service ExpirableStorageBakeryService

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
	entityFinder EntityFinder, tag names.Tag, req params.LoginRequest,
) (state.Entity, error) {
	userTag, ok := tag.(names.UserTag)
	if !ok {
		return nil, errors.Errorf("invalid request")
	}
	if req.Credentials == "" && userTag.IsLocal() {
		return u.authenticateMacaroons(entityFinder, userTag, req)
	}
	return u.AgentAuthenticator.Authenticate(entityFinder, tag, req)
}

// CreateLocalLoginMacaroon creates a macaroon that may be provided to a
// user as proof that they have logged in with a valid username and password.
// This macaroon may then be used to obtain a discharge macaroon so that
// the user can log in without presenting their password for a set amount
// of time.
func CreateLocalLoginMacaroon(
	tag names.UserTag,
	service BakeryService,
	clock clock.Clock,
) (*macaroon.Macaroon, error) {
	// We create the macaroon with a random ID and random root key, which
	// enables multiple clients to login as the same user and obtain separate
	// macaroons without having them use the same root key.
	return service.NewMacaroon("", nil, []checkers.Caveat{
		{Condition: "is-authenticated-user " + tag.Canonical()},
		checkers.TimeBeforeCaveat(clock.Now().Add(LocalLoginInteractionTimeout)),
	})
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
// *bakery.VerificationError cause if the macaroon verification failed. If the
// macaroon is valid, CheckLocalLoginRequest returns a list of caveats to add
// to the discharge macaroon.
func CheckLocalLoginRequest(
	service *bakery.Service,
	req *http.Request,
	tag names.UserTag,
	clock clock.Clock,
) ([]checkers.Caveat, error) {
	_, err := httpbakery.CheckRequest(service, req, nil, checkers.CheckerFunc{
		// Having a macaroon with an is-authenticated-user
		// caveat is proof that the user is "logged in".
		"is-authenticated-user",
		func(cond, arg string) error { return nil },
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	firstPartyCaveats := []checkers.Caveat{
		checkers.DeclaredCaveat("username", tag.Canonical()),
		checkers.TimeBeforeCaveat(clock.Now().Add(localLoginExpiryTime)),
	}
	return firstPartyCaveats, nil
}

func (u *UserAuthenticator) authenticateMacaroons(
	entityFinder EntityFinder, tag names.UserTag, req params.LoginRequest,
) (state.Entity, error) {
	// Check for a valid request macaroon.
	assert := map[string]string{usernameKey: tag.Canonical()}
	_, err := u.Service.CheckAny(req.Macaroons, assert, checkers.New(checkers.TimeBefore))
	if err != nil {
		cause := err
		logger.Debugf("local-login macaroon authentication failed: %v", cause)
		if _, ok := errors.Cause(err).(*bakery.VerificationError); !ok {
			return nil, errors.Trace(err)
		}

		// The root keys for these macaroons are stored in MongoDB.
		// Expire the documents after after a set amount of time.
		expiryTime := u.Clock.Now().Add(localLoginExpiryTime)
		service, err := u.Service.ExpireStorageAt(expiryTime)
		if err != nil {
			return nil, errors.Trace(err)
		}

		m, err := service.NewMacaroon("", nil, []checkers.Caveat{
			checkers.NeedDeclaredCaveat(
				checkers.Caveat{
					Location:  u.LocalUserIdentityLocation,
					Condition: "is-authenticated-user " + tag.Canonical(),
				},
				usernameKey,
			),
			checkers.TimeBeforeCaveat(expiryTime),
		})
		if err != nil {
			return nil, errors.Annotate(err, "cannot create macaroon")
		}
		return nil, &common.DischargeRequiredError{
			Cause:    cause,
			Macaroon: m,
		}
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
	// Service holds the service that is
	// used to verify macaroon authorization.
	Service BakeryService

	// Macaroon guards macaroon-authentication-based access
	// to the APIs. Appropriate caveats will be added before
	// sending it to a client.
	Macaroon *macaroon.Macaroon

	// IdentityLocation holds the URL of the trusted third party
	// that is used to address the is-authenticated-user
	// third party caveat to.
	IdentityLocation string
}

var _ EntityAuthenticator = (*ExternalMacaroonAuthenticator)(nil)

func (m *ExternalMacaroonAuthenticator) newDischargeRequiredError(cause error) error {
	if m.Service == nil || m.Macaroon == nil {
		return errors.Trace(cause)
	}
	mac := m.Macaroon.Clone()
	// TODO(fwereade): 2016-03-17 lp:1558657
	expiryTime := time.Now().Add(externalLoginExpiryTime)
	if err := addMacaroonTimeBeforeCaveat(m.Service, mac, expiryTime); err != nil {
		return errors.Annotatef(err, "cannot create macaroon")
	}
	err := m.Service.AddCaveat(mac, checkers.NeedDeclaredCaveat(
		checkers.Caveat{
			Location:  m.IdentityLocation,
			Condition: "is-authenticated-user",
		},
		usernameKey,
	))
	if err != nil {
		return errors.Annotatef(err, "cannot create macaroon")
	}
	return &common.DischargeRequiredError{
		Cause:    cause,
		Macaroon: mac,
	}
}

// Authenticate authenticates the provided entity. If there is no macaroon provided, it will
// return a *DischargeRequiredError containing a macaroon that can be used to grant access.
func (m *ExternalMacaroonAuthenticator) Authenticate(entityFinder EntityFinder, _ names.Tag, req params.LoginRequest) (state.Entity, error) {
	declared, err := m.Service.CheckAny(req.Macaroons, nil, checkers.New(checkers.TimeBefore))
	if _, ok := errors.Cause(err).(*bakery.VerificationError); ok {
		return nil, m.newDischargeRequiredError(err)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	username := declared[usernameKey]
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

func addMacaroonTimeBeforeCaveat(svc BakeryService, m *macaroon.Macaroon, t time.Time) error {
	return svc.AddCaveat(m, checkers.TimeBeforeCaveat(t))
}

// BakeryService defines the subset of bakery.Service
// that we require for authentication.
type BakeryService interface {
	AddCaveat(*macaroon.Macaroon, checkers.Caveat) error
	CheckAny([]macaroon.Slice, map[string]string, checkers.Checker) (map[string]string, error)
	NewMacaroon(string, []byte, []checkers.Caveat) (*macaroon.Macaroon, error)
}

// ExpirableStorageBakeryService extends BakeryService
// with the ExpireStorageAt method so that root keys are
// removed from storage at that time.
type ExpirableStorageBakeryService interface {
	BakeryService

	// ExpireStorageAt returns a new ExpirableStorageBakeryService with
	// a store that will expire items added to it at the specified time.
	ExpireStorageAt(time.Time) (ExpirableStorageBakeryService, error)
}
