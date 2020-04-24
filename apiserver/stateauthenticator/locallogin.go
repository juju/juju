// Copyright 2016-2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"gopkg.in/httprequest.v1"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon-bakery.v2/bakery/checkers"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon-bakery.v2/httpbakery/form"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

var (
	logger = loggo.GetLogger("juju.apiserver.stateauthenticator")
)

type localLoginHandlers struct {
	authCtxt   *authContext
	finder     state.EntityFinder
	userTokens map[string]string
}

var formURL = "/form"

// AddHandlers adds the local login handlers to the given mux.
func (h *localLoginHandlers) AddHandlers(mux *apiserverhttp.Mux) {
	dischargeMux := http.NewServeMux()
	discharger := httpbakery.NewDischarger(
		httpbakery.DischargerParams{
			Key:     h.authCtxt.localUserThirdPartyBakeryKey,
			Checker: httpbakery.ThirdPartyCaveatCheckerFunc(h.checkThirdPartyCaveat),
		})
	discharger.AddMuxHandlers(dischargeMux, localUserIdentityLocationPath)

	dischargeMux.Handle(
		localUserIdentityLocationPath+formURL,
		http.HandlerFunc(h.formHandler),
	)
	mux.AddHandler("POST", localUserIdentityLocationPath+"/discharge", dischargeMux)
	mux.AddHandler("GET", localUserIdentityLocationPath+"/publickey", dischargeMux)
	mux.AddHandler("GET", localUserIdentityLocationPath+"/form", dischargeMux)
	mux.AddHandler("POST", localUserIdentityLocationPath+"/form", dischargeMux)

	h.AddLegacyHandlers(mux, dischargeMux)
}

func (h *localLoginHandlers) bakeryError(w http.ResponseWriter, err error) {
	httpbakery.WriteError(context.TODO(), w, err)
}

func (h *localLoginHandlers) formHandler(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "POST":
		ctx := req.Context()
		reqParams := httprequest.Params{
			Response: w,
			Request:  req,
			Context:  ctx,
		}
		loginRequest := form.LoginRequest{}
		if err := httprequest.Unmarshal(reqParams, &loginRequest); err != nil {
			h.bakeryError(w, errors.Annotate(err, "can't unmarshal login request"))
			return
		}

		username := loginRequest.Body.Form["user"].(string)
		password := loginRequest.Body.Form["password"].(string)
		userTag := names.NewUserTag(username)
		if !userTag.IsLocal() {
			h.bakeryError(w, errors.NotValidf("non-local username %q", username))
			return
		}

		authenticator := h.authCtxt.authenticator(req.Host)
		if _, err := authenticator.Authenticate(ctx, h.finder, userTag, params.LoginRequest{
			Credentials: password,
		}); err != nil {
			h.bakeryError(w, err)
			return
		}

		token, err := newId()
		if err != nil {
			h.bakeryError(w, errors.Annotate(err, "cannot generate token"))
			return
		}
		h.userTokens[token] = username

		loginResponse := form.LoginResponse{
			Token: &httpbakery.DischargeToken{
				Kind:  "juju_userpass",
				Value: []byte(token),
			},
		}
		httprequest.WriteJSON(w, http.StatusOK, loginResponse)
	default:
		http.Error(w, fmt.Sprintf("%s method not allowed", req.Method), http.StatusMethodNotAllowed)
	}
}

func newId() (string, error) {
	var id [12]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("cannot read random id: %v", err)
	}
	return fmt.Sprintf("%x", id[:]), nil
}

func (h *localLoginHandlers) checkThirdPartyCaveat(stdCtx context.Context, req *http.Request, cavInfo *bakery.ThirdPartyCaveatInfo, token *httpbakery.DischargeToken) ([]checkers.Caveat, error) {
	tag, err := h.authCtxt.CheckLocalLoginCaveat(string(cavInfo.Condition))
	if err != nil {
		return nil, errors.Trace(err)
	}
	if token == nil {
		if err := h.authCtxt.CheckLocalLoginRequest(stdCtx, req); err == nil {
			return h.authCtxt.DischargeCaveats(tag), nil
		}
		err2 := httpbakery.NewInteractionRequiredError(nil, req)
		err2.SetInteraction("juju_userpass", form.InteractionInfo{URL: localUserIdentityLocationPath + formURL})

		// TODO(juju3) - remove legacy client support
		waitId, err := h.authCtxt.localUserInteractions.Start(
			cavInfo.Caveat,
			h.authCtxt.clock.Now().Add(authentication.LocalLoginInteractionTimeout),
		)
		if err != nil {
			return nil, errors.Trace(err)
		}
		visitURL := localUserIdentityLocationPath + "/login?waitid=" + waitId
		waitURL := localUserIdentityLocationPath + "/wait?waitid=" + waitId
		httpbakery.SetLegacyInteraction(err2, visitURL, waitURL)
		return nil, err2
	}

	tokenString := string(token.Value)
	username, ok := h.userTokens[tokenString]
	delete(h.userTokens, tokenString)
	if token.Kind != "juju_userpass" || !ok {
		return nil, errors.Errorf("invalid token %#v", token)
	}

	// Sanity check.
	if tag.Id() != username {
		return nil, errors.Errorf("discharge token for user %q does not match declared user %q", username, tag.Id())
	}
	return h.authCtxt.DischargeCaveats(tag), nil
}
