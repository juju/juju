// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator

import (
	"context"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"gopkg.in/httprequest.v1"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon-bakery.v2/bakery/checkers"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmstore"
)

// TODO(juju3) - legacy login handlers are not needed once we stop supporting Juju 2 clients.

// AddHandlers adds the local login handlers to the given mux.
func (h *localLoginHandlers) AddLegacyHandlers(mux *apiserverhttp.Mux, dischargeMux *http.ServeMux) {
	makeHandler := func(h func(http.ResponseWriter, *http.Request) (interface{}, error)) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			val, err := h(w, req)
			if err != nil {
				httpbakery.WriteError(context.TODO(), w, err)
				return
			}
			httprequest.WriteJSON(w, http.StatusOK, val)
		})
	}
	dischargeMux.Handle(
		localUserIdentityLocationPath+"/login",
		makeHandler(h.serveLogin),
	)
	dischargeMux.Handle(
		localUserIdentityLocationPath+"/wait",
		makeHandler(h.serveWait),
	)
	mux.AddHandler("GET", localUserIdentityLocationPath+"/wait", dischargeMux)
	mux.AddHandler("GET", localUserIdentityLocationPath+"/login", dischargeMux)
	mux.AddHandler("POST", localUserIdentityLocationPath+"/login", dischargeMux)
}

func (h *localLoginHandlers) serveLogin(response http.ResponseWriter, req *http.Request) (interface{}, error) {
	switch req.Method {
	case "POST":
		return h.serveLoginPost(response, req)
	case "GET":
		return h.serveLoginGet(req)
	default:
		return nil, errors.Errorf("unsupported method %q", req.Method)
	}
}

func (h *localLoginHandlers) serveLoginPost(response http.ResponseWriter, req *http.Request) (interface{}, error) {
	if err := req.ParseForm(); err != nil {
		return nil, err
	}
	waitId := req.Form.Get("waitid")
	if waitId == "" {
		return nil, errors.NotValidf("missing waitid")
	}
	username := req.Form.Get("user")
	password := req.Form.Get("password")
	if !names.IsValidUser(username) {
		return nil, errors.NotValidf("username %q", username)
	}
	userTag := names.NewUserTag(username)
	if !userTag.IsLocal() {
		return nil, errors.NotValidf("non-local username %q", username)
	}

	authenticator := h.authCtxt.authenticator(req.Host)
	if _, err := authenticator.Authenticate(req.Context(), h.finder, userTag, params.LoginRequest{
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
	m, err := h.authCtxt.CreateLocalLoginMacaroon(req.Context(), userTag, httpbakery.RequestVersion(req))
	if err != nil {
		return nil, err
	}
	cookie, err := httpbakery.NewCookie(charmstore.MacaroonNamespace, macaroon.Slice{m.M()})
	if err != nil {
		return nil, err
	}
	http.SetCookie(response, cookie)

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

func (h *localLoginHandlers) serveLoginGet(req *http.Request) (interface{}, error) {
	if req.Header.Get("Accept") == "application/json" {
		// The application/json content-type is used to
		// inform the client of the supported auth methods.
		return map[string]string{
			"juju_userpass": req.URL.String(),
		}, nil
	}
	// TODO(axw) return an HTML form. If waitid is supplied,
	// it should be passed through so we can unblock a request
	// on the /auth/wait endpoint. We should also support logging
	// in when not specifically directed to the login page.
	return nil, errors.NotImplementedf("GET")
}

func (h *localLoginHandlers) serveWait(_ http.ResponseWriter, req *http.Request) (interface{}, error) {
	if err := req.ParseForm(); err != nil {
		return nil, err
	}
	if req.Method != "GET" {
		return nil, errors.Errorf("unsupported method %q", req.Method)
	}
	waitId := req.Form.Get("waitid")
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
		req:         req,
	}
	mac, err := bakery.Discharge(context.TODO(), bakery.DischargeParams{
		Id:      interaction.CaveatId,
		Key:     h.authCtxt.localUserThirdPartyBakeryKey,
		Checker: bakery.ThirdPartyCaveatCheckerFunc(ctx.legacyCheckThirdPartyCaveat),
	})
	if err != nil {
		return nil, errors.Annotate(err, "discharging macaroon")
	}
	return httpbakery.WaitResponse{mac}, nil
}

type macaroonAuthContext struct {
	*authContext
	req *http.Request
}

func (ctx *macaroonAuthContext) legacyCheckThirdPartyCaveat(stdCtx context.Context, cavInfo *bakery.ThirdPartyCaveatInfo) ([]checkers.Caveat, error) {
	tag, err := ctx.CheckLocalLoginCaveat(string(cavInfo.Condition))
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := ctx.CheckLocalLoginRequest(stdCtx, ctx.req); err != nil {
		return nil, errors.Trace(err)
	}
	return ctx.DischargeCaveats(tag), nil
}
