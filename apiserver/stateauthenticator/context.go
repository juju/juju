// Copyright 2015-2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator

import (
	"net/http"
	"net/url"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/bakeryutil"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

const (
	localUserIdentityLocationPath = "/auth"
)

// authContext holds authentication context shared
// between all API endpoints.
type authContext struct {
	st *state.State

	clock     clock.Clock
	agentAuth authentication.AgentAuthenticator

	// localUserBakeryService is the bakery.Service used by the controller
	// for authenticating local users. In time, we may want to use this for
	// both local and external users. Note that this service does not
	// discharge the third-party caveats.
	localUserBakeryService *bakeryutil.ExpirableStorageBakeryService

	// localUserThirdPartyBakeryService is the bakery.Service used by the
	// controller for discharging third-party caveats for local users.
	localUserThirdPartyBakeryService *bakery.Service

	// localUserInteractions maintains a set of in-progress local user
	// authentication interactions.
	localUserInteractions *authentication.Interactions

	// macaroonAuthOnce guards the fields below it.
	macaroonAuthOnce   sync.Once
	_macaroonAuth      *authentication.ExternalMacaroonAuthenticator
	_macaroonAuthError error
}

// newAuthContext creates a new authentication context for st.
func newAuthContext(
	st *state.State,
	clock clock.Clock,
) (*authContext, error) {
	ctxt := &authContext{
		st:    st,
		clock: clock,
		localUserInteractions: authentication.NewInteractions(),
	}

	// Create a bakery service for discharging third-party caveats for
	// local user authentication. This service does not persist keys;
	// its macaroons should be very short-lived.
	localUserThirdPartyBakeryService, _, err := bakeryutil.NewBakeryService(st, nil, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ctxt.localUserThirdPartyBakeryService = localUserThirdPartyBakeryService

	// Create a bakery service for local user authentication. This service
	// persists keys into MongoDB in a TTL collection.
	store, err := st.NewBakeryStorage()
	if err != nil {
		return nil, errors.Trace(err)
	}
	locator := bakeryutil.BakeryServicePublicKeyLocator{ctxt.localUserThirdPartyBakeryService}
	localUserBakeryService, localUserBakeryServiceKey, err := bakeryutil.NewBakeryService(
		st, store, locator,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ctxt.localUserBakeryService = &bakeryutil.ExpirableStorageBakeryService{
		localUserBakeryService, localUserBakeryServiceKey, store, locator,
	}
	return ctxt, nil
}

// CreateLocalLoginMacaroon creates a macaroon that may be provided to a user
// as proof that they have logged in with a valid username and password. This
// macaroon may then be used to obtain a discharge macaroon so that the user
// can log in without presenting their password for a set amount of time.
func (ctxt *authContext) CreateLocalLoginMacaroon(tag names.UserTag) (*macaroon.Macaroon, error) {
	return authentication.CreateLocalLoginMacaroon(tag, ctxt.localUserThirdPartyBakeryService, ctxt.clock)
}

// CheckLocalLoginCaveat parses and checks that the given caveat string is
// valid for a local login request, and returns the tag of the local user
// that the caveat asserts is logged in. checkers.ErrCaveatNotRecognized will
// be returned if the caveat is not recognised.
func (ctxt *authContext) CheckLocalLoginCaveat(caveat string) (names.UserTag, error) {
	return authentication.CheckLocalLoginCaveat(caveat)
}

// CheckLocalLoginRequest checks that the given HTTP request contains at least
// one valid local login macaroon minted using CreateLocalLoginMacaroon. It
// returns an error with a *bakery.VerificationError cause if the macaroon
// verification failed. If the macaroon is valid, CheckLocalLoginRequest returns
// a list of caveats to add to the discharge macaroon.
func (ctxt *authContext) CheckLocalLoginRequest(req *http.Request, tag names.UserTag) ([]checkers.Caveat, error) {
	return authentication.CheckLocalLoginRequest(ctxt.localUserThirdPartyBakeryService, req, tag, ctxt.clock)
}

// authenticator returns an authenticator.EntityAuthenticator for the API
// connection associated with the specified API server host.
func (ctxt *authContext) authenticator(serverHost string) authenticator {
	return authenticator{ctxt: ctxt, serverHost: serverHost}
}

// authenticator implements authenticator.EntityAuthenticator, delegating
// to the appropriate authenticator based on the tag kind.
type authenticator struct {
	ctxt       *authContext
	serverHost string
}

// Authenticate implements authentication.EntityAuthenticator
// by choosing the right kind of authentication for the given
// tag.
func (a authenticator) Authenticate(
	entityFinder authentication.EntityFinder,
	tag names.Tag,
	req params.LoginRequest,
) (state.Entity, error) {
	auth, err := a.authenticatorForTag(tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return auth.Authenticate(entityFinder, tag, req)
}

// authenticatorForTag returns the authenticator appropriate
// to use for a login with the given possibly-nil tag.
func (a authenticator) authenticatorForTag(tag names.Tag) (authentication.EntityAuthenticator, error) {
	if tag == nil {
		auth, err := a.ctxt.externalMacaroonAuth()
		if errors.Cause(err) == errMacaroonAuthNotConfigured {
			err = errors.Trace(common.ErrNoCreds)
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		return auth, nil
	}
	switch tag.Kind() {
	case names.UnitTagKind, names.MachineTagKind, names.ApplicationTagKind:
		return &a.ctxt.agentAuth, nil
	case names.UserTagKind:
		return a.localUserAuth(), nil
	default:
		return nil, errors.Annotatef(common.ErrBadRequest, "unexpected login entity tag")
	}
}

// localUserAuth returns an authenticator that can authenticate logins for
// local users with either passwords or macaroons.
func (a authenticator) localUserAuth() *authentication.UserAuthenticator {
	localUserIdentityLocation := url.URL{
		Scheme: "https",
		Host:   a.serverHost,
		Path:   localUserIdentityLocationPath,
	}
	return &authentication.UserAuthenticator{
		Service: a.ctxt.localUserBakeryService,
		Clock:   a.ctxt.clock,
		LocalUserIdentityLocation: localUserIdentityLocation.String(),
	}
}

// externalMacaroonAuth returns an authenticator that can authenticate macaroon-based
// logins for external users. If it fails once, it will always fail.
func (ctxt *authContext) externalMacaroonAuth() (authentication.EntityAuthenticator, error) {
	ctxt.macaroonAuthOnce.Do(func() {
		ctxt._macaroonAuth, ctxt._macaroonAuthError = newExternalMacaroonAuth(ctxt.st)
	})
	if ctxt._macaroonAuth == nil {
		return nil, errors.Trace(ctxt._macaroonAuthError)
	}
	return ctxt._macaroonAuth, nil
}

var errMacaroonAuthNotConfigured = errors.New("macaroon authentication is not configured")

// newExternalMacaroonAuth returns an authenticator that can authenticate
// macaroon-based logins for external users. This is just a helper function
// for authCtxt.externalMacaroonAuth.
func newExternalMacaroonAuth(st *state.State) (*authentication.ExternalMacaroonAuthenticator, error) {
	controllerCfg, err := st.ControllerConfig()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get model config")
	}
	idURL := controllerCfg.IdentityURL()
	if idURL == "" {
		return nil, errMacaroonAuthNotConfigured
	}
	var locator bakery.PublicKeyLocator
	// The identity server has been configured,
	// so configure the bakery service appropriately.
	idPK := controllerCfg.IdentityPublicKey()
	if idPK != nil {
		locator = bakery.PublicKeyLocatorMap{idURL: idPK}
	} else {
		// No public key supplied - retrieve it from the identity manager on demand.
		// Note that we don't fetch it immediately because then we'll fail
		// forever if the initial fetch fails (because newExternalMacaroonAuth
		// only ever called once).
		locator = httpbakery.NewPublicKeyRing(nil, nil)
	}
	// We pass in nil for the storage, which leads to in-memory storage
	// being used. We only use in-memory storage for now, since we never
	// expire the keys, and don't want garbage to accumulate.
	//
	// TODO(axw) we should store the key in mongo, so that multiple servers
	// can authenticate. That will require that we encode the server's ID
	// in the macaroon ID so that servers don't overwrite each others' keys.
	svc, _, err := bakeryutil.NewBakeryService(st, nil, locator)
	if err != nil {
		return nil, errors.Annotate(err, "cannot make bakery service")
	}
	var auth authentication.ExternalMacaroonAuthenticator
	auth.Service = svc
	auth.Macaroon, err = svc.NewMacaroon("api-login", nil, nil)
	if err != nil {
		return nil, errors.Annotate(err, "cannot make macaroon")
	}
	auth.IdentityLocation = idURL
	return &auth, nil
}
