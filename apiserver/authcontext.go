// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net/http"
	"sync"

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
	srv *Server

	agentAuth authentication.AgentAuthenticator
	userAuth  authentication.UserAuthenticator

	// macaroonAuthOnce guards the fields below it.
	macaroonAuthOnce   sync.Once
	_macaroonAuth      *authentication.MacaroonAuthenticator
	_macaroonAuthError error
}

// newAuthContext creates a new authentication context for srv.
func newAuthContext(srv *Server) *authContext {
	return &authContext{
		srv: srv,
	}
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
		ctxt._macaroonAuth, ctxt._macaroonAuthError = newMacaroonAuth(ctxt.srv.statePool.SystemState())
	})
	if ctxt._macaroonAuth == nil {
		return nil, errors.Trace(ctxt._macaroonAuthError)
	}
	return ctxt._macaroonAuth, nil
}

var errMacaroonAuthNotConfigured = errors.New("macaroon authentication is not configured")

// newMacaroonAuth returns an authenticator that can authenticate
// macaroon-based logins. This is just a helper function for authCtxt.macaroonAuth.
func newMacaroonAuth(st *state.State) (*authentication.MacaroonAuthenticator, error) {
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
	svc, err := bakery.NewService(
		bakery.NewServiceParams{
			Location: "juju model " + st.ModelUUID(),
			Locator: bakery.PublicKeyLocatorMap{
				idURL: idPK,
			},
		},
	)
	if err != nil {
		return nil, errors.Annotate(err, "cannot make bakery service")
	}
	var auth authentication.MacaroonAuthenticator
	auth.Service = svc
	auth.Macaroon, err = svc.NewMacaroon("api-login", nil, nil)
	if err != nil {
		return nil, errors.Annotate(err, "cannot make macaroon")
	}
	auth.IdentityLocation = idURL
	return &auth, nil
}
