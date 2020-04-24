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
	"github.com/juju/names/v4"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon-bakery.v2/bakery/checkers"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"
	"gopkg.in/macaroon-bakery.v2/httpbakery"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/bakeryutil"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmstore"
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

	// localUserBakery is the bakery.Bakery used by the controller
	// for authenticating local users. In time, we may want to use this for
	// both local and external users. Note that this service does not
	// discharge the third-party caveats.
	localUserBakery *bakeryutil.ExpirableStorageBakery

	// localUserThirdPartyBakery is the bakery.Bakery used by the
	// controller for discharging third-party caveats for local users.
	localUserThirdPartyBakery *bakery.Bakery
	// localUserThirdPartyBakeryKey is the bakery.Bakery's key.
	localUserThirdPartyBakeryKey *bakery.KeyPair

	// localUserInteractions maintains a set of in-progress local user
	// authentication interactions.
	localUserInteractions *authentication.Interactions

	// macaroonAuthOnce guards the fields below it.
	macaroonAuthOnce   sync.Once
	_macaroonAuth      *authentication.ExternalMacaroonAuthenticator
	_macaroonAuthError error
}

// OpenAuthorizer authorises any login operation presented to it.
type OpenLoginAuthorizer struct{}

// AuthorizeOps implements OpsAuthorizer.AuthorizeOps.
func (OpenLoginAuthorizer) AuthorizeOps(ctx context.Context, authorizedOp bakery.Op, queryOps []bakery.Op) ([]bool, []checkers.Caveat, error) {
	logger.Debugf("authorize query ops check for %v: %v", authorizedOp, queryOps)
	allowed := make([]bool, len(queryOps))
	for i := range allowed {
		allowed[i] = queryOps[i] == identchecker.LoginOp
	}
	return allowed, nil, nil
}

// newAuthContext creates a new authentication context for st.
func newAuthContext(
	st *state.State,
	clock clock.Clock,
) (*authContext, error) {
	ctxt := &authContext{
		st:                    st,
		clock:                 clock,
		localUserInteractions: authentication.NewInteractions(),
	}

	// Create a bakery for discharging third-party caveats for
	// local user authentication. This service does not persist keys;
	// its macaroons should be very short-lived.
	checker := checkers.New(charmstore.MacaroonNamespace)
	checker.Register("is-authenticated-user", charmstore.MacaroonURI,
		// Having a macaroon with an is-authenticated-user
		// caveat is proof that the user is "logged in".
		//"is-authenticated-user",
		func(ctx context.Context, cond, arg string) error { return nil },
	)
	location := "juju model " + st.ModelUUID()
	var err error
	ctxt.localUserThirdPartyBakeryKey, err = bakery.GenerateKey()
	if err != nil {
		return nil, errors.Annotate(err, "generating key for local user third party bakery key")
	}
	ctxt.localUserThirdPartyBakery = bakery.New(
		bakery.BakeryParams{
			Checker:       checker,
			Key:           ctxt.localUserThirdPartyBakeryKey,
			OpsAuthorizer: OpenLoginAuthorizer{},
			Location:      location,
		})

	// Create a bakery service for local user authentication. This service
	// persists keys into MongoDB in a TTL collection.
	store, err := st.NewBakeryStorage()
	if err != nil {
		return nil, errors.Trace(err)
	}
	locator := bakeryutil.BakeryThirdPartyLocator{PublicKey: ctxt.localUserThirdPartyBakeryKey.Public}
	localUserBakeryKey, err := bakery.GenerateKey()
	if err != nil {
		return nil, errors.Annotate(err, "generating key for local user bakery key")
	}
	localUserBakery := bakery.New(
		bakery.BakeryParams{
			RootKeyStore:  store,
			Key:           localUserBakeryKey,
			OpsAuthorizer: OpenLoginAuthorizer{},
			Location:      location,
		})

	ctxt.localUserBakery = &bakeryutil.ExpirableStorageBakery{
		localUserBakery, location, localUserBakeryKey, store, locator,
	}
	return ctxt, nil
}

// CreateLocalLoginMacaroon creates a macaroon that may be provided to a user
// as proof that they have logged in with a valid username and password. This
// macaroon may then be used to obtain a discharge macaroon so that the user
// can log in without presenting their password for a set amount of time.
func (ctxt *authContext) CreateLocalLoginMacaroon(ctx context.Context, tag names.UserTag, version bakery.Version) (*bakery.Macaroon, error) {
	return authentication.CreateLocalLoginMacaroon(ctx, tag, ctxt.localUserThirdPartyBakery.Oven, ctxt.clock, version)
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
// verification failed.
func (ctxt *authContext) CheckLocalLoginRequest(ctx context.Context, req *http.Request) error {
	return authentication.CheckLocalLoginRequest(ctx, ctxt.localUserThirdPartyBakery.Checker, req)
}

// Discharge caveats returns the caveats to add to a login discharge macaroon.
func (ctxt *authContext) DischargeCaveats(tag names.UserTag) []checkers.Caveat {
	return authentication.DischargeCaveats(tag, ctxt.clock)
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
	ctx context.Context,
	entityFinder authentication.EntityFinder,
	tag names.Tag,
	req params.LoginRequest,
) (state.Entity, error) {
	auth, err := a.authenticatorForTag(tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return auth.Authenticate(ctx, entityFinder, tag, req)
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
		Bakery:                    a.ctxt.localUserBakery,
		Clock:                     a.ctxt.clock,
		LocalUserIdentityLocation: localUserIdentityLocation.String(),
	}
}

// externalMacaroonAuth returns an authenticator that can authenticate macaroon-based
// logins for external users. If it fails once, it will always fail.
func (ctxt *authContext) externalMacaroonAuth(identClient identchecker.IdentityClient) (authentication.EntityAuthenticator, error) {
	ctxt.macaroonAuthOnce.Do(func() {
		ctxt._macaroonAuth, ctxt._macaroonAuthError = newExternalMacaroonAuth(ctxt.st, ctxt.clock, identClient)
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
func newExternalMacaroonAuth(st *state.State, clock clock.Clock, identClient identchecker.IdentityClient) (*authentication.ExternalMacaroonAuthenticator, error) {
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
