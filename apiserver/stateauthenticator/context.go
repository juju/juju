// Copyright 2015-2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator

import (
	"context"
	"net/http"
	"net/url"
	"sync"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"
	legacybakery "gopkg.in/macaroon-bakery.v2-unstable/bakery"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery/checkers"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"

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

	ctx       context.Context
	clock     clock.Clock
	agentAuth authentication.AgentAuthenticator

	// localUserBakeryService is the bakery.Service used by the controller
	// for authenticating local users. In time, we may want to use this for
	// both local and external users. Note that this service does not
	// discharge the third-party caveats.
	localUserBakeryService *bakeryutil.ExpirableStorageBakeryService

	// localUserThirdPartyBakeryService is the bakery.Service used by the
	// controller for discharging third-party caveats for local users.
	localUserThirdPartyBakeryService *legacybakery.Service

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
	ctx context.Context,
) (*authContext, error) {
	ctxt := &authContext{
		st:                    st,
		clock:                 clock,
		ctx:                   ctx,
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
		auth, err := a.ctxt.externalMacaroonAuth(nil)
		if errors.Cause(err) == errMacaroonAuthNotConfigured {
			err = errors.Trace(common.ErrNoCreds)
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		return auth, nil
	}
	for _, agentKind := range AgentTags {
		if tag.Kind() == agentKind {
			return &a.ctxt.agentAuth, nil
		}
	}
	if tag.Kind() == names.UserTagKind {
		return a.localUserAuth(), nil
	}
	return nil, errors.Annotatef(common.ErrBadRequest, "unexpected login entity tag")
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
		Service:                   a.ctxt.localUserBakeryService,
		Clock:                     a.ctxt.clock,
		LocalUserIdentityLocation: localUserIdentityLocation.String(),
	}
}

// externalMacaroonAuth returns an authenticator that can authenticate macaroon-based
// logins for external users. If it fails once, it will always fail.
func (ctxt *authContext) externalMacaroonAuth(identClient identchecker.IdentityClient) (authentication.EntityAuthenticator, error) {
	ctxt.macaroonAuthOnce.Do(func() {
		ctxt._macaroonAuth, ctxt._macaroonAuthError = newExternalMacaroonAuth(ctxt.ctx, ctxt.st, ctxt.clock, identClient)
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
func newExternalMacaroonAuth(ctx context.Context, st *state.State, clock clock.Clock, identClient identchecker.IdentityClient) (*authentication.ExternalMacaroonAuthenticator, error) {
	controllerCfg, err := st.ControllerConfig()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get model config")
	}
	idURL := controllerCfg.IdentityURL()
	if idURL == "" {
		return nil, errMacaroonAuthNotConfigured
	}
	idPK := controllerCfg.IdentityPublicKey()
	key, err := bakery.GenerateKey()
	if err != nil {
		return nil, errors.Trace(err)
	}

	pkCache := bakery.NewThirdPartyStore()
	pkLocator := httpbakery.NewThirdPartyLocator(nil, pkCache)
	if idPK != nil {
		pkCache.AddInfo(idURL, bakery.ThirdPartyInfo{
			PublicKey: *idPK,
			Version:   3,
		})
	}

	auth := authentication.ExternalMacaroonAuthenticator{
		Context:          ctx,
		Clock:            clock,
		IdentityLocation: idURL,
	}

	// We pass in nil for the storage, which leads to in-memory storage
	// being used. We only use in-memory storage for now, since we never
	// expire the keys, and don't want garbage to accumulate.
	//
	// TODO(axw) we should store the key in mongo, so that multiple servers
	// can authenticate. That will require that we encode the server's ID
	// in the macaroon ID so that servers don't overwrite each others' keys.
	if identClient == nil {
		identClient = &auth
	}
	indentBakery := identchecker.NewBakery(identchecker.BakeryParams{
		Checker:        httpbakery.NewChecker(),
		Locator:        pkLocator,
		Key:            key,
		IdentityClient: identClient,
		Authorizer: identchecker.ACLAuthorizer{
			GetACL: func(ctx context.Context, op bakery.Op) ([]string, bool, error) {
				return []string{identchecker.Everyone}, false, nil
			},
		},
		Location: idURL,
	})
	auth.Bakery = indentBakery
	if err != nil {
		return nil, errors.Annotate(err, "cannot make macaroon")
	}
	return &auth, nil
}
