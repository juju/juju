// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/httpcontext"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// AgentTags are those used by any Juju agent.
var AgentTags = []string{
	names.MachineTagKind,
	names.ControllerAgentTagKind,
	names.UnitTagKind,
	names.ApplicationTagKind,
	names.ModelTagKind,
}

// Authenticator is an implementation of httpcontext.Authenticator,
// using *state.State for authentication.
//
// This Authenticator only works with requests that have been handled
// by one of the httpcontext.*ModelHandler handlers.
type Authenticator struct {
	statePool   *state.StatePool
	authContext *authContext
}

// NewAuthenticator returns a new Authenticator using the given StatePool.
func NewAuthenticator(statePool *state.StatePool, clock clock.Clock) (*Authenticator, error) {
	systemState, err := statePool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	authContext, err := newAuthContext(systemState, clock)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &Authenticator{
		statePool:   statePool,
		authContext: authContext,
	}, nil
}

// Maintain periodically expires local login interactions.
func (a *Authenticator) Maintain(done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		case <-a.authContext.clock.After(authentication.LocalLoginInteractionTimeout):
			now := a.authContext.clock.Now()
			a.authContext.localUserInteractions.Expire(now)
		}
	}
}

// CreateLocalLoginMacaroon is part of the
// httpcontext.LocalMacaroonAuthenticator interface.
func (a *Authenticator) CreateLocalLoginMacaroon(ctx context.Context, tag names.UserTag, version bakery.Version) (*macaroon.Macaroon, error) {
	mac, err := a.authContext.CreateLocalLoginMacaroon(ctx, tag, version)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return mac.M(), nil
}

// AddHandlers adds the handlers to the given mux for handling local
// macaroon logins.
func (a *Authenticator) AddHandlers(mux *apiserverhttp.Mux) error {
	systemState, err := a.statePool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}
	h := &localLoginHandlers{
		authCtxt:   a.authContext,
		finder:     systemState,
		userTokens: map[string]string{},
	}
	h.AddHandlers(mux)
	return nil
}

// Authenticate is part of the httpcontext.Authenticator interface.
func (a *Authenticator) Authenticate(req *http.Request) (httpcontext.AuthInfo, error) {
	modelUUID := httpcontext.RequestModelUUID(req)
	if modelUUID == "" {
		return httpcontext.AuthInfo{}, errors.New("model UUID not found")
	}
	loginRequest, err := LoginRequest(req)
	if err != nil {
		return httpcontext.AuthInfo{}, errors.Trace(err)
	}
	return a.AuthenticateLoginRequest(req.Context(), req.Host, modelUUID, loginRequest)
}

// AuthenticateLoginRequest authenticates a LoginRequest.
//
// TODO(axw) we shouldn't be using params types here.
func (a *Authenticator) AuthenticateLoginRequest(
	ctx context.Context,
	serverHost string,
	modelUUID string,
	req params.LoginRequest,
) (httpcontext.AuthInfo, error) {
	var authTag names.Tag
	if req.AuthTag != "" {
		tag, err := names.ParseTag(req.AuthTag)
		if err != nil {
			return httpcontext.AuthInfo{}, errors.Trace(err)
		}
		authTag = tag
	}

	st, err := a.statePool.Get(modelUUID)
	if err != nil {
		return httpcontext.AuthInfo{}, errors.Trace(err)
	}
	defer st.Release()

	authenticator := a.authContext.authenticator(serverHost)
	authInfo, err := a.checkCreds(ctx, st.State, req, authTag, true, authenticator)
	if err != nil {
		var dischargeRequired *apiservererrors.DischargeRequiredError
		if errors.As(err, &dischargeRequired) || errors.Is(err, errors.NotProvisioned) {
			// TODO(axw) move out of common?
			return httpcontext.AuthInfo{}, errors.Trace(err)
		}
		_, isMachineTag := authTag.(names.MachineTag)
		_, isControllerAgentTag := authTag.(names.ControllerAgentTag)
		systemState, errS := a.statePool.SystemState()
		if errS != nil {
			return httpcontext.AuthInfo{}, errors.Trace(err)
		}
		if (isMachineTag || isControllerAgentTag) && !st.IsController() {
			// Controller agents are allowed to log into any model.
			var err2 error
			authInfo, err2 = a.checkCreds(
				ctx,
				systemState,
				req, authTag, false, authenticator,
			)
			if err2 == nil && authInfo.Controller {
				err = nil
			}
		}
		if err != nil {
			return httpcontext.AuthInfo{}, errors.NewUnauthorized(err, "")
		}
	}
	authInfo.ModelTag = names.NewModelTag(modelUUID)
	return authInfo, nil
}

func (a *Authenticator) checkCreds(
	ctx context.Context,
	st *state.State,
	req params.LoginRequest,
	authTag names.Tag,
	userLogin bool,
	authenticator authentication.Authenticator,
) (httpcontext.AuthInfo, error) {
	var entityFinder authentication.EntityFinder = st
	if userLogin {
		// When looking up model users, use a custom
		// entity finder that looks up both the local user (if the user
		// tag is in the local domain) and the model user.
		entityFinder = modelUserEntityFinder{st}
	}
	entity, err := authenticator.Authenticate(ctx, entityFinder, authTag, req)
	if err != nil {
		return httpcontext.AuthInfo{}, errors.Trace(err)
	}

	authInfo := httpcontext.AuthInfo{Entity: entity}
	type withIsManager interface {
		IsManager() bool
	}
	if entity, ok := entity.(withIsManager); ok {
		authInfo.Controller = entity.IsManager()
	}

	type withLogin interface {
		UpdateLastLogin() error
	}
	if entity, ok := entity.(withLogin); ok {
		if err := entity.UpdateLastLogin(); err != nil {
			logger.Warningf("updating last login time for %v", authTag)
		}
	}
	return authInfo, nil
}

// LoginRequest extracts basic auth login details from an http.Request.
//
// TODO(axw) we shouldn't be using params types here.
func LoginRequest(req *http.Request) (params.LoginRequest, error) {
	authHeader := req.Header.Get("Authorization")
	macaroons := httpbakery.RequestMacaroons(req)

	if authHeader == "" {
		return params.LoginRequest{Macaroons: macaroons}, nil
	}
	username, password, ok := req.BasicAuth()
	if !ok {
		// Invalid header format or no header provided.
		return params.LoginRequest{}, errors.NotValidf("request format")
	}

	// Ensure that a sensible tag was passed.
	if _, err := names.ParseTag(username); err != nil {
		return params.LoginRequest{}, errors.Trace(err)
	}

	bakeryVersion, _ := strconv.Atoi(req.Header.Get(httpbakery.BakeryProtocolHeader))
	loginRequest := params.LoginRequest{
		AuthTag:       username,
		Credentials:   password,
		Nonce:         req.Header.Get(params.MachineNonceHeader),
		Macaroons:     macaroons,
		BakeryVersion: bakery.Version(bakeryVersion),
	}
	// Default client version to 2 since older 2.x clients
	// don't send this field.
	requestClientVersion := version.Number{Major: 2}
	if clientVersion, err := common.JujuClientVersionFromRequest(req); err == nil {
		requestClientVersion = clientVersion
	}
	loginRequest.ClientVersion = requestClientVersion.String()
	return loginRequest, nil
}
