// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net/http"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// authContext holds authentication context shared
// between all API endpoints.
type authContext struct {
	st *state.State

	agentAuth authentication.AgentAuthenticator
	userAuth  authentication.UserAuthenticator

	// macaroonAuthOnce guards the fields below it.
	macaroonAuthOnce   sync.Once
	_macaroonAuth      *authentication.ExternalMacaroonAuthenticator
	_macaroonAuthError error
}

// newAuthContext creates a new authentication context for st.
func newAuthContext(st *state.State) (*authContext, error) {
	ctxt := &authContext{st: st}
	bakeryService, err := newBakeryService(st, authentication.LocalLoginExpiryTime, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ctxt.userAuth.Service = bakeryService
	return ctxt, nil
}

// Authenticate implements authentication.EntityAuthenticator
// by choosing the right kind of authentication for the given
// tag.
func (ctxt *authContext) Authenticate(entityFinder authentication.EntityFinder, tag names.Tag, req params.LoginRequest) (state.Entity, error) {
	auth, err := ctxt.authenticatorForTag(tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return auth.Authenticate(entityFinder, tag, req)
}

// authenticatorForTag returns the authenticator appropriate
// to use for a login with the given possibly-nil tag.
func (ctxt *authContext) authenticatorForTag(tag names.Tag) (authentication.EntityAuthenticator, error) {
	if tag == nil {
		auth, err := ctxt.macaroonAuth()
		if errors.Cause(err) == errMacaroonAuthNotConfigured {
			// Make a friendlier error message.
			err = errors.New("no credentials provided")
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		return auth, nil
	}
	switch tag.Kind() {
	case names.UnitTagKind, names.MachineTagKind:
		return &ctxt.agentAuth, nil
	case names.UserTagKind:
		return &ctxt.userAuth, nil
	default:
		return nil, errors.Annotatef(common.ErrBadRequest, "unexpected login entity tag")
	}
}

// macaroonAuth returns an authenticator that can authenticate macaroon-based
// logins. If it fails once, it will always fail.
func (ctxt *authContext) macaroonAuth() (authentication.EntityAuthenticator, error) {
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
// for authCtxt.macaroonAuth.
func newExternalMacaroonAuth(st *state.State) (*authentication.ExternalMacaroonAuthenticator, error) {
	envCfg, err := st.ModelConfig()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get model config")
	}
	idURL := envCfg.IdentityURL()
	if idURL == "" {
		return nil, errMacaroonAuthNotConfigured
	}
	// The identity server has been configured,
	// so configure the bakery service appropriately.
	idPK := envCfg.IdentityPublicKey()
	if idPK == nil {
		// No public key supplied - retrieve it from the identity manager.
		idPK, err = httpbakery.PublicKeyForLocation(http.DefaultClient, idURL)
		if err != nil {
			return nil, errors.Annotate(err, "cannot get identity public key")
		}
	}
	svc, err := newBakeryService(st, authentication.ExternalLoginExpiryTime, bakery.PublicKeyLocatorMap{
		idURL: idPK,
	})
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

// newBakeryService creates a new bakery.Service.
func newBakeryService(st *state.State, expiry time.Duration, locator bakery.PublicKeyLocator) (*bakery.Service, error) {
	storage, err := st.NewBakeryStorage(expiry)
	if err != nil {
		return nil, errors.Annotate(err, "creating bakery storage")
	}
	return bakery.NewService(bakery.NewServiceParams{
		Location: "juju model " + st.ModelUUID(),
		Locator:  locator,
		Store:    storage,
	})
}
