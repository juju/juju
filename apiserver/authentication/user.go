// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
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
}

const (
	usernameKey = "username"

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

// CreateLocalLoginMacaroon creates a time-limited macaroon for a local user
// to log into the controller with. The macaroon will be valid for use with
// UserAuthenticator.Authenticate until the time limit expires, or the Juju
// controller agent restarts.
//
// NOTE(axw) this method will generate a key for a previously unseen user,
// and store it in the bakery.Service's storage. Callers should first ensure
// the user is valid before calling this, to avoid filling storage with keys
// for invalid users.
func (u *UserAuthenticator) CreateLocalLoginMacaroon(tag names.UserTag) (*macaroon.Macaroon, error) {

	// Ensure that the private key that we generate and store will be
	// removed from storage once the expiry time has elapsed.
	expiryTime := u.Clock.Now().Add(localLoginExpiryTime)
	bakeryService, err := u.Service.ExpireStorageAt(expiryTime)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// We create the macaroon with a random ID and random root key, which
	// enables multiple clients to login as the same user and obtain separate
	// macaroons without having them use the same root key.
	m, err := bakeryService.NewMacaroon("", nil, []checkers.Caveat{
		// The macaroon may only be used to log in as the user
		// specified by the tag passed to CreateLocalUserMacaroon.
		checkers.DeclaredCaveat(usernameKey, tag.Canonical()),
	})
	if err != nil {
		return nil, errors.Annotate(err, "cannot create macaroon")
	}
	if err := addMacaroonTimeBeforeCaveat(bakeryService, m, expiryTime); err != nil {
		return nil, errors.Trace(err)
	}
	return m, nil
}

func (u *UserAuthenticator) authenticateMacaroons(
	entityFinder EntityFinder, tag names.UserTag, req params.LoginRequest,
) (state.Entity, error) {
	// Check for a valid request macaroon.
	assert := map[string]string{usernameKey: tag.Canonical()}
	_, err := u.Service.CheckAny(req.Macaroons, assert, checkers.New(checkers.TimeBefore))
	if err != nil {
		logger.Debugf("local-login macaroon authentication failed: %v", err)
		if allMacaroonsExpired(u.Clock.Now(), req.Macaroons) {
			return nil, common.ErrLoginExpired
		}
		return nil, errors.Trace(common.ErrBadCreds)
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

// allMacaroonsExpired reports whether or not all of the macaroon
// slices' primary macaroons have expired.
func allMacaroonsExpired(now time.Time, ms []macaroon.Slice) bool {
	for _, ms := range ms {
		if len(ms) == 0 {
			continue
		}
		m := ms[0]
		var expired bool
		for _, c := range m.Caveats() {
			if c.Location != "" {
				continue
			}
			cond, arg, err := checkers.ParseCaveat(c.Id)
			if err != nil {
				continue
			}
			if cond != checkers.CondTimeBefore {
				continue
			}
			t, err := time.Parse(time.RFC3339Nano, arg)
			if err != nil {
				return false
			}
			if !now.Before(t) {
				expired = true
			}
		}
		if !expired {
			return false
		}
	}
	return true
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
