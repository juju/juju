// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/httprequest"
	"github.com/julienschmidt/httprouter"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	macaroon "gopkg.in/macaroon.v1"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

var (
	errorMapper httprequest.ErrorMapper = httpbakery.ErrorToResponse
	handleJSON                          = errorMapper.HandleJSON
)

func makeHandler(h httprouter.Handle) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		h(w, req, nil)
	})
}

type localLoginHandlers struct {
	authCtxt *authContext
	state    *state.State
}

func (h *localLoginHandlers) serveLogin(p httprequest.Params) (interface{}, error) {
	switch p.Request.Method {
	case "POST":
		return h.serveLoginPost(p)
	case "GET":
		return h.serveLoginGet(p)
	default:
		return nil, errors.Errorf("unsupported method %q", p.Request.Method)
	}
}

func (h *localLoginHandlers) serveLoginPost(p httprequest.Params) (interface{}, error) {
	if err := p.Request.ParseForm(); err != nil {
		return nil, err
	}
	waitId := p.Request.Form.Get("waitid")
	if waitId == "" {
		return nil, errors.NotValidf("missing waitid")
	}
	username := p.Request.Form.Get("user")
	password := p.Request.Form.Get("password")
	if !names.IsValidUser(username) {
		return nil, errors.NotValidf("username %q", username)
	}
	userTag := names.NewUserTag(username)
	if !userTag.IsLocal() {
		return nil, errors.NotValidf("non-local username %q", username)
	}

	authenticator := h.authCtxt.authenticator(p.Request.Host)
	if _, err := authenticator.Authenticate(h.state, userTag, params.LoginRequest{
		Credentials: password,
	}); err != nil {
		// Mark the interaction as done (but failed),
		// unblocking a pending "/auth/wait" request.
		if err := h.authCtxt.localUserInteractions.Done(waitId, userTag, err); err != nil {
			if !errors.IsNotFound(err) {
				logger.Warningf(
					"failed to record completion of interaction %q for %q",
					waitId, userTag.Id(),
				)
			}
		}
		return nil, errors.Trace(err)
	}

	// Provide the client with a macaroon that they can use to
	// prove that they have logged in, and obtain a discharge
	// macaroon.
	m, err := h.authCtxt.CreateLocalLoginMacaroon(userTag)
	if err != nil {
		return nil, err
	}
	cookie, err := httpbakery.NewCookie(macaroon.Slice{m})
	if err != nil {
		return nil, err
	}
	http.SetCookie(p.Response, cookie)

	// Mark the interaction as done, unblocking a pending
	// "/auth/wait" request.
	if err := h.authCtxt.localUserInteractions.Done(
		waitId, userTag, nil,
	); err != nil {
		if errors.IsNotFound(err) {
			err = errors.New("login timed out")
		}
		return nil, err
	}
	return nil, nil
}

func (h *localLoginHandlers) serveLoginGet(p httprequest.Params) (interface{}, error) {
	if p.Request.Header.Get("Accept") == "application/json" {
		// The application/json content-type is used to
		// inform the client of the supported auth methods.
		return map[string]string{
			"juju_userpass": p.Request.URL.String(),
		}, nil
	}
	// TODO(axw) return an HTML form. If waitid is supplied,
	// it should be passed through so we can unblock a request
	// on the /auth/wait endpoint. We should also support logging
	// in when not specifically directed to the login page.
	return nil, errors.NotImplementedf("GET")
}

func (h *localLoginHandlers) serveWait(p httprequest.Params) (interface{}, error) {
	if err := p.Request.ParseForm(); err != nil {
		return nil, err
	}
	if p.Request.Method != "GET" {
		return nil, errors.Errorf("unsupported method %q", p.Request.Method)
	}
	waitId := p.Request.Form.Get("waitid")
	if waitId == "" {
		return nil, errors.NotValidf("missing waitid")
	}
	interaction, err := h.authCtxt.localUserInteractions.Wait(waitId, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if interaction.LoginError != nil {
		return nil, errors.Trace(err)
	}
	ctx := macaroonAuthContext{
		authContext: h.authCtxt,
		req:         p.Request,
	}
	macaroon, err := h.authCtxt.localUserThirdPartyBakeryService.Discharge(
		&ctx, interaction.CaveatId,
	)
	if err != nil {
		return nil, errors.Annotate(err, "discharging macaroon")
	}
	return httpbakery.WaitResponse{macaroon}, nil
}

func (h *localLoginHandlers) checkThirdPartyCaveat(req *http.Request, cavId, cav string) ([]checkers.Caveat, error) {
	ctx := &macaroonAuthContext{authContext: h.authCtxt, req: req}
	return ctx.CheckThirdPartyCaveat(cavId, cav)
}

type macaroonAuthContext struct {
	*authContext
	req *http.Request
}

// CheckThirdPartyCaveat is part of the bakery.ThirdPartyChecker interface.
func (ctx *macaroonAuthContext) CheckThirdPartyCaveat(cavId, cav string) ([]checkers.Caveat, error) {
	tag, err := ctx.CheckLocalLoginCaveat(cav)
	if err != nil {
		return nil, errors.Trace(err)
	}
	firstPartyCaveats, err := ctx.CheckLocalLoginRequest(ctx.req, tag)
	if err != nil {
		if _, ok := errors.Cause(err).(*bakery.VerificationError); ok {
			waitId, err := ctx.localUserInteractions.Start(
				cavId,
				ctx.clock.Now().Add(authentication.LocalLoginInteractionTimeout),
			)
			if err != nil {
				return nil, errors.Trace(err)
			}
			visitURL := localUserIdentityLocationPath + "/login?waitid=" + waitId
			waitURL := localUserIdentityLocationPath + "/wait?waitid=" + waitId
			return nil, httpbakery.NewInteractionRequiredError(visitURL, waitURL, nil, ctx.req)
		}
		return nil, errors.Trace(err)
	}
	return firstPartyCaveats, nil
}
